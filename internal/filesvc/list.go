package filesvc

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type FileEntry struct {
	Name string
	Path string
}

func ListRequestFiles(root string, recursive bool) ([]FileEntry, error) {
	var entries []FileEntry
	exts := map[string]struct{}{".http": {}, ".rest": {}}
	if recursive {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && path != root {
					return filepath.SkipDir
				}
				return nil
			}

			if _, ok := exts[strings.ToLower(filepath.Ext(d.Name()))]; ok {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = d.Name()
				}
				entries = append(entries, FileEntry{Name: rel, Path: path})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		dirEntries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, entry := range dirEntries {
			if entry.IsDir() {
				continue
			}
			if _, ok := exts[strings.ToLower(filepath.Ext(entry.Name()))]; ok {
				path := filepath.Join(root, entry.Name())
				entries = append(entries, FileEntry{Name: entry.Name(), Path: path})
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}
