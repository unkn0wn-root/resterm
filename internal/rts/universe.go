package rts

import "maps"

// newUniverse returns the universe, the scope of the predeclared names in pre.
// It binds each of them as a constant.
func newUniverse(pre map[string]Value) *Env {
	uni := NewEnv(nil)
	for name, v := range pre {
		uni.DefConst(name, v)
	}
	return uni
}

// newTopEnv returns a new top-level scope, the one that a module's or an
// evaluation's own declarations go into. It starts with a copy of the names
// predeclared in uni, so a declaration cannot reuse one of them.
func newTopEnv(uni *Env) *Env {
	return &Env{Vars: maps.Clone(uni.Vars)}
}
