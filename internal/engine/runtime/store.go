package runtime

import (
	"slices"
	"strings"
	"sync"
	"time"
)

type storedValue struct {
	Name      string
	Value     string
	Secret    bool
	UpdatedAt time.Time
}

type scopedValueStore struct {
	mu     sync.RWMutex
	values map[string]map[string]storedValue
}

func newScopedValueStore() scopedValueStore {
	return scopedValueStore{values: make(map[string]map[string]storedValue)}
}

func (s *scopedValueStore) snapshot(scope string) map[string]storedValue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneStoredValues(s.values[scope])
}

func (s *scopedValueStore) set(scope, name, value string, secret bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.init()
	if s.values[scope] == nil {
		s.values[scope] = make(map[string]storedValue)
	}
	s.values[scope][nameKey(name)] = storedValue{
		Name:      strings.TrimSpace(name),
		Value:     value,
		Secret:    secret,
		UpdatedAt: time.Now(),
	}
}

func (s *scopedValueStore) delete(scope, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	src := s.values[scope]
	if len(src) == 0 {
		return
	}
	delete(src, nameKey(name))
	if len(src) == 0 {
		delete(s.values, scope)
	}
}

func (s *scopedValueStore) clear(scope string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.values, scope)
}

func (s *scopedValueStore) clearIf(match func(string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for scope := range s.values {
		if match(scope) {
			delete(s.values, scope)
		}
	}
}

func (s *scopedValueStore) init() {
	if s.values == nil {
		s.values = make(map[string]map[string]storedValue)
	}
}

func cloneStoredValues(src map[string]storedValue) map[string]storedValue {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]storedValue, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func storeEntries[T any](
	s *scopedValueStore,
	build func(scope string, v storedValue) T,
	compare func(a, b T) int,
) []T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]T, 0, len(s.values))
	for scope, xs := range s.values {
		for _, v := range xs {
			out = append(out, build(scope, v))
		}
	}
	slices.SortFunc(out, compare)
	return out
}

func restoreStore[T any](
	s *scopedValueStore,
	xs []T,
	scopeOf func(T) string,
	valueOf func(T) (storedValue, bool),
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.values = make(map[string]map[string]storedValue)
	for _, x := range xs {
		v, ok := valueOf(x)
		if !ok {
			continue
		}
		scope := scopeOf(x)
		if s.values[scope] == nil {
			s.values[scope] = make(map[string]storedValue)
		}
		s.values[scope][nameKey(v.Name)] = v
	}
}
