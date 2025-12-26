package rts

import "strings"

type ms struct {
	g string
	h string
	r string
}

func newMS(n string) ms {
	return ms{
		g: n + ".get(name)",
		h: n + ".has(name)",
		r: n + ".require(name[, msg])",
	}
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	if len(m) == 0 {
		return map[K]V{}
	}
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func lowerMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[lowerKey(k)] = v
	}
	return out
}

func mapLookup(m map[string]string, name string) (string, bool) {
	v, ok := m[lowerKey(name)]
	return v, ok
}

func mapMember(m map[string]string, name string) (Value, bool) {
	v, ok := mapLookup(m, name)
	if !ok {
		return Null(), false
	}
	return Str(v), true
}

func mapIndex(m map[string]string, key Value) (Value, error) {
	k, err := toKey(Pos{}, key)
	if err != nil {
		return Null(), err
	}
	v, ok := mapLookup(m, k)
	if !ok {
		return Null(), nil
	}
	return Str(v), nil
}

func mapGet(ctx *Ctx, pos Pos, args []Value, sig string, m map[string]string) (Value, error) {
	if err := argCount(ctx, pos, args, 1, sig); err != nil {
		return Null(), err
	}
	k, err := toKey(pos, args[0])
	if err != nil {
		return Null(), wrapErr(ctx, err)
	}
	v, ok := mapLookup(m, k)
	if !ok {
		return Null(), nil
	}
	return Str(v), nil
}

func mapHas(ctx *Ctx, pos Pos, args []Value, sig string, m map[string]string) (Value, error) {
	if err := argCount(ctx, pos, args, 1, sig); err != nil {
		return Null(), err
	}
	k, err := toKey(pos, args[0])
	if err != nil {
		return Null(), wrapErr(ctx, err)
	}
	_, ok := mapLookup(m, k)
	return Bool(ok), nil
}

func mapRequire(ctx *Ctx, pos Pos, args []Value, sig, obj string, m map[string]string) (Value, error) {
	if err := argCountRange(ctx, pos, args, 1, 2, sig); err != nil {
		return Null(), err
	}
	k, err := keyArg(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}
	v, ok := mapLookup(m, k)
	if ok && strings.TrimSpace(v) != "" {
		return Str(v), nil
	}
	return Null(), reqErr(ctx, pos, obj, k, args)
}
