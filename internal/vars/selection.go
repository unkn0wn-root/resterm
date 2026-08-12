package vars

import (
	"errors"
	"maps"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

// defaultEnvNames is the preference order for picking an environment when the
// caller did not name one.
var defaultEnvNames = [...]string{"dev", "default", "local"}

// ErrUnknownSelection marks a name that no longer resolves against the catalog.
// Callers that recover from a stale selection, such as history replay, match it
// with errors.Is rather than assuming Select is their only failing call.
var ErrUnknownSelection = errors.New("unknown environment selection")

// unknownError carries a diagnostic that also matches ErrUnknownSelection. The
// sentinel stays out of the message so callers keep the specific text.
type unknownError struct{ err error }

func (u unknownError) Error() string        { return u.err.Error() }
func (u unknownError) Unwrap() error        { return u.err }
func (u unknownError) Is(target error) bool { return target == ErrUnknownSelection }

func errUnknown(format string, args ...any) error {
	return unknownError{err: diag.Newf(diag.ClassParse, format, args...)}
}

// Selection is a resolved choice of environment: either a single name or one
// profile per group. Groups returns the internal map, which callers must not
// mutate.
type Selection struct {
	name     string
	profiles map[string]string
}

// Environment is an immutable resolved environment. Accessors return internal
// maps that callers must not mutate.
type Environment struct {
	values map[string]string
	label  string
	scope  string
	sel    Selection
}

func GroupSelection(profiles map[string]string) Selection {
	return Selection{profiles: str.CloneMap(profiles)}
}

func (c Catalog) DefaultSelection() Selection {
	if !c.Grouped() {
		for _, want := range defaultEnvNames {
			if env, ok := c.findEnv(want); ok {
				return Selection{name: env.name}
			}
		}
		if len(c.envs) > 0 {
			return Selection{name: c.envs[0].name}
		}
		return Selection{}
	}

	sel := Selection{profiles: make(map[string]string, len(c.groups))}
	for _, g := range c.groups {
		sel.profiles[g.Name] = g.Default
	}
	return sel
}

func (c Catalog) Select(name string, profiles map[string]string) (Selection, error) {
	if !c.Grouped() {
		if len(profiles) > 0 {
			return Selection{}, diag.New(
				diag.ClassParse,
				"group selection requires grouped environments",
			)
		}
		name = str.Trim(name)
		if name == "" {
			return c.DefaultSelection(), nil
		}
		env, ok := c.findEnv(name)
		if !ok {
			if len(c.envs) == 0 {
				return Selection{name: name}, nil
			}
			return Selection{}, errUnknown("unknown environment %q", name)
		}
		return Selection{name: env.name}, nil
	}

	if str.Trim(name) != "" {
		return Selection{}, diag.New(
			diag.ClassParse,
			"environment name cannot be combined with grouped environments",
		)
	}
	sel := c.DefaultSelection()
	seen := newNames("selected group", len(profiles))
	for rawGroup, rawProfile := range profiles {
		gname, err := seen.add(rawGroup)
		if err != nil {
			return Selection{}, err
		}
		g, ok := c.findGroup(gname)
		if !ok {
			return Selection{}, errUnknown("unknown environment group %q", gname)
		}
		p, ok := mapName(g.Profiles, rawProfile)
		if !ok {
			return Selection{}, errUnknown(
				"unknown profile %q in group %q",
				str.Trim(rawProfile),
				g.Name,
			)
		}
		sel.profiles[g.Name] = p
	}
	return sel, nil
}

func (s Selection) WithGroup(group, profile string) Selection {
	out := Selection{profiles: make(map[string]string, len(s.profiles)+1)}
	maps.Copy(out.profiles, s.profiles)
	name := str.Trim(group)
	if g, ok := mapName(out.profiles, name); ok {
		name = g
	}
	out.profiles[name] = str.Trim(profile)
	return out
}

func (s Selection) Name() string {
	return s.name
}

func (s Selection) Groups() map[string]string {
	return s.profiles
}

func (s Selection) Profile(group string) (string, bool) {
	if g, ok := mapName(s.profiles, group); ok {
		return s.profiles[g], true
	}
	return "", false
}

func (s Selection) Empty() bool {
	return s.name == "" && len(s.profiles) == 0
}

func (c Catalog) Resolve(sel Selection) (Environment, error) {
	next, err := c.Select(sel.name, sel.profiles)
	if err != nil {
		return Environment{}, err
	}

	if !c.Grouped() {
		var values map[string]string
		if env, ok := c.findEnv(next.name); ok {
			values = collapseNames(env.values)
		}
		return Environment{
			values: values,
			label:  next.name,
			scope:  flatScope(next.name, c.source),
			sel:    next,
		}, nil
	}

	layers := make([]map[string]string, 0, len(c.groups)+1)
	layers = append(layers, c.shared)
	parts := make([]string, 0, len(c.groups))
	for _, g := range c.groups {
		p := next.profiles[g.Name]
		layers = append(layers, g.Profiles[p])
		parts = append(parts, g.Name+"="+p)
	}
	return Environment{
		values: mergeValues(layers...),
		label:  strings.Join(parts, ", "),
		scope:  groupScope(c.groups, next, c.source),
		sel:    next,
	}, nil
}

func (e Environment) Values() map[string]string {
	return e.values
}

func (e Environment) Label() string {
	return e.label
}

func (e Environment) Scope() string {
	return e.scope
}

func (e Environment) Selection() Selection {
	return e.sel
}
