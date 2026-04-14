package runtime

import (
	"cmp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
)

type GlobalValue struct {
	Name      string
	Value     string
	Secret    bool
	UpdatedAt time.Time
}

type Globals struct {
	mu     sync.RWMutex
	values map[string]map[string]GlobalValue
}

func NewGlobals() *Globals {
	return &Globals{values: make(map[string]map[string]GlobalValue)}
}

func (s *Globals) Snapshot(env string) map[string]GlobalValue {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	envKey := envKey(env)
	src := s.values[envKey]
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]GlobalValue, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (s *Globals) Set(env, name, value string, secret bool) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := envKey(env)
	if s.values[key] == nil {
		s.values[key] = make(map[string]GlobalValue)
	}
	s.values[key][nameKey(name)] = GlobalValue{
		Name:      strings.TrimSpace(name),
		Value:     value,
		Secret:    secret,
		UpdatedAt: time.Now(),
	}
}

func (s *Globals) Delete(env, name string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := envKey(env)
	src := s.values[key]
	if len(src) == 0 {
		return
	}
	delete(src, nameKey(name))
	if len(src) == 0 {
		delete(s.values, key)
	}
}

func (s *Globals) Clear(env string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.values, envKey(env))
}

func (s *Globals) Entries() []engine.RuntimeGlobal {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]engine.RuntimeGlobal, 0, len(s.values))
	for env, xs := range s.values {
		for _, x := range xs {
			out = append(out, engine.RuntimeGlobal{
				Env:       stateEnv(env),
				Name:      x.Name,
				Value:     x.Value,
				Secret:    x.Secret,
				UpdatedAt: x.UpdatedAt,
			})
		}
	}
	slices.SortFunc(out, func(a, b engine.RuntimeGlobal) int {
		if n := cmp.Compare(a.Env, b.Env); n != 0 {
			return n
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}

func (s *Globals) Restore(xs []engine.RuntimeGlobal) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.values = make(map[string]map[string]GlobalValue)
	for _, x := range xs {
		name := strings.TrimSpace(x.Name)
		if name == "" {
			continue
		}
		env := envKey(x.Env)
		if s.values[env] == nil {
			s.values[env] = make(map[string]GlobalValue)
		}
		s.values[env][nameKey(name)] = GlobalValue{
			Name:      name,
			Value:     x.Value,
			Secret:    x.Secret,
			UpdatedAt: x.UpdatedAt,
		}
	}
}
