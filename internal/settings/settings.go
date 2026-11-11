package settings

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/str"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Matcher func(string) bool
type ApplyFunc func(key, val string) error

type Handler struct {
	// Component labels errors about the keys Match accepts.
	Component diag.Component
	Match     Matcher
	Apply     ApplyFunc
}

type Applier struct {
	handlers []Handler
}

func New(handlers ...Handler) Applier {
	return Applier{handlers: handlers}
}

// ApplyAll applies each setting through the first handler that matches its key
// and returns the settings no handler matched. settings must come from Merge, so
// its keys are canonical and both names of one option are left only when one
// scope wrote both. That fails the request.
func (a Applier) ApplyAll(settings map[string]string) (map[string]string, error) {
	if len(settings) == 0 || len(a.handlers) == 0 {
		return settings, nil
	}
	left := make(map[string]string)
	for _, key := range slices.Sorted(maps.Keys(settings)) {
		val := settings[key]
		i := slices.IndexFunc(a.handlers, func(h Handler) bool { return h.Match != nil && h.Match(key) })
		if i < 0 {
			left[key] = val
			continue
		}
		h := a.handlers[i]
		if other, ok := sameOption[key]; ok {
			if _, dup := settings[other]; dup {
				msg := fmt.Sprintf("%s and %s are the same setting. Use one of them.", key, other)
				return nil, diag.New(diag.ClassProtocol, msg, diag.WithComponent(h.Component))
			}
		}
		if h.Apply != nil {
			if err := h.Apply(key, val); err != nil {
				return nil, err
			}
		}
	}
	return left, nil
}

func PrefixMatcher(prefixes ...string) Matcher {
	return func(key string) bool {
		lower := str.LowerTrim(key)
		for _, p := range prefixes {
			if strings.HasPrefix(lower, str.LowerTrim(p)) {
				return true
			}
		}
		return false
	}
}

type ResolverProvider interface {
	Resolver() *vars.Resolver
}
