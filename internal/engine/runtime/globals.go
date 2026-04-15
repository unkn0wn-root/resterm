package runtime

import (
	"cmp"
	"strings"
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
	store scopedValueStore
}

func NewGlobals() *Globals {
	return &Globals{store: newScopedValueStore()}
}

func (s *Globals) Snapshot(env string) map[string]GlobalValue {
	if s == nil {
		return nil
	}

	src := s.store.snapshot(envKey(env))
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]GlobalValue, len(src))
	for k, v := range src {
		dst[k] = GlobalValue(v)
	}
	return dst
}

func (s *Globals) Set(env, name, value string, secret bool) {
	if s == nil {
		return
	}
	s.store.set(envKey(env), name, value, secret)
}

func (s *Globals) Delete(env, name string) {
	if s == nil {
		return
	}
	s.store.delete(envKey(env), name)
}

func (s *Globals) Clear(env string) {
	if s == nil {
		return
	}
	s.store.clear(envKey(env))
}

func (s *Globals) Entries() []engine.RuntimeGlobal {
	if s == nil {
		return nil
	}

	return storeEntries(&s.store, func(env string, x storedValue) engine.RuntimeGlobal {
		return engine.RuntimeGlobal{
			Env:       stateEnv(env),
			Name:      x.Name,
			Value:     x.Value,
			Secret:    x.Secret,
			UpdatedAt: x.UpdatedAt,
		}
	}, func(a, b engine.RuntimeGlobal) int {
		if n := cmp.Compare(a.Env, b.Env); n != 0 {
			return n
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

func (s *Globals) Restore(xs []engine.RuntimeGlobal) {
	if s == nil {
		return
	}

	restoreStore(&s.store, xs, func(x engine.RuntimeGlobal) string {
		return envKey(x.Env)
	}, func(x engine.RuntimeGlobal) (storedValue, bool) {
		name := strings.TrimSpace(x.Name)
		if name == "" {
			return storedValue{}, false
		}
		return storedValue{
			Name:      name,
			Value:     x.Value,
			Secret:    x.Secret,
			UpdatedAt: x.UpdatedAt,
		}, true
	})
}
