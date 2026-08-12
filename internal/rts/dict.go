package rts

import "maps"

// CloneDict returns a shallow copy of an RTS dictionary. Nil input is treated
// as an empty dictionary.
func CloneDict(src map[string]Value) map[string]Value {
	if len(src) == 0 {
		return map[string]Value{}
	}
	return maps.Clone(src)
}
