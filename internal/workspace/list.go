package workspace

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

const (
	templateOpenMarker  = "{{"
	templateCloseMarker = "}}"
	parentRel           = ".."

	// Separates absolute paths in seenRTSModuleRefs keys. NUL cannot appear in
	// valid file paths, so it avoids ambiguity without escaping.
	pathKeySeparator = "\x00"
)

var templateMarkers = [...]string{
	templateOpenMarker,
	templateCloseMarker,
}

type ListOptions struct {
	Recursive       bool
	ExplicitEnvFile string
	CurrentFile     string
	CurrentDoc      *restfile.Document
}

func List(root string, opt ListOptions) ([]files.Entry, error) {
	l := newLister(root, opt)
	return l.list()
}

type lister struct {
	root              string
	rootAbs           string
	opt               ListOptions
	entries           map[string]files.Entry
	docs              []files.Entry
	seenRTSModuleRefs map[string]struct{}
}

func newLister(root string, opt ListOptions) *lister {
	root = filepath.Clean(root)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}

	return &lister{
		root:              root,
		rootAbs:           rootAbs,
		opt:               opt,
		entries:           make(map[string]files.Entry),
		seenRTSModuleRefs: make(map[string]struct{}),
	}
}

func (l *lister) list() ([]files.Entry, error) {
	base, err := l.loadBase()
	if err != nil {
		return nil, err
	}
	l.addBaseEntries(base)
	l.addCurrentFile()
	l.addDocumentRefs()
	return l.sorted(), nil
}

func (l *lister) loadBase() ([]files.Entry, error) {
	return files.ListWorkspace(l.root, files.ListOptions{
		Recursive:       l.opt.Recursive,
		ExplicitEnvFile: l.opt.ExplicitEnvFile,
	})
}

func (l *lister) addBaseEntries(base []files.Entry) {
	for _, e := range base {
		switch e.Kind {
		case files.KindRequest:
			l.addEntry(e)
			l.addDoc(e)
		case files.KindEnv:
			l.addEntry(e)
		}
	}
}

func (l *lister) addCurrentFile() {
	e, ok := l.currentEntry()
	if !ok {
		return
	}
	l.addEntry(e)
	if e.Kind == files.KindRequest {
		l.addDoc(e)
	}
}

func (l *lister) addDocumentRefs() {
	for _, e := range l.docs {
		doc := l.loadDoc(e.Path)
		if doc == nil {
			continue
		}
		l.addRefs(e.Path, Refs(doc))
	}
}

func (l *lister) addRefs(src string, refs []Ref) {
	for _, ref := range refs {
		l.addRef(src, ref)
	}
}

func (l *lister) addRef(referrerPath string, ref Ref) {
	e, ok := l.refEntry(referrerPath, ref.Path)
	if !ok {
		return
	}
	l.addEntry(e)
	l.addRTSModuleRefs(referrerPath, e)
}

func (l *lister) addRTSModuleRefs(referrerPath string, e files.Entry) {
	if e.Kind != files.KindScript {
		return
	}

	key := rtsModuleRefScanKey(e.Path, referrerPath)
	if _, ok := l.seenRTSModuleRefs[key]; ok {
		return
	}
	l.seenRTSModuleRefs[key] = struct{}{}

	data, err := os.ReadFile(e.Path)
	if err != nil {
		return
	}

	for _, path := range jsonFileModuleRefs(e.Path, string(data)) {
		l.addRef(referrerPath, Ref{Path: path, Kind: RefRTSJSON})
	}
}

func (l *lister) loadDoc(path string) *restfile.Document {
	if l.opt.CurrentDoc != nil && util.SamePath(util.Trim(path), util.Trim(l.opt.CurrentFile)) {
		return l.opt.CurrentDoc
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parser.Parse(path, data)
}

func (l *lister) addEntry(e files.Entry) {
	l.entries[pathKey(e.Path)] = e
}

func (l *lister) addDoc(e files.Entry) {
	for _, doc := range l.docs {
		if util.SamePath(util.Trim(doc.Path), util.Trim(e.Path)) {
			return
		}
	}
	l.docs = append(l.docs, e)
}

func (l *lister) currentEntry() (files.Entry, bool) {
	path := util.Trim(l.opt.CurrentFile)
	if path == "" {
		return files.Entry{}, false
	}
	kind, ok := files.ClassifyWorkspace(path)
	if !ok {
		return files.Entry{}, false
	}
	return l.entryFor(path, kind)
}

func (l *lister) refEntry(src, ref string) (files.Entry, bool) {
	ref = util.Trim(ref)
	if ref == "" {
		return files.Entry{}, false
	}
	if isDynamicRef(ref) {
		return files.Entry{}, false
	}

	path := ref
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(src), path)
	}

	kind, ok := files.ClassifyWorkspace(path)
	if !ok {
		return files.Entry{}, false
	}
	return l.entryFor(path, kind)
}

func (l *lister) entryFor(path string, kind files.Kind) (files.Entry, bool) {
	pathAbs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		pathAbs = filepath.Clean(path)
	}

	rel, err := filepath.Rel(l.rootAbs, pathAbs)
	if err != nil {
		return files.Entry{}, false
	}
	if relEscapesRoot(rel) {
		return files.Entry{}, false
	}

	info, err := os.Stat(pathAbs)
	if err != nil || info.IsDir() {
		return files.Entry{}, false
	}
	return files.Entry{
		Name: rel,
		Path: filepath.Join(l.root, rel),
		Kind: kind,
	}, true
}

func (l *lister) sorted() []files.Entry {
	out := make([]files.Entry, 0, len(l.entries))
	for _, e := range l.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Path < out[j].Path
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func pathKey(path string) string {
	path = filepath.Clean(util.Trim(path))
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func isDynamicRef(ref string) bool {
	for _, marker := range templateMarkers {
		if util.Contains(ref, marker) {
			return true
		}
	}
	return false
}

func relEscapesRoot(rel string) bool {
	switch {
	case rel == parentRel:
		return true
	case util.HasPrefix(rel, parentRel+string(filepath.Separator)):
		return true
	default:
		return false
	}
}

func rtsModuleRefScanKey(modulePath, referrerPath string) string {
	rtbase := util.Trim(referrerPath)
	if rtbase == "" {
		rtbase = modulePath
	}
	return pathKey(modulePath) + pathKeySeparator + pathKey(filepath.Dir(rtbase))
}
