package ui

import (
	"net/http"
	"net/http/cookiejar"
	"sync"
)

type cookieStore struct {
	mu   sync.RWMutex
	jars map[string]http.CookieJar
}

func newCookieStore() *cookieStore {
	return &cookieStore{
		jars: make(map[string]http.CookieJar),
	}
}

func (s *cookieStore) getOrCreate(env string) http.CookieJar {
	s.mu.RLock()
	jar, exists := s.jars[env]
	s.mu.RUnlock()

	if exists {
		return jar
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	jar, _ = cookiejar.New(nil)
	s.jars[env] = jar
	return jar
}

func (s *cookieStore) clear(env string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jars, env)
}
