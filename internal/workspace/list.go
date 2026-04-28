package workspace

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type ListOptions struct {
	Recursive       bool
	ExplicitEnvFile string
	CurrentFile     string
	CurrentDoc      *restfile.Document
}

func List(root string, opt ListOptions) ([]filesvc.FileEntry, error) {
	l := newLister(root, opt)
	return l.list()
}

type lister struct {
	root     string
	rootAbs  string
	opt      ListOptions
	entries  map[string]filesvc.FileEntry
	docs     []filesvc.FileEntry
	seenMods map[string]struct{}
}

func newLister(root string, opt ListOptions) *lister {
	root = filepath.Clean(root)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}

	return &lister{
		root:     root,
		rootAbs:  rootAbs,
		opt:      opt,
		entries:  make(map[string]filesvc.FileEntry),
		seenMods: make(map[string]struct{}),
	}
}

func (l *lister) list() ([]filesvc.FileEntry, error) {
	base, err := l.loadBase()
	if err != nil {
		return nil, err
	}
	l.addBaseEntries(base)
	l.addCurrentFile()
	l.addDocumentRefs()
	return l.sorted(), nil
}

func (l *lister) loadBase() ([]filesvc.FileEntry, error) {
	return filesvc.ListWorkspaceFiles(
		l.root,
		l.opt.Recursive,
		filesvc.ListOptions{ExplicitEnvFile: l.opt.ExplicitEnvFile},
	)
}

func (l *lister) addBaseEntries(base []filesvc.FileEntry) {
	for _, e := range base {
		switch e.Kind {
		case filesvc.FileKindRequest:
			l.addEntry(e)
			l.addDoc(e)
		case filesvc.FileKindEnv:
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
	if e.Kind == filesvc.FileKindRequest {
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

func (l *lister) addRef(src string, ref Ref) {
	e, ok := l.refEntry(src, ref.Path)
	if !ok {
		return
	}
	l.addEntry(e)
	l.addRTSModuleRefs(e)
}

func (l *lister) addRTSModuleRefs(e filesvc.FileEntry) {
	if e.Kind != filesvc.FileKindScript {
		return
	}

	key := pathKey(e.Path)
	if _, ok := l.seenMods[key]; ok {
		return
	}
	l.seenMods[key] = struct{}{}

	data, err := os.ReadFile(e.Path)
	if err != nil {
		return
	}

	for _, path := range jsonFileModuleRefs(e.Path, string(data)) {
		l.addRef(e.Path, Ref{Path: path, Kind: RefRTSJSON})
	}
}

func (l *lister) loadDoc(path string) *restfile.Document {
	if l.opt.CurrentDoc != nil && samePath(path, l.opt.CurrentFile) {
		return l.opt.CurrentDoc
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parser.Parse(path, data)
}

func (l *lister) addEntry(e filesvc.FileEntry) {
	l.entries[pathKey(e.Path)] = e
}

func (l *lister) addDoc(e filesvc.FileEntry) {
	for _, doc := range l.docs {
		if samePath(doc.Path, e.Path) {
			return
		}
	}
	l.docs = append(l.docs, e)
}

func (l *lister) currentEntry() (filesvc.FileEntry, bool) {
	path := str.Trim(l.opt.CurrentFile)
	if path == "" {
		return filesvc.FileEntry{}, false
	}
	kind, ok := filesvc.ClassifyWorkspacePath(path)
	if !ok {
		return filesvc.FileEntry{}, false
	}
	return l.entryFor(path, kind)
}

func (l *lister) refEntry(src, ref string) (filesvc.FileEntry, bool) {
	ref = str.Trim(ref)
	if ref == "" || str.Contains(ref, "{{") || str.Contains(ref, "}}") {
		return filesvc.FileEntry{}, false
	}

	path := ref
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(src), path)
	}

	kind, ok := filesvc.ClassifyWorkspacePath(path)
	if !ok {
		return filesvc.FileEntry{}, false
	}
	return l.entryFor(path, kind)
}

func (l *lister) entryFor(path string, kind filesvc.FileKind) (filesvc.FileEntry, bool) {
	pathAbs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		pathAbs = filepath.Clean(path)
	}

	rel, err := filepath.Rel(l.rootAbs, pathAbs)
	if err != nil || rel == ".." || str.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filesvc.FileEntry{}, false
	}

	info, err := os.Stat(pathAbs)
	if err != nil || info.IsDir() {
		return filesvc.FileEntry{}, false
	}
	return filesvc.FileEntry{
		Name: rel,
		Path: filepath.Join(l.root, rel),
		Kind: kind,
	}, true
}

func (l *lister) sorted() []filesvc.FileEntry {
	out := make([]filesvc.FileEntry, 0, len(l.entries))
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

func samePath(a, b string) bool {
	if str.Trim(a) == "" || str.Trim(b) == "" {
		return false
	}
	return pathKey(a) == pathKey(b)
}

func pathKey(path string) string {
	path = filepath.Clean(str.Trim(path))
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
