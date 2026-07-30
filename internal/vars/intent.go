package vars

import (
	"maps"
	"slices"
	"strings"
)

// Intent is the environment the session asked for, before any catalog resolved
// it. A resolved Selection already carries the defaults its own catalog filled
// in, so it cannot be replayed against another one.
type Intent struct {
	Name   string
	Groups map[string]string
}

func (i Intent) Describe() string {
	if i.Name != "" {
		return i.Name
	}
	if len(i.Groups) == 0 {
		return "the previous selection"
	}

	parts := make([]string, 0, len(i.Groups))
	for _, g := range slices.Sorted(maps.Keys(i.Groups)) {
		parts = append(parts, g+"="+i.Groups[g])
	}
	return strings.Join(parts, ", ")
}

// Resolve replays the intent against cat and reports false when cat does not
// have what the intent names.
func (i Intent) Resolve(cat Catalog) (Selection, bool) {
	sel, err := cat.Select(i.Name, i.Groups)
	if err != nil {
		return Selection{}, false
	}
	return sel, true
}

// IntentFor turns an applied selection back into an intent, so a later workspace
// change replays the picker choice rather than the command line one.
func (c Catalog) IntentFor(env Environment) Intent {
	if c.Grouped() {
		return Intent{Groups: maps.Clone(env.Selection().Groups())}
	}
	return Intent{Name: env.Selection().Name()}
}
