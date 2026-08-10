package rts

import (
	"iter"
	"maps"
	"slices"
)

// Extensions are host objects a caller adds to the base runtime, such as the
// TUI's mock inspector.
//
// An evaluation's prelude is assembled one layer at a time and later layers
// win:
//
//	standard library and host objects   the base runtime
//	Extensions                          added, must not collide with the base
//	@use aliases                        added, must not collide with either
//	Locals                              shadow everything before them
//	assertion shorthands                shadow locals, inside an assertion only
//
// Extensions sit on the additive side of that rule, so a name the base runtime
// already binds is a host bug rather than a shadow.
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
type Locals struct{ binds }

// NewLocals binds a copy of values as lexical locals.
func NewLocals(values map[string]Value) Locals { return Locals{newBinds(values)} }

// Local binds a single lexical local.
func Local(name string, value Value) Locals { return Locals{oneBind(name, value)} }

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

// pre is a prelude under construction. Callers hold it by value and share the
// map, so a layer applied to a copy is visible to the builder.
type pre struct{ values map[string]Value }

func (p pre) addExtensions(cx *Ctx, pos Pos, ext Extensions) error {
	for name, v := range ext.all() {
		if name == "" {
			continue
		}
		if IsKeyword(name) {
			return Errf(cx, pos, "name is a reserved word: %s", name)
		}
		if _, ok := p.values[name]; ok {
			return Errf(cx, pos, "name already defined: %s", name)
		}
		p.values[name] = v
	}
	return nil
}

// overlayLocals binds lexical values last. The parser already rejects a
// reserved word as a loop variable. Checking again here stops a host that
// builds locals by hand from creating a binding no expression could name.
func (p pre) overlayLocals(cx *Ctx, pos Pos, loc Locals) error {
	for name, v := range loc.all() {
		if name == "" {
			continue
		}
		if IsKeyword(name) {
			return Errf(cx, pos, "name is a reserved word: %s", name)
		}
		p.values[name] = v
	}
	return nil
}
