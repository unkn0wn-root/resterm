package ui

import (
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type fileVariable struct {
	Name      string
	Value     string
	Secret    bool
	UpdatedAt time.Time
}

type fileStore struct {
	mu     sync.RWMutex
	values map[string]map[string]fileVariable
}

func newFileStore() *fileStore {
	return &fileStore{values: make(map[string]map[string]fileVariable)}
}

func (s *fileStore) snapshot(env, path string) map[string]fileVariable {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fileStoreKey(env, path)
	entries := s.values[key]
	if len(entries) == 0 {
		return nil
	}

	clone := make(map[string]fileVariable, len(entries))
	for k, v := range entries {
		clone[k] = v
	}
	return clone
}

func (s *fileStore) set(env, path, name, value string, secret bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fileStoreKey(env, path)
	if s.values[key] == nil {
		s.values[key] = make(map[string]fileVariable)
	}

	normalized := normalizeNameKey(name)
	s.values[key][normalized] = fileVariable{
		Name:      strings.TrimSpace(name),
		Value:     value,
		Secret:    secret,
		UpdatedAt: time.Now(),
	}
}

func (s *fileStore) delete(env, path, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := fileStoreKey(env, path)
	entries := s.values[key]
	if len(entries) == 0 {
		return
	}

	delete(entries, normalizeNameKey(name))
	if len(entries) == 0 {
		delete(s.values, key)
	}
}

func (s *fileStore) clear(env, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.values, fileStoreKey(env, path))
}

func (s *fileStore) clearEnv(env string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix := normalizeEnvKey(env) + "|"
	for key := range s.values {
		if strings.HasPrefix(key, prefix) {
			delete(s.values, key)
		}
	}
}

func fileStoreKey(env, path string) string {
	envKey := normalizeEnvKey(env)
	pathKey := strings.TrimSpace(path)
	if pathKey == "" {
		pathKey = "__scratch__"
	}
	pathKey = strings.ToLower(filepath.Clean(pathKey))
	return envKey + "|" + pathKey
}
