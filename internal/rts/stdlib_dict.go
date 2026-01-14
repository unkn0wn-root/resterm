package rts

import (
	"maps"
	"sort"
)

var dictSpec = nsSpec{name: "dict", fns: map[string]NativeFunc{
	"keys":   dictKeys,
	"values": dictValues,
	"items":  dictItems,
	"set":    dictSet,
	"merge":  dictMerge,
	"remove": dictRemove,
	"get":    dictGet,
	"has":    dictHas,
	"pick":   dictPick,
	"omit":   dictOmit,
}}

func dictKeys(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.keys(dict)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	keys, err := sortedDictKeys(ctx, pos, m)
	if err != nil {
		return Null(), err
	}
	if len(keys) == 0 {
		return List(nil), nil
	}

	out := make([]Value, len(keys))
	for i, k := range keys {
		out[i] = Str(k)
	}
	return List(out), nil
}

func dictValues(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.values(dict)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	keys, err := sortedDictKeys(ctx, pos, m)
	if err != nil {
		return Null(), err
	}
	if len(keys) == 0 {
		return List(nil), nil
	}

	out := make([]Value, len(keys))
	for i, k := range keys {
		out[i] = m[k]
	}
	return List(out), nil
}

func dictItems(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.items(dict)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	keys, err := sortedDictKeys(ctx, pos, m)
	if err != nil {
		return Null(), err
	}
	if len(keys) == 0 {
		return List(nil), nil
	}

	out := make([]Value, len(keys))
	for i, k := range keys {
		out[i] = Dict(map[string]Value{
			"key":   Str(k),
			"value": m[k],
		})
	}
	return List(out), nil
}

func dictSet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.set(dict, key, value)")
	if err := na.count(3); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	key, err := na.key(1)
	if err != nil {
		return Null(), err
	}

	out := cloneDict(m)
	out[key] = na.arg(2)
	if err := chkDict(ctx, pos, len(out)); err != nil {
		return Null(), err
	}
	return Dict(out), nil
}

func dictMerge(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.merge(a, b)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	a, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	b, err := na.dict(1)
	if err != nil {
		return Null(), err
	}

	out := cloneDict(a)
	maps.Copy(out, b)

	if err := chkDict(ctx, pos, len(out)); err != nil {
		return Null(), err
	}
	return Dict(out), nil
}

func dictRemove(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.remove(dict, key)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	key, err := na.key(1)
	if err != nil {
		return Null(), err
	}

	out := cloneDict(m)
	delete(out, key)
	if err := chkDict(ctx, pos, len(out)); err != nil {
		return Null(), err
	}
	return Dict(out), nil
}

func dictGet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.get(dict, key[, def])")
	if err := na.countRange(2, 3); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	if m == nil {
		if len(args) == 3 {
			return na.arg(2), nil
		}
		return Null(), nil
	}

	key, err := na.key(1)
	if err != nil {
		return Null(), err
	}

	v, ok := m[key]
	if ok {
		return v, nil
	}
	if len(args) == 3 {
		return na.arg(2), nil
	}
	return Null(), nil
}

func dictHas(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.has(dict, key)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil || m == nil {
		return Bool(false), err
	}

	key, err := na.key(1)
	if err != nil {
		return Null(), err
	}

	_, ok := m[key]
	return Bool(ok), nil
}

func dictPick(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.pick(dict, keys)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	keys, err := keyList(ctx, pos, na.arg(1), na.sig)
	if err != nil {
		return Null(), err
	}
	if len(keys) == 0 || len(m) == 0 {
		return Dict(map[string]Value{}), nil
	}

	out := make(map[string]Value)
	for k := range setOfStrings(keys) {
		if err := ctxTick(ctx, pos); err != nil {
			return Null(), err
		}
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}

	if err := chkDict(ctx, pos, len(out)); err != nil {
		return Null(), err
	}

	return Dict(out), nil
}

func dictOmit(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "dict.omit(dict, keys)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	out := cloneDict(m)
	keys, err := keyList(ctx, pos, na.arg(1), na.sig)
	if err != nil {
		return Null(), err
	}

	if len(keys) == 0 || len(out) == 0 {
		return Dict(out), nil
	}

	for k := range setOfStrings(keys) {
		if err := ctxTick(ctx, pos); err != nil {
			return Null(), err
		}
		delete(out, k)
	}

	if err := chkDict(ctx, pos, len(out)); err != nil {
		return Null(), err
	}

	return Dict(out), nil
}

func sortedDictKeys(ctx *Ctx, pos Pos, m map[string]Value) ([]string, error) {
	if len(m) == 0 {
		return nil, nil
	}
	if err := chkList(ctx, pos, len(m)); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func keyList(ctx *Ctx, pos Pos, v Value, sig string) ([]string, error) {
	switch v.K {
	case VNull:
		return nil, nil
	case VStr:
		k, err := keyArg(ctx, pos, v, sig)
		if err != nil {
			return nil, err
		}
		return []string{k}, nil
	case VList:
		if err := chkList(ctx, pos, len(v.L)); err != nil {
			return nil, err
		}
		if len(v.L) == 0 {
			return nil, nil
		}
		out := make([]string, 0, len(v.L))
		for _, it := range v.L {
			if err := ctxTick(ctx, pos); err != nil {
				return nil, err
			}
			k, err := keyArg(ctx, pos, it, sig)
			if err != nil {
				return nil, err
			}
			out = append(out, k)
		}
		return out, nil
	default:
		return nil, rtErr(ctx, pos, "%s expects list or string", sig)
	}
}

func setOfStrings(in []string) map[string]struct{} {
	if len(in) == 0 {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{}, len(in))
	for _, it := range in {
		out[it] = struct{}{}
	}
	return out
}
