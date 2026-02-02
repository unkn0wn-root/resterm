package initcmd

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type TempFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

type FS interface {
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
	CreateTemp(string, string) (TempFile, error)
	Rename(string, string) error
	Remove(string) error
}

type OSFS struct{}

func (OSFS) Stat(p string) (fs.FileInfo, error) { return os.Stat(p) }
func (OSFS) MkdirAll(p string, m fs.FileMode) error {
	return os.MkdirAll(p, m)
}
func (OSFS) ReadFile(p string) ([]byte, error) { return os.ReadFile(p) }
func (OSFS) WriteFile(p string, b []byte, m fs.FileMode) error {
	return os.WriteFile(p, b, m)
}
func (OSFS) CreateTemp(d, pat string) (TempFile, error) { return os.CreateTemp(d, pat) }
func (OSFS) Rename(a, b string) error                   { return os.Rename(a, b) }
func (OSFS) Remove(p string) error                      { return os.Remove(p) }

func normalizeTemplatePath(path string) string {
	return filepath.FromSlash(strings.TrimSpace(path))
}

func safeJoin(baseDir, rel string) (string, error) {
	rel = filepath.Clean(rel)
	if rel == "" || rel == "." || rel == ".." {
		return "", fmt.Errorf("init: invalid template path %q", rel)
	}
	if filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("init: invalid template path %q", rel)
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("init: invalid template path %q", rel)
	}
	return filepath.Join(baseDir, rel), nil
}
