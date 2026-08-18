package vars

import (
	"maps"
	"slices"
)

// envRef records missing references so they cannot fall through to another
// provider.
type envRef struct {
	value string
	found bool
}

// ResolvedEnv holds environment values for one execution. It keeps the original
// env: references for templates and their captured values for scripts.
type ResolvedEnv struct {
	env      Environment
	values   map[string]string
	refs     map[string]envRef
	refNames []string
	secrets  []string
	withheld bool
}

// Resolve reads each referenced OS environment variable once. The result does
// not change if the OS environment changes later.
func (e Environment) Resolve() ResolvedEnv {
	out := ResolvedEnv{env: e, values: e.values}
	var secrets Secrets
	for _, name := range slices.Sorted(maps.Keys(e.values)) {
		raw := e.values[name]
		key, ok := envRefKey(raw)
		if !ok {
			continue
		}
		if out.refs == nil {
			out.refs = make(map[string]envRef)
			out.values = maps.Clone(e.values)
		}
		out.refNames = append(out.refNames, name)

		ref, seen := out.refs[key]
		if !seen {
			value, _, found := EnvRefResolver(raw)
			ref = envRef{value: value, found: found}
			out.refs[key] = ref
		}
		if !ref.found {
			delete(out.values, name)
			continue
		}
		out.values[name] = ref.value
		secrets.Add(ref.value)
	}
	out.secrets = secrets.Values()
	return out
}

// Values returns the values exposed to scripts and settings. Missing env:
// references are omitted. The caller must not modify the returned map.
func (r ResolvedEnv) Values() map[string]string {
	return r.values
}

// AuthoredValues returns the values from the environment file, including env:
// references. The caller must not modify the returned map.
func (r ResolvedEnv) AuthoredValues() map[string]string {
	return r.env.values
}

func (r ResolvedEnv) HasRefs() bool {
	return len(r.refNames) > 0
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

// Secrets returns resolved env: values for redaction. The caller must not modify
// the returned slice.
func (r ResolvedEnv) Secrets() []string {
	return r.secrets
}

// RefResolver resolves declared env: references from the captured values. A
// reference not declared by the environment uses the current OS environment.
func (r ResolvedEnv) RefResolver() RefResolver {
	return func(raw string) (string, bool, bool) {
		key, ok := envRefKey(raw)
		if !ok {
			return "", false, false
		}
		ref, seen := r.refs[key]
		switch {
		case !seen:
			return EnvRefResolver(raw)
		case r.withheld:
			return "", true, false
		}
		return ref.value, true, ref.found
	}
}

// WithoutRefValues returns a copy with mapped OS values hidden. Their names
// remain defined so lower-priority providers cannot supply another value.
func (r ResolvedEnv) WithoutRefValues() ResolvedEnv {
	if !r.HasRefs() || r.withheld {
		return r
	}
	r.values = maps.Clone(r.values)
	for _, name := range r.refNames {
		delete(r.values, name)
	}
	r.withheld = true
	r.secrets = nil
	return r
}
