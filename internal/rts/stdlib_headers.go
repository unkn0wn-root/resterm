package rts

import "strings"

var headersSpec = nsSpec{name: "headers", top: true, fns: map[string]NativeFunc{
	"get":       headersGet,
	"has":       headersHas,
	"set":       headersSet,
	"remove":    headersRemove,
	"merge":     headersMerge,
	"normalize": headersNormalize,
}}

func headersNormalize(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "headers.normalize(h)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	out, err := normHeaders(ctx, pos, m, na.sig)
	if err != nil {
		return Null(), err
	}
	return Dict(out), nil
}

func headersGet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "headers.get(h, name)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil || m == nil {
		return Null(), err
	}

	name, err := na.key(1)
	if err != nil {
		return Null(), err
	}

	val, ok, err := findHeader(ctx, pos, m, name)
	if err != nil || !ok {
		return Null(), err
	}
	return headValue(ctx, pos, val)
}

func headersHas(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "headers.has(h, name)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil || m == nil {
		return Bool(false), err
	}

	name, err := na.key(1)
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

func headersSet(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "headers.set(h, name, value)")
	if err := na.count(3); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	name, err := na.key(1)
	if err != nil {
		return Null(), err
	}

	val, err := headerValue(ctx, pos, na.arg(2))
	if err != nil {
		return Null(), err
	}

	out := cloneDict(m)
	out[strings.ToLower(name)] = val
	return Dict(out), nil
}

func headersRemove(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "headers.remove(h, name)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	m, err := na.dict(0)
	if err != nil {
		return Null(), err
	}

	name, err := na.key(1)
	if err != nil {
		return Null(), err
	}

	out := cloneDict(m)
	delete(out, strings.ToLower(name))
	return Dict(out), nil
}

func headersMerge(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "headers.merge(a, b)")
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

	normA, err := normHeaders(ctx, pos, a, na.sig)
	if err != nil {
		return Null(), err
	}

	normB, err := normHeaders(ctx, pos, b, na.sig)
	if err != nil {
		return Null(), err
	}

	out := cloneDict(normA)
	for k, v := range normB {
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
