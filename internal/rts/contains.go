package rts

import (
	"slices"
	"strings"
)

// ValueContains is the language's membership relation. It backs in, not in,
// and contains(container, value).
func ValueContains(ctx *Ctx, pos Pos, container, value Value) (bool, error) {
	switch container.K {
	case VList:
		return slices.ContainsFunc(container.L, func(v Value) bool {
			return ValueEqual(v, value)
		}), nil
	case VStr:
		s, err := ToStr(ctx, pos, value)
		if err != nil {
			return false, err
		}
		return strings.Contains(container.S, s), nil
	case VDict:
		key, err := ToStr(ctx, pos, value)
		if err != nil {
			return false, err
		}
		_, ok := container.M[key]
		return ok, nil
	default:
		return false, Errf(ctx, pos, "membership container must be string, list, or dict")
	}
}
