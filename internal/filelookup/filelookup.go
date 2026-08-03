// Package filelookup keeps request file resolution consistent across protocol clients.
package filelookup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/unkn0wn-root/resterm/internal/util"
)

// FileSystem provides the file reads used during lookup.
type FileSystem interface {
	ReadFile(name string) ([]byte, error)
}

type OSFileSystem struct{}

func (OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

type Lookup struct {
	BaseDir   string
	Fallbacks []string
	// AllowRaw tries the original path after all rooted candidates.
	AllowRaw bool
}

// For builds a lookup from request file settings. With no base directory, the
// original path remains the only candidate even when fallbacks are disabled.
func For(baseDir string, fallbacks []string, noFallback bool) Lookup {
	if noFallback {
		return Lookup{BaseDir: baseDir, AllowRaw: baseDir == ""}
	}
	return Lookup{BaseDir: baseDir, Fallbacks: fallbacks, AllowRaw: true}
}

// Read returns the first readable candidate and the path used to find it.
func (l Lookup) Read(fsys FileSystem, path string) ([]byte, string, error) {
	if filepath.IsAbs(path) {
		data, err := fsys.ReadFile(path)
		if err != nil {
			return nil, path, err
		}
		return data, path, nil
	}

	var (
		lastErr  error
		lastPath = path
	)
	for _, candidate := range l.Candidates(path) {
		data, err := fsys.ReadFile(candidate)
		if err == nil {
			return data, candidate, nil
		}
		if Fatal(err) {
			return nil, candidate, err
		}
		lastErr = err
		lastPath = candidate
	}

	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return nil, lastPath, lastErr
}

func (l Lookup) Candidates(path string) []string {
	out := make([]string, 0, 2+len(l.Fallbacks))
	if l.BaseDir != "" {
		out = append(out, filepath.Join(l.BaseDir, path))
	}
	for _, fb := range l.Fallbacks {
		if fb != "" {
			out = append(out, filepath.Join(fb, path))
		}
	}
	if l.AllowRaw {
		out = append(out, path)
	}
	return util.DedupeNonEmptyStrings(out)
}

// Fatal reports whether a lookup should stop instead of trying another root.
// Permission and directory errors identify the intended file and are returned
// directly.
func Fatal(err error) bool {
	return isPerm(err) || isDir(err) || errors.Is(err, os.ErrInvalid)
}

func isPerm(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, fs.ErrPermission)
}

func isDir(err error) bool {
	if errors.Is(err, syscall.EISDIR) {
		return true
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && errors.Is(pe.Err, syscall.EISDIR) {
		return true
	}
	return false
}
