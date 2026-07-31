package runtime

import (
	"cmp"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
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
	store scopedValueStore
}

func NewFiles() *Files {
	return &Files{store: newScopedValueStore()}
}

func (s *Files) Snapshot(env, path string) map[string]FileValue {
	if s == nil {
		return nil
	}

	src := s.store.snapshot(fileKey(env, path))
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]FileValue, len(src))
	for k, v := range src {
		dst[k] = FileValue(v)
	}
	return dst
}

func (s *Files) Set(env, path, name, value string, secret bool) {
	if s == nil {
		return
	}
	s.store.set(fileKey(env, path), name, value, secret)
}

func (s *Files) ClearEnv(env string) {
	if s == nil {
		return
	}
	pfx := fileEnvPrefix(env)
	s.store.clearIf(func(scope string) bool { return strings.HasPrefix(scope, pfx) })
}

func (s *Files) clearIf(match func(env string) bool) {
	s.store.clearIf(func(key string) bool {
		env, _ := splitFileKey(key)
		return match(env)
	})
}

func (s *Files) Entries() []engine.RuntimeFile {
	if s == nil {
		return nil
	}

	return storeEntries(&s.store, func(key string, x storedValue) engine.RuntimeFile {
		env, path := splitFileKey(key)
		return engine.RuntimeFile{
			Env:       env,
			Path:      path,
			Name:      x.Name,
			Value:     x.Value,
			Secret:    x.Secret,
			UpdatedAt: x.UpdatedAt,
		}
	}, func(a, b engine.RuntimeFile) int {
		if n := cmp.Compare(a.Env, b.Env); n != 0 {
			return n
		}
		if n := cmp.Compare(a.Path, b.Path); n != 0 {
			return n
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

func (s *Files) Restore(xs []engine.RuntimeFile) {
	if s == nil {
		return
	}

	restoreStore(&s.store, xs, func(x engine.RuntimeFile) string {
		return fileKey(x.Env, x.Path)
	}, func(x engine.RuntimeFile) (storedValue, bool) {
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

// Length prefixed so the split stays exact when a name contains the separator.
func fileEnvPrefix(env string) string {
	e := envKey(env)
	return fmt.Sprintf("%d:%s|", len(e), e)
}

func fileKey(env, path string) string {
	key := strings.TrimSpace(path)
	if key == "" {
		key = "__scratch__"
	}
	return fileEnvPrefix(env) + strings.ToLower(filepath.Clean(key))
}

func splitFileKey(key string) (string, string) {
	head, rest, ok := strings.Cut(key, ":")
	if !ok {
		return "", key
	}
	n, err := strconv.Atoi(head)
	if err != nil || n < 0 || n >= len(rest) || rest[n] != '|' {
		return "", key
	}
	return stateEnv(rest[:n]), rest[n+1:]
}
