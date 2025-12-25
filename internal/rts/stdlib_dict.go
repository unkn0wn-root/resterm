package rts

import "sort"

func stdlibDictKeys(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "dict.keys(dict)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "dict.keys(dict)")
	if err != nil {
		return Null(), err
	}
	keys, err := dictKeys(ctx, pos, m)
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

func stdlibDictValues(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "dict.values(dict)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "dict.values(dict)")
	if err != nil {
		return Null(), err
	}
	keys, err := dictKeys(ctx, pos, m)
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

func stdlibDictItems(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "dict.items(dict)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "dict.items(dict)")
	if err != nil {
		return Null(), err
	}
	keys, err := dictKeys(ctx, pos, m)
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

func stdlibDictSet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 3, "dict.set(dict, key, value)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "dict.set(dict, key, value)")
	if err != nil {
		return Null(), err
	}

	key, err := keyArg(ctx, pos, args[1], "dict.set(dict, key, value)")
	if err != nil {
		return Null(), err
	}

	out := cloneDict(m)
	out[key] = args[2]
	if err := chkDict(ctx, pos, len(out)); err != nil {
		return Null(), err
	}
	return Dict(out), nil
}

func stdlibDictMerge(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "dict.merge(a, b)"); err != nil {
		return Null(), err
	}

	a, err := dictArg(ctx, pos, args[0], "dict.merge(a, b)")
	if err != nil {
		return Null(), err
	}

	b, err := dictArg(ctx, pos, args[1], "dict.merge(a, b)")
	if err != nil {
		return Null(), err
	}

	out := cloneDict(a)
	for k, v := range b {
		out[k] = v
	}

	if err := chkDict(ctx, pos, len(out)); err != nil {
		return Null(), err
	}
	return Dict(out), nil
}

func stdlibDictRemove(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "dict.remove(dict, key)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "dict.remove(dict, key)")
	if err != nil {
		return Null(), err
	}

	key, err := keyArg(ctx, pos, args[1], "dict.remove(dict, key)")
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

func dictKeys(ctx *Ctx, pos Pos, m map[string]Value) ([]string, error) {
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
