package files

import (
	"cmp"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ListOptions configures both listers. ExplicitEnvFile applies to ListWorkspace
// only, since ListRequests never yields environment entries.
type ListOptions struct {
	Recursive       bool
	ExplicitEnvFile string
}

// classifier decides which kind a path holds, if any. ClassifyRequest and
// ClassifyWorkspace are the two implementations, and they are what separates a
// request listing from a workspace listing.
type classifier func(path string) (Kind, bool)

func ListRequests(root string, opts ListOptions) ([]Entry, error) {
	return listFiles(root, ClassifyRequest, ListOptions{Recursive: opts.Recursive})
}

func ListWorkspace(root string, opts ListOptions) ([]Entry, error) {
	return listFiles(root, ClassifyWorkspace, opts)
}

func listFiles(root string, classify classifier, opts ListOptions) ([]Entry, error) {
	l := newLister(root, classify)

	var err error
	if opts.Recursive {
		err = l.walk()
	} else {
		err = l.readDir()
	}
	if err != nil {
		return nil, err
	}

	if entry, ok := explicitWorkspaceEnvEntry(root, opts.ExplicitEnvFile); ok {
		l.put(entry)
	}
	return l.sorted(), nil
}

// lister keys entries by cleaned path so a file reached twice, by the walk and
// by an explicit env file, collapses to one entry.
type lister struct {
	root     string
	classify classifier
	byPath   map[string]Entry
}

func newLister(root string, classify classifier) *lister {
	return &lister{
		root:     root,
		classify: classify,
		byPath:   make(map[string]Entry),
	}
}

func (l *lister) put(e Entry) {
	l.byPath[filepath.Clean(e.Path)] = e
}

func (l *lister) add(path string, kind Kind) {
	rel := filepath.Base(path)
	if r, err := filepath.Rel(l.root, path); err == nil {
		rel = r
	}
	l.put(Entry{Name: rel, Path: path, Kind: kind})
}

// walk skips subtrees it cannot read so one unreadable directory does not empty
// the whole listing. Only an unreadable root is fatal, matching readDir.
func (l *lister) walk() error {
	return filepath.WalkDir(l.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == l.root {
				return err
			}
			return nil
		}
		if d.IsDir() {
			if path != l.root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if kind, ok := l.classify(path); ok {
			l.add(path, kind)
		}
		return nil
	})
}

func (l *lister) readDir() error {
	entries, err := os.ReadDir(l.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(l.root, entry.Name())
		if kind, ok := l.classify(path); ok {
			l.add(path, kind)
		}
	}
	return nil
}

func (l *lister) sorted() []Entry {
	entries := make([]Entry, 0, len(l.byPath))
	for _, entry := range l.byPath {
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Path, b.Path))
	})
	return entries
}

func explicitWorkspaceEnvEntry(root, envFile string) (Entry, bool) {
	envFile = strings.TrimSpace(envFile)
	if envFile == "" {
		return Entry{}, false
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = filepath.Clean(root)
	}
	envAbs, err := filepath.Abs(envFile)
	if err != nil {
		envAbs = filepath.Clean(envFile)
	}

	info, err := os.Stat(envAbs)
	if err != nil || info.IsDir() {
		return Entry{}, false
	}

	rel, err := filepath.Rel(rootAbs, envAbs)
	if err != nil {
		return Entry{}, false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Entry{}, false
	}

	return Entry{
		Name: rel,
		Path: filepath.Join(root, rel),
		Kind: KindEnv,
	}, true
}
