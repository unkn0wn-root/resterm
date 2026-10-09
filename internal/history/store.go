package history

import (
	"path/filepath"
	"runtime"
	"strings"
	"time"
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

type MaintenanceStore interface {
	Store
	Stats() (Stats, error)
	Check(full bool) error
	Compact() error
	Backup(path string) error
	ExportJSON(path string) (int, error)
	ImportJSON(path string) (int, error)
	MigrateJSON(path string) (int, error)
}

type Stats struct {
	Path     string
	Schema   int
	Rows     int64
	Oldest   time.Time
	Newest   time.Time
	DBBytes  int64
	WALBytes int64
	SHMBytes int64
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
