package runtime

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
)

type FileValue struct {
	Name      string
	Value     string
	Secret    bool
	UpdatedAt time.Time
}

type Files struct {
	mu     sync.RWMutex
	values map[string]map[string]FileValue
}

func NewFiles() *Files {
	return &Files{values: make(map[string]map[string]FileValue)}
}

func (s *Files) Snapshot(env, path string) map[string]FileValue {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := fileKey(env, path)
	src := s.values[key]
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]FileValue, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (s *Files) Set(env, path, name, value string, secret bool) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := fileKey(env, path)
	if s.values[key] == nil {
		s.values[key] = make(map[string]FileValue)
	}
	s.values[key][nameKey(name)] = FileValue{
		Name:      strings.TrimSpace(name),
		Value:     value,
		Secret:    secret,
		UpdatedAt: time.Now(),
	}
}

func (s *Files) ClearEnv(env string) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	pfx := envKey(env) + "|"
	for key := range s.values {
		if strings.HasPrefix(key, pfx) {
			delete(s.values, key)
		}
	}
}

func (s *Files) Entries() []engine.RuntimeFile {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]engine.RuntimeFile, 0, len(s.values))
	for key, xs := range s.values {
		env, path := splitFileKey(key)
		for _, x := range xs {
			out = append(out, engine.RuntimeFile{
				Env:       env,
				Path:      path,
				Name:      x.Name,
				Value:     x.Value,
				Secret:    x.Secret,
				UpdatedAt: x.UpdatedAt,
			})
		}
	}
	slices.SortFunc(out, func(a, b engine.RuntimeFile) int {
		if n := cmp.Compare(a.Env, b.Env); n != 0 {
			return n
		}
		if n := cmp.Compare(a.Path, b.Path); n != 0 {
			return n
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out
}

func (s *Files) Restore(xs []engine.RuntimeFile) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.values = make(map[string]map[string]FileValue)
	for _, x := range xs {
		name := strings.TrimSpace(x.Name)
		if name == "" {
			continue
		}
		key := fileKey(x.Env, x.Path)
		if s.values[key] == nil {
			s.values[key] = make(map[string]FileValue)
		}
		s.values[key][nameKey(name)] = FileValue{
			Name:      name,
			Value:     x.Value,
			Secret:    x.Secret,
			UpdatedAt: x.UpdatedAt,
		}
	}
}

func fileKey(env, path string) string {
	key := strings.TrimSpace(path)
	if key == "" {
		key = "__scratch__"
	}
	return envKey(env) + "|" + strings.ToLower(filepath.Clean(key))
}

func splitFileKey(key string) (string, string) {
	env, path, ok := strings.Cut(key, "|")
	if !ok {
		return "", key
	}
	return stateEnv(env), path
}
