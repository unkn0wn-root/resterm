package ui

import (
	"strings"
	"sync"
	"time"
)

type globalValue struct {
	Name      string
	Value     string
	Secret    bool
	UpdatedAt time.Time
}

type globalStore struct {
	mu     sync.RWMutex
	values map[string]map[string]globalValue
}

func newGlobalStore() *globalStore {
	return &globalStore{values: make(map[string]map[string]globalValue)}
}

func (s *globalStore) snapshot(env string) map[string]globalValue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	envKey := normalizeEnvKey(env)
	entries := s.values[envKey]
	if len(entries) == 0 {
		return nil
	}

	clone := make(map[string]globalValue, len(entries))
	for k, v := range entries {
		clone[k] = v
	}
	return clone
}

func (s *globalStore) set(env, name, value string, secret bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	envKey := normalizeEnvKey(env)
	if s.values[envKey] == nil {
		s.values[envKey] = make(map[string]globalValue)
	}
	s.values[envKey][normalizeNameKey(name)] = globalValue{
		Name:      strings.TrimSpace(name),
		Value:     value,
		Secret:    secret,
		UpdatedAt: time.Now(),
	}
}

func (s *globalStore) delete(env, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	envKey := normalizeEnvKey(env)
	entries := s.values[envKey]
	if len(entries) == 0 {
		return
	}
	delete(entries, normalizeNameKey(name))
	if len(entries) == 0 {
		delete(s.values, envKey)
	}
}

func (s *globalStore) clear(env string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.values, normalizeEnvKey(env))
}

func normalizeEnvKey(name string) string {
	if strings.TrimSpace(name) == "" {
		return "__default__"
	}
	return strings.ToLower(strings.TrimSpace(name))
}

func normalizeNameKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
