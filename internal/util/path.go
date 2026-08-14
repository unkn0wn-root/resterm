package util

import (
	"os"
	"path/filepath"
	"strings"
)

// SamePath reports whether two non-empty paths resolve to the same lexical path.
//
// It does not stat files or resolve symlinks; use os.SameFile with os.Stat
// results when physical file identity is required.
func SamePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}

	if a == b {
		return true
	}

	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	return absA == absB
}

// SameFile reports whether two paths name one file on disk. Use it instead of
// SamePath when the same file can reach the caller under more than one name, as
// happens through a symlink or on a case insensitive filesystem.
func SameFile(a, b string) bool {
	if SamePath(a, b) {
		return true
	}
	if a == "" || b == "" {
		return false
	}

	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}

func ExpandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	rest := strings.TrimPrefix(path[1:], string(filepath.Separator))
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return home
	}
	return filepath.Join(home, rest)
}

// SamePathOrBothEmpty reports whether a and b are both empty or name the same path.
func SamePathOrBothEmpty(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	return SamePath(a, b)
}
