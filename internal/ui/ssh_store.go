package ui

import (
	"strings"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type sshStore struct {
	mu     sync.RWMutex
	byPath map[string]map[string]restfile.SSHProfile
	cache  map[string]restfile.SSHProfile
}

func newSSHStore() *sshStore {
	return &sshStore{
		byPath: make(map[string]map[string]restfile.SSHProfile),
		cache:  make(map[string]restfile.SSHProfile),
	}
}

func (s *sshStore) set(path string, profiles []restfile.SSHProfile) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := strings.ToLower(strings.TrimSpace(path))
	old := s.byPath[p]
	for name := range old {
		delete(s.cache, name)
	}

	next := make(map[string]restfile.SSHProfile)
	for _, prof := range profiles {
		if prof.Scope != restfile.SSHScopeGlobal {
			continue
		}
		name := normalizeNameKey(prof.Name)
		if name == "" {
			name = "default"
		}
		next[name] = prof
		s.cache[name] = prof
	}

	if len(next) == 0 {
		delete(s.byPath, p)
		return
	}
	s.byPath[p] = next
}

func (s *sshStore) all() []restfile.SSHProfile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.cache) == 0 {
		return nil
	}
	out := make([]restfile.SSHProfile, 0, len(s.cache))
	for _, prof := range s.cache {
		out = append(out, prof)
	}
	return out
}
