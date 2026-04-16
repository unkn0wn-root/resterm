package runtime

import (
	"net/http"
	"net/http/cookiejar"
	"sync"
)

// Cookies keeps in-memory cookie jars scoped by effective environment.
// Persistence across process restarts is intentionally out of scope here.
type Cookies struct {
	mu   sync.RWMutex
	jars map[string]http.CookieJar
}

func NewCookies() *Cookies {
	return &Cookies{jars: make(map[string]http.CookieJar)}
}

func (s *Cookies) Jar(env string) http.CookieJar {
	if s == nil {
		return nil
	}

	key := envKey(env)

	s.mu.RLock()
	jar := s.jars[key]
	s.mu.RUnlock()
	if jar != nil {
		return jar
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.jars == nil {
		s.jars = make(map[string]http.CookieJar)
	}
	if jar = s.jars[key]; jar != nil {
		return jar
	}
	jar, _ = cookiejar.New(nil)
	s.jars[key] = jar
	return jar
}

func (s *Cookies) Clear(env string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.jars, envKey(env))
}
