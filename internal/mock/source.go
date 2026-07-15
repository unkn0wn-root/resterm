package mock

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

func Load(path string, recursive bool, overlay *restfile.Document) (*Handler, error) {
	docs, err := LoadDocuments(path, recursive, overlay)
	if err != nil {
		return nil, err
	}
	dir, err := fixtureRoot(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open mock fixture root %s: %w", dir, err)
	}
	defer func() { _ = root.Close() }()
	return compile(docs, rootedReader(root, dir))
}

func fixtureRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat mock source %s: %w", path, err)
	}
	if !info.IsDir() {
		path = filepath.Dir(path)
	}
	dir, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve mock fixture root %s: %w", path, err)
	}
	return dir, nil
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

func LoadDocuments(path string, recursive bool, overlay *restfile.Document) ([]*restfile.Document, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat mock source %s: %w", path, err)
	}

	if !info.IsDir() {
		if !filesvc.IsRequestFile(path) {
			return nil, fmt.Errorf("mock source must be a .http or .rest file: %s", path)
		}
		doc, err := loadDoc(path, overlay)
		if err != nil {
			return nil, err
		}
		return []*restfile.Document{doc}, nil
	}

	entries, err := filesvc.ListRequestFiles(path, recursive)
	if err != nil {
		return nil, fmt.Errorf("list mock workspace %s: %w", path, err)
	}
	docs := make([]*restfile.Document, 0, len(entries)+1)
	found := false
	for _, e := range entries {
		if e.Kind != filesvc.FileKindRequest {
			continue
		}
		doc, err := loadDoc(e.Path, overlay)
		if err != nil {
			return nil, err
		}
		if overlay != nil && util.SamePath(e.Path, overlay.Path) {
			found = true
		}
		docs = append(docs, doc)
	}
	if !found && includeOverlay(path, overlay) {
		docs = append(docs, overlay)
	}
	return docs, nil
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
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	overlayAbs, err := filepath.Abs(overlay.Path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, overlayAbs)
	return err == nil && filepath.IsLocal(rel)
}
