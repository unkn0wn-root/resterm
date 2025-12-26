package filesvc

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	extHTTP = ".http"
	extREST = ".rest"
	extRTS  = ".rts"
)

type FileEntry struct {
	Name string
	Path string
}

func IsRequestFile(path string) bool {
	ext := fileExt(path)
	return ext == extHTTP || ext == extREST
}

func IsRTSFile(path string) bool {
	return fileExt(path) == extRTS
}

func IsSupportedFile(path string) bool {
	ext := fileExt(path)
	switch ext {
	case extHTTP, extREST, extRTS:
		return true
	default:
		return false
	}
}

func ListRequestFiles(root string, recursive bool) ([]FileEntry, error) {
	var entries []FileEntry
	include := func(name string) bool {
		return IsSupportedFile(name)
	}

	appendEntry := func(name, path string) {
		entries = append(entries, FileEntry{Name: name, Path: path})
	}

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
			if !include(d.Name()) {
				return nil
			}
			rel := d.Name()
			if r, relErr := filepath.Rel(root, path); relErr == nil {
				rel = r
			}
			appendEntry(rel, path)
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
			if entry.IsDir() || !include(entry.Name()) {
				continue
			}
			appendEntry(entry.Name(), filepath.Join(root, entry.Name()))
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

func fileExt(path string) string {
	return strings.ToLower(filepath.Ext(path))
}
