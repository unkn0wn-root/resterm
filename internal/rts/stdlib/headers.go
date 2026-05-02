package stdlib

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigHeadersNormalize = "headers.normalize(h)"
	sigHeadersGet       = "headers.get(h, name)"
	sigHeadersHas       = "headers.has(h, name)"
	sigHeadersSet       = "headers.set(h, name, value)"
	sigHeadersRemove    = "headers.remove(h, name)"
	sigHeadersMerge     = "headers.merge(a, b)"
)

var headersSpec = nsSpec{name: "headers", top: true, fns: map[string]rts.NativeFunc{
	"get":       headersGet,
	"has":       headersHas,
	"set":       headersSet,
	"remove":    headersRemove,
	"merge":     headersMerge,
	"normalize": headersNormalize,
}}

func headersNormalize(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigHeadersNormalize)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	out, err := normHeaders(ctx, pos, m, sigHeadersNormalize)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Dict(out), nil
}

func headersGet(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigHeadersGet)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil || m == nil {
		return rts.Null(), err
	}

	name, err := na.Key(1)
	if err != nil {
		return rts.Null(), err
	}

	val, ok, err := findHeader(ctx, pos, m, name)
	if err != nil || !ok {
		return rts.Null(), err
	}
	return headValue(ctx, pos, val)
}

func headersHas(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigHeadersHas)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil || m == nil {
		return rts.Bool(false), err
	}

	name, err := na.Key(1)
	if err != nil {
		return rts.Null(), err
	}

	val, ok, err := findHeader(ctx, pos, m, name)
	if err != nil || !ok {
		return rts.Bool(false), err
	}
	checked, err := headerValue(ctx, pos, val)
	if err != nil {
		return rts.Null(), err
	}

	switch checked.K {
	case rts.VNull:
		return rts.Bool(false), nil
	case rts.VStr:
		return rts.Bool(true), nil
	case rts.VList:
		return rts.Bool(len(checked.L) > 0), nil
	default:
		return rts.Null(), rts.Errf(
			ctx,
			pos,
			"%s expects header values as string/list",
			sigHeadersHas,
		)
	}
}

func headersSet(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigHeadersSet)
	if err := na.Count(3); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	name, err := na.Key(1)
	if err != nil {
		return rts.Null(), err
	}

	val, err := headerValue(ctx, pos, na.Arg(2))
	if err != nil {
		return rts.Null(), err
	}

	out := rts.CloneDict(m)
	out[strings.ToLower(name)] = val
	return rts.Dict(out), nil
}

func headersRemove(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigHeadersRemove)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	name, err := na.Key(1)
	if err != nil {
		return rts.Null(), err
	}

	out := rts.CloneDict(m)
	delete(out, strings.ToLower(name))
	return rts.Dict(out), nil
}

func headersMerge(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigHeadersMerge)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	a, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	b, err := na.Dict(1)
	if err != nil {
		return rts.Null(), err
	}

	normA, err := normHeaders(ctx, pos, a, sigHeadersMerge)
	if err != nil {
		return rts.Null(), err
	}

	normB, err := normHeaders(ctx, pos, b, sigHeadersMerge)
	if err != nil {
		return rts.Null(), err
	}

	out := rts.CloneDict(normA)
	for k, v := range normB {
		if v.K == rts.VNull {
			delete(out, k)
			continue
		}
		out[k] = v
	}
	return rts.Dict(out), nil
}

func normHeaders(
	ctx *rts.Ctx,
	pos rts.Pos,
	m map[string]rts.Value,
	sig string,
) (map[string]rts.Value, error) {
	if len(m) == 0 {
		return map[string]rts.Value{}, nil
	}

	out := make(map[string]rts.Value, len(m))
	for k, v := range m {
		name, err := rts.MapKey(ctx, pos, k, sig)
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

func headerValue(ctx *rts.Ctx, pos rts.Pos, v rts.Value) (rts.Value, error) {
	switch v.K {
	case rts.VNull:
		return rts.Null(), nil
	case rts.VStr:
		return rts.Str(v.S), nil
	case rts.VList:
		out := make([]rts.Value, 0, len(v.L))
		for _, it := range v.L {
			if it.K != rts.VStr {
				return rts.Null(), rts.Errf(ctx, pos, "headers expect string values")
			}
			out = append(out, rts.Str(it.S))
		}
		return rts.List(out), nil
	default:
		return rts.Null(), rts.Errf(ctx, pos, "headers expect string values")
	}
}

func findHeader(
	ctx *rts.Ctx,
	pos rts.Pos,
	m map[string]rts.Value,
	name string,
) (rts.Value, bool, error) {
	key := strings.ToLower(name)
	if val, ok := m[key]; ok {
		return val, true, nil
	}
	for k, v := range m {
		if strings.EqualFold(k, name) {
			return v, true, nil
		}
	}
	return rts.Null(), false, nil
}

func headValue(ctx *rts.Ctx, pos rts.Pos, v rts.Value) (rts.Value, error) {
	switch v.K {
	case rts.VNull:
		return rts.Null(), nil
	case rts.VStr:
		return rts.Str(v.S), nil
	case rts.VList:
		if len(v.L) == 0 {
			return rts.Null(), nil
		}
		if v.L[0].K != rts.VStr {
			return rts.Null(), rts.Errf(ctx, pos, "headers expect string values")
		}
		return rts.Str(v.L[0].S), nil
	default:
		return rts.Null(), rts.Errf(ctx, pos, "headers expect string values")
	}
}
