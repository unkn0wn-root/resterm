package vars

import (
	"maps"
	"slices"
)

// ResolvedEnv holds environment values and the OS lookups shared by one run.
// The zero value is not usable.
type ResolvedEnv struct {
	env      Environment
	refs     *EnvRefs
	vals     NameMap[Value]
	plain    map[string]string
	refNames []string
}

// Resolve creates a snapshot for callers without document declarations.
func (e Environment) Resolve() ResolvedEnv {
	return e.ResolveWith(NewEnvRefs(nil))
}

// ResolveWith creates a snapshot that shares refs with the rest of the run.
func (e Environment) ResolveWith(refs *EnvRefs) ResolvedEnv {
	out := ResolvedEnv{
		env:   e,
		refs:  refs,
		plain: make(map[string]string, len(e.values)),
	}
	for _, name := range slices.Sorted(maps.Keys(e.values)) {
		raw := e.values[name]
		if _, ok := EnvRefKey(raw); ok {
			out.refNames = append(out.refNames, name)
		}
		v := refs.Declared(raw)
		out.vals.Set(name, v)
		if !v.Missing {
			out.plain[name] = v.Text
		}
	}
	return out
}

// Refs returns the snapshot shared by this environment.
func (r ResolvedEnv) Refs() *EnvRefs {
	return r.refs
}

// Values returns resolved environment values. Callers must not modify the map.
func (r ResolvedEnv) Values() map[string]string {
	return r.plain
}

// ProviderValues returns resolver inputs, including references with no value.
func (r ResolvedEnv) ProviderValues() NameMap[Value] {
	return r.vals
}

// Secrets returns every OS value read by this environment.
func (r ResolvedEnv) Secrets() []string {
	return r.refs.Secrets()
}

// WithoutRefValues hides referenced values while preserving their precedence.
func (r ResolvedEnv) WithoutRefValues() ResolvedEnv {
	if r.refs.withheld {
		return r
	}
	r.refs = r.refs.Withheld()
	if len(r.refNames) == 0 {
		return r
	}
	r.vals = r.vals.Clone()
	r.plain = maps.Clone(r.plain)
	for _, name := range r.refNames {
		r.vals.Set(name, Value{Missing: true})
		delete(r.plain, name)
	}
	return r
}

func (r ResolvedEnv) Label() string {
	return r.env.label
}

func (r ResolvedEnv) Scope() string {
	return r.env.scope
}

func (r ResolvedEnv) Selection() Selection {
	return r.env.sel
}
