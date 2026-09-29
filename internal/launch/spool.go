package launch

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type Spool struct {
	parent string

	mu     sync.Mutex
	dir    string
	closed bool
}

func NewSpool(parent string) *Spool {
	return &Spool{parent: parent}
}

// Each file gets its own directory so it keeps its exact name.
func (s *Spool) Write(name string, data []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", errors.New("spool is closed")
	}
	if s.dir == "" {
		dir, err := os.MkdirTemp(s.parent, "resterm-")
		if err != nil {
			return "", err
		}
		s.dir = dir
	}

	dir, err := os.MkdirTemp(s.dir, "")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Spool) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.dir == "" {
		return nil
	}
	return os.RemoveAll(s.dir)
}
