package ui

import (
	"strings"
	"sync"
)

type namedStore[T any] struct {
	mu sync.RWMutex
	by map[string]map[string]T
	ca map[string]T
	ok func(T) bool
	nm func(T) string
}

func newNamedStore[T any](ok func(T) bool, nm func(T) string) *namedStore[T] {
	return &namedStore[T]{
		by: make(map[string]map[string]T),
		ca: make(map[string]T),
		ok: ok,
		nm: nm,
	}
}

func (s *namedStore[T]) set(p string, xs []T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := strings.ToLower(strings.TrimSpace(p))
	nx := make(map[string]T)
	for _, x := range xs {
		if s.ok != nil && !s.ok(x) {
			continue
		}
		n := "default"
		if s.nm != nil {
			if v := normalizeNameKey(s.nm(x)); v != "" {
				n = v
			}
		}
		nx[n] = x
	}

	if len(nx) == 0 {
		delete(s.by, k)
	} else {
		s.by[k] = nx
	}
	ca := make(map[string]T)
	for _, pm := range s.by {
		for n, x := range pm {
			ca[n] = x
		}
	}
	s.ca = ca
}

func (s *namedStore[T]) all() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.ca) == 0 {
		return nil
	}
	out := make([]T, 0, len(s.ca))
	for _, x := range s.ca {
		out = append(out, x)
	}
	return out
}
