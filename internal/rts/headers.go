package rts

import "strings"

func stdlibHeadersNormalize(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "headers.normalize(h)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "headers.normalize(h)")
	if err != nil {
		return Null(), err
	}

	out, err := normHeaders(ctx, pos, m, "headers.normalize(h)")
	if err != nil {
		return Null(), err
	}
	return Dict(out), nil
}

func stdlibHeadersGet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "headers.get(h, name)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "headers.get(h, name)")
	if err != nil || m == nil {
		return Null(), err
	}

	name, err := keyArg(ctx, pos, args[1], "headers.get(h, name)")
	if err != nil {
		return Null(), err
	}

	val, ok, err := findHeader(ctx, pos, m, name)
	if err != nil || !ok {
		return Null(), err
	}
	return headValue(ctx, pos, val)
}

func stdlibHeadersHas(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "headers.has(h, name)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "headers.has(h, name)")
	if err != nil || m == nil {
		return Bool(false), err
	}

	name, err := keyArg(ctx, pos, args[1], "headers.has(h, name)")
	if err != nil {
		return Null(), err
	}

	val, ok, err := findHeader(ctx, pos, m, name)
	if err != nil || !ok {
		return Bool(false), err
	}
	checked, err := headerValue(ctx, pos, val)
	if err != nil {
		return Null(), err
	}

	switch checked.K {
	case VNull:
		return Bool(false), nil
	case VStr:
		return Bool(true), nil
	case VList:
		return Bool(len(checked.L) > 0), nil
	default:
		return Null(), rtErr(ctx, pos, "headers.has(h, name) expects header values as string/list")
	}
}

func stdlibHeadersSet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 3, "headers.set(h, name, value)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "headers.set(h, name, value)")
	if err != nil {
		return Null(), err
	}

	name, err := keyArg(ctx, pos, args[1], "headers.set(h, name, value)")
	if err != nil {
		return Null(), err
	}

	val, err := headerValue(ctx, pos, args[2])
	if err != nil {
		return Null(), err
	}

	out := cloneDict(m)
	out[strings.ToLower(name)] = val
	return Dict(out), nil
}

func stdlibHeadersRemove(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "headers.remove(h, name)"); err != nil {
		return Null(), err
	}

	m, err := dictArg(ctx, pos, args[0], "headers.remove(h, name)")
	if err != nil {
		return Null(), err
	}

	name, err := keyArg(ctx, pos, args[1], "headers.remove(h, name)")
	if err != nil {
		return Null(), err
	}

	out := cloneDict(m)
	delete(out, strings.ToLower(name))
	return Dict(out), nil
}

func stdlibHeadersMerge(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "headers.merge(a, b)"); err != nil {
		return Null(), err
	}

	a, err := dictArg(ctx, pos, args[0], "headers.merge(a, b)")
	if err != nil {
		return Null(), err
	}

	b, err := dictArg(ctx, pos, args[1], "headers.merge(a, b)")
	if err != nil {
		return Null(), err
	}

	na, err := normHeaders(ctx, pos, a, "headers.merge(a, b)")
	if err != nil {
		return Null(), err
	}

	nb, err := normHeaders(ctx, pos, b, "headers.merge(a, b)")
	if err != nil {
		return Null(), err
	}

	out := cloneDict(na)
	for k, v := range nb {
		if v.K == VNull {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return Dict(out), nil
}

func normHeaders(ctx *Ctx, pos Pos, m map[string]Value, sig string) (map[string]Value, error) {
	if len(m) == 0 {
		return map[string]Value{}, nil
	}

	out := make(map[string]Value, len(m))
	for k, v := range m {
		name, err := mapKey(ctx, pos, k, sig)
		if err != nil {
			return nil, err
		}
		name = strings.ToLower(name)
		val, err := headerValue(ctx, pos, v)
		if err != nil {
			return nil, err
		}
		out[name] = val
	}
	return out, nil
}

func headerValue(ctx *Ctx, pos Pos, v Value) (Value, error) {
	switch v.K {
	case VNull:
		return Null(), nil
	case VStr:
		return Str(v.S), nil
	case VList:
		out := make([]Value, 0, len(v.L))
		for _, it := range v.L {
			if it.K != VStr {
				return Null(), rtErr(ctx, pos, "headers expect string values")
			}
			out = append(out, Str(it.S))
		}
		return List(out), nil
	default:
		return Null(), rtErr(ctx, pos, "headers expect string values")
	}
}

func findHeader(ctx *Ctx, pos Pos, m map[string]Value, name string) (Value, bool, error) {
	key := strings.ToLower(name)
	if val, ok := m[key]; ok {
		return val, true, nil
	}
	for k, v := range m {
		if strings.EqualFold(k, name) {
			return v, true, nil
		}
	}
	return Null(), false, nil
}

func headValue(ctx *Ctx, pos Pos, v Value) (Value, error) {
	switch v.K {
	case VNull:
		return Null(), nil
	case VStr:
		return Str(v.S), nil
	case VList:
		if len(v.L) == 0 {
			return Null(), nil
		}
		if v.L[0].K != VStr {
			return Null(), rtErr(ctx, pos, "headers expect string values")
		}
		return Str(v.L[0].S), nil
	default:
		return Null(), rtErr(ctx, pos, "headers expect string values")
	}
}
