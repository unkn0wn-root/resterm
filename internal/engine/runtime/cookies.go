package runtime

import (
	"net/http"
	"net/http/cookiejar"
	"sync"
)

type Cookies struct {
	mu   sync.RWMutex
	jars map[string]http.CookieJar
}

func NewCookies() *Cookies {
	return &Cookies{
		jars: make(map[string]http.CookieJar),
	}
}

func (s *Cookies) GetOrCreate(env string) http.CookieJar {
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

func (s *Cookies) Clear(env string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.jars, env)
}
