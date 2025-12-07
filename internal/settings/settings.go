package settings

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Matcher func(string) bool
type ApplyFunc func(key, val string) error

type Handler struct {
	Match Matcher
	Apply ApplyFunc
}

type Applier struct {
	handlers []Handler
}

func New(handlers ...Handler) Applier {
	return Applier{handlers: handlers}
}

func (a Applier) ApplyAll(settings map[string]string) (map[string]string, error) {
	if len(settings) == 0 || len(a.handlers) == 0 {
		return settings, nil
	}
	left := make(map[string]string)
	for k, v := range settings {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		applied := false
		for _, h := range a.handlers {
			if h.Match != nil && h.Match(key) {
				if h.Apply != nil {
					if err := h.Apply(key, v); err != nil {
						return nil, err
					}
				}
				applied = true
				break
			}
		}
		if !applied {
			left[key] = v
		}
	}
	return left, nil
}

func PrefixMatcher(prefixes ...string) Matcher {
	return func(key string) bool {
		lower := strings.ToLower(strings.TrimSpace(key))
		for _, p := range prefixes {
			if strings.HasPrefix(lower, strings.ToLower(strings.TrimSpace(p))) {
				return true
			}
		}
		return false
	}
}

func ExactMatcher(keys ...string) Matcher {
	return func(key string) bool {
		lower := strings.ToLower(strings.TrimSpace(key))
		for _, k := range keys {
			if lower == strings.ToLower(strings.TrimSpace(k)) {
				return true
			}
		}
		return false
	}
}

type ResolverProvider interface {
	Resolver() *vars.Resolver
}
