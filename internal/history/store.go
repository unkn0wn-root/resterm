package history

import (
	"path/filepath"
	"runtime"
	"strings"
)

type Store interface {
	Load() error
	Append(Entry) error
	Entries() ([]Entry, error)
	ByRequest(string) ([]Entry, error)
	ByWorkflow(string) ([]Entry, error)
	ByFile(string) ([]Entry, error)
	Delete(string) (bool, error)
	Close() error
}

// NormPath returns the key that ByFile matches on.
// The sqlite store saves it with every row, so changing this breaks lookups of older history.
func NormPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	n := filepath.Clean(p)
	if n == "." {
		return ""
	}
	if runtime.GOOS == "windows" {
		n = strings.ToLower(n)
	}
	return n
}
