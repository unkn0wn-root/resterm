package rts

import (
	"iter"
	"maps"
	"slices"
)

// Extensions are host objects a caller adds to the base runtime, such as the
// TUI's mock inspector.
//
// The standard library lives in the universe, a scope every evaluation shares.
// An evaluation's top-level scope sits inside it and is assembled one layer at
// a time, with later layers winning:
//
//	rts, stdlib and module bindings     the base runtime
//	Extensions                          added, must not collide with the base or the standard library
//	@use aliases                        added, must not collide with the base or Extensions
//	Locals                              shadow everything before them
//	assertion shorthands and test t     shadow locals in assertions and tests
//
// The script's own declarations join the same scope, so at the top level they
// may shadow the standard library and nothing else. Extensions sit on the
// additive side of that rule, so a name the standard library or the base runtime already binds
// is a host bug rather than a shadow.
type Extensions struct{ binds }

// NewExtensions binds a copy of values as host extensions.
func NewExtensions(values map[string]Value) Extensions { return Extensions{newBinds(values)} }

// Extension binds a single host extension.
func Extension(name string, value Value) Extensions { return Extensions{oneBind(name, value)} }

// Locals are lexical bindings for one evaluation, such as an @for-each item.
// They shadow the standard library, host objects, extensions, and @use aliases,
// so a loop variable named json refers to the item and not to the namespace.
// Being a distinct type from Extensions is what keeps a caller from handing
// lexical values to the layer that refuses to shadow.
type Locals struct {
	binds
	unset []string
}

// NewLocals binds a copy of values as lexical locals.
func NewLocals(values map[string]Value) Locals { return Locals{binds: newBinds(values)} }

// Local binds a single lexical local.
func Local(name string, value Value) Locals { return Locals{binds: oneBind(name, value)} }

// Unset binds name as a local with no value yet, such as a loop variable before
// its loop starts. Reading it fails as an undefined name.
func Unset(name string) Locals { return Locals{unset: []string{name}} }

// binds is the storage shared by every binding layer. A constructor copies the
// caller's values and nothing writes to the layer afterwards.
type binds struct{ values map[string]Value }

func newBinds(values map[string]Value) binds {
	return binds{values: CloneDict(values)}
}

func oneBind(name string, value Value) binds {
	return binds{values: map[string]Value{name: value}}
}

// all walks the layer by name so a rejected binding is always the same one.
func (b binds) all() iter.Seq2[string, Value] {
	return func(yield func(string, Value) bool) {
		for _, k := range slices.Sorted(maps.Keys(b.values)) {
			if !yield(k, b.values[k]) {
				return
			}
		}
	}
}

func addExtensions(cx *Ctx, pos Pos, env *Env, ext Extensions) error {
	for name, v := range ext.all() {
		if name == "" {
			continue
		}
		if IsKeyword(name) {
			return Errf(cx, pos, "name is a reserved word: %s", name)
		}
		if _, ok := env.Get(name); ok {
			return Errf(cx, pos, "name already defined: %s", name)
		}
		env.DefConst(name, v)
	}
	return nil
}

// overlayLocals binds lexical values last. The parser already rejects a
// reserved word as a loop variable. Checking again here stops a host that
// builds locals by hand from creating a binding no expression could name.
func overlayLocals(cx *Ctx, pos Pos, env *Env, loc Locals) error {
	for name, v := range loc.all() {
		if name == "" {
			continue
		}
		if IsKeyword(name) {
			return Errf(cx, pos, "name is a reserved word: %s", name)
		}
		env.DefConst(name, v)
	}
	for _, name := range loc.unset {
		env.DefUnset(name)
	}
	return nil
}
