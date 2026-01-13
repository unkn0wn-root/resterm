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

func stdlibDictGet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "dict.get(dict, key[, def])"
	if err := argCountRange(ctx, pos, args, 2, 3, sig); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}

	if m == nil {
		if len(args) == 3 {
			return args[2], nil
		}
		return Null(), nil
	}

	key, err := keyArg(ctx, pos, args[1], sig)
	if err != nil {
		return Null(), err
	}

	v, ok := m[key]
	if ok {
		return v, nil
	}
	if len(args) == 3 {
		return args[2], nil
	}
	return Null(), nil
}

func stdlibDictHas(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "dict.has(dict, key)"
	if err := argCount(ctx, pos, args, 2, sig); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], sig)
	if err != nil || m == nil {
		return Bool(false), err
	}

	key, err := keyArg(ctx, pos, args[1], sig)
	if err != nil {
		return Null(), err
	}

	_, ok := m[key]
	return Bool(ok), nil
}

func stdlibDictPick(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "dict.pick(dict, keys)"
	if err := argCount(ctx, pos, args, 2, sig); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}

	keys, err := keyList(ctx, pos, args[1], sig)
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

func stdlibDictOmit(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "dict.omit(dict, keys)"
	if err := argCount(ctx, pos, args, 2, sig); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}

	out := cloneDict(m)
	keys, err := keyList(ctx, pos, args[1], sig)
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
