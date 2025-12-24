package rts

import "sort"

func builtinDictKeys(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "dict.keys(dict)"); err != nil {
		return Null(), err
	}
	m, err := dictArg(ctx, pos, args[0], "dict.keys(dict)")
	if err != nil {
		return Null(), err
	}
	if len(m) == 0 {
		return List(nil), nil
	}
	if err := chkList(ctx, pos, len(m)); err != nil {
		return Null(), err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Value, 0, len(keys))
	for _, k := range keys {
		out = append(out, Str(k))
	}
	return List(out), nil
}

func builtinDictValues(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "dict.values(dict)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "dict.values(dict)")
	if err != nil {
		return Null(), err
	}
	if len(m) == 0 {
		return List(nil), nil
	}
	if err := chkList(ctx, pos, len(m)); err != nil {
		return Null(), err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	out := make([]Value, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return List(out), nil
}

func builtinDictItems(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "dict.items(dict)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "dict.items(dict)")
	if err != nil {
		return Null(), err
	}

	if len(m) == 0 {
		return List(nil), nil
	}

	if err := chkList(ctx, pos, len(m)); err != nil {
		return Null(), err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	out := make([]Value, 0, len(keys))
	for _, k := range keys {
		item := map[string]Value{
			"key":   Str(k),
			"value": m[k],
		}
		out = append(out, Dict(item))
	}
	return List(out), nil
}

func builtinDictSet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
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

func builtinDictMerge(ctx *Ctx, pos Pos, args []Value) (Value, error) {
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

func builtinDictRemove(ctx *Ctx, pos Pos, args []Value) (Value, error) {
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
