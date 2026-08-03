package mock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

// Sources selects the request files a mock server compiles. Path is a request
// file or a directory whose request files load by default. Files narrows a
// directory Path to those files. Path stays the root that file-based response
// bodies resolve against and are confined to.
type Sources struct {
	Path      string
	Recursive bool
	Files     []string
}

// NewSources resolves entries against root. Entries may repeat and may hold
// comma-separated lists. Duplicates collapse to their first mention.
func NewSources(root string, recursive bool, entries []string) (Sources, error) {
	if len(entries) == 0 {
		return Sources{Path: root, Recursive: recursive}, nil
	}
	if recursive {
		return Sources{}, errors.New("--recursive cannot be combined with --source")
	}

	info, err := os.Stat(root)
	if err != nil {
		return Sources{}, fmt.Errorf("stat mock source root %s: %w", root, err)
	}
	if !info.IsDir() {
		return Sources{}, fmt.Errorf("mock sources need a directory root: %s", root)
	}

	src := Sources{Path: root}
	for _, entry := range entries {
		for name := range strings.SplitSeq(entry, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			path, err := rootedSource(root, name)
			if err != nil {
				return Sources{}, err
			}
			known := func(f string) bool { return util.SamePath(f, path) }
			if !slices.ContainsFunc(src.Files, known) {
				src.Files = append(src.Files, path)
			}
		}
	}
	if len(src.Files) == 0 {
		return Sources{}, errors.New("no mock source files named")
	}
	return src, nil
}

// rootedSource rejects entries outside root because fixture bodies are confined
// to it. Such a document could only serve inline responses and would fail as
// soon as it referenced a file.
func rootedSource(root, name string) (string, error) {
	path := filepath.Clean(name)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if !filesvc.IsRequestFile(path) {
		return "", fmt.Errorf("mock source must be a .http or .rest file: %s", name)
	}
	if !underRoot(root, path) {
		return "", fmt.Errorf("mock source is outside the mock source root: %s", name)
	}
	return path, nil
}

func underRoot(root, path string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, abs)
	return err == nil && filepath.IsLocal(rel)
}

type resolved struct {
	files     []string
	root      string
	workspace bool // whole-directory scope, the only one an unlisted overlay may join
}

// resolve is the only place Sources turns into a file set, so the loader and
// the reload fingerprint can never disagree about which files are in scope.
func (s Sources) resolve() (resolved, error) {
	path := strings.TrimSpace(s.Path)
	if path == "" {
		path = "."
	}
	info, err := os.Stat(path)
	if err != nil {
		return resolved{}, fmt.Errorf("stat mock source %s: %w", path, err)
	}
	root := path
	if !info.IsDir() {
		root = filepath.Dir(path)
	}

	switch {
	case len(s.Files) > 0:
		for _, f := range s.Files {
			if !filesvc.IsRequestFile(f) {
				return resolved{}, fmt.Errorf("mock source must be a .http or .rest file: %s", f)
			}
		}
		return resolved{files: s.Files, root: root}, nil
	case !info.IsDir():
		if !filesvc.IsRequestFile(path) {
			return resolved{}, fmt.Errorf("mock source must be a .http or .rest file: %s", path)
		}
		return resolved{files: []string{path}, root: root}, nil
	}

	entries, err := filesvc.ListRequestFiles(path, s.Recursive)
	if err != nil {
		return resolved{}, fmt.Errorf("list mock workspace %s: %w", path, err)
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Kind == filesvc.FileKindRequest {
			files = append(files, e.Path)
		}
	}
	return resolved{files: files, root: root, workspace: true}, nil
}

// documents parses the resolved set, substituting overlay for its own file. An
// overlay outside the set joins whole-directory scope only. A caller that
// listed files already named the exact set to serve.
func (r resolved) documents(overlay *restfile.Document) ([]*restfile.Document, error) {
	docs := make([]*restfile.Document, 0, len(r.files)+1)
	found := false
	for _, f := range r.files {
		doc, err := loadDoc(f, overlay)
		if err != nil {
			return nil, err
		}
		if overlay != nil && util.SamePath(f, overlay.Path) {
			found = true
		}
		docs = append(docs, doc)
	}
	if r.workspace && !found && includeOverlay(r.root, overlay) {
		docs = append(docs, overlay)
	}
	return docs, nil
}

func Load(src Sources, overlay *restfile.Document) (*Handler, error) {
	res, err := src.resolve()
	if err != nil {
		return nil, err
	}
	docs, err := res.documents(overlay)
	if err != nil {
		return nil, err
	}
	dir, err := filepath.Abs(res.root)
	if err != nil {
		return nil, fmt.Errorf("resolve mock fixture root %s: %w", res.root, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open mock fixture root %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()
	return compile(docs, rootedReader(root, dir))
}

func LoadDocuments(src Sources, overlay *restfile.Document) ([]*restfile.Document, error) {
	res, err := src.resolve()
	if err != nil {
		return nil, err
	}
	return res.documents(overlay)
}

func rootedReader(root *os.Root, dir string) fixtureReader {
	return func(path, ref string) ([]byte, string, error) {
		if path == "" {
			return nil, "", fmt.Errorf("relative mock response body requires a document path")
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, "", fmt.Errorf("resolve mock document path %s: %w", path, err)
		}
		rel, err := filepath.Rel(dir, abs)
		if err != nil || !filepath.IsLocal(rel) {
			return nil, "", fmt.Errorf("read mock response body %q: document is outside mock source root", ref)
		}
		if filepath.IsAbs(ref) {
			return nil, "", fmt.Errorf("read mock response body %q: path escapes mock source root", ref)
		}
		name := filepath.Clean(filepath.Join(filepath.Dir(rel), ref))
		if !filepath.IsLocal(name) {
			return nil, "", fmt.Errorf("read mock response body %q: path escapes mock source root", ref)
		}
		body, err := root.ReadFile(name)
		if err != nil {
			return nil, "", fmt.Errorf("read mock response body %q: %w", ref, err)
		}
		return body, filepath.Join(dir, name), nil
	}
}

func loadDoc(path string, overlay *restfile.Document) (*restfile.Document, error) {
	if overlay != nil && overlay.Path != "" && util.SamePath(path, overlay.Path) {
		return overlay, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mock source %s: %w", path, err)
	}
	return parser.Parse(path, data), nil
}

func includeOverlay(root string, overlay *restfile.Document) bool {
	if overlay == nil {
		return false
	}
	if strings.TrimSpace(overlay.Path) == "" {
		return len(overlay.Mocks) > 0
	}
	if !filesvc.IsRequestFile(overlay.Path) {
		return false
	}
	return underRoot(root, overlay.Path)
}
