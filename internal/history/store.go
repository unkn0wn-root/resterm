package history

import (
	"path/filepath"
	"runtime"
	"strings"
)

const InitCap = 64

type Store interface {
	Load() error
	Append(Entry) error
	Entries() []Entry
	ByRequest(string) []Entry
	ByWorkflow(string) []Entry
	ByFile(string) []Entry
	Delete(string) (bool, error)
	Close() error
}

func NormalizeWorkflowName(name string) string {
	return strings.TrimSpace(name)
}

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
