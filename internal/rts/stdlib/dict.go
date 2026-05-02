package stdlib

import (
	"maps"
	"sort"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigDictKeys   = "dict.keys(dict)"
	sigDictValues = "dict.values(dict)"
	sigDictItems  = "dict.items(dict)"
	sigDictSet    = "dict.set(dict, key, value)"
	sigDictMerge  = "dict.merge(a, b)"
	sigDictRemove = "dict.remove(dict, key)"
	sigDictGet    = "dict.get(dict, key[, def])"
	sigDictHas    = "dict.Has(dict, key)"
	sigDictPick   = "dict.pick(dict, keys)"
	sigDictOmit   = "dict.omit(dict, keys)"
)

var dictSpec = nsSpec{name: "dict", fns: map[string]rts.NativeFunc{
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

func dictKeys(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictKeys)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	keys, err := sortedDictKeys(ctx, pos, m)
	if err != nil {
		return rts.Null(), err
	}
	if len(keys) == 0 {
		return rts.List(nil), nil
	}

	out := make([]rts.Value, len(keys))
	for i, k := range keys {
		out[i] = rts.Str(k)
	}
	return rts.List(out), nil
}

func dictValues(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictValues)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	keys, err := sortedDictKeys(ctx, pos, m)
	if err != nil {
		return rts.Null(), err
	}
	if len(keys) == 0 {
		return rts.List(nil), nil
	}

	out := make([]rts.Value, len(keys))
	for i, k := range keys {
		out[i] = m[k]
	}
	return rts.List(out), nil
}

func dictItems(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictItems)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	keys, err := sortedDictKeys(ctx, pos, m)
	if err != nil {
		return rts.Null(), err
	}
	if len(keys) == 0 {
		return rts.List(nil), nil
	}

	out := make([]rts.Value, len(keys))
	for i, k := range keys {
		out[i] = rts.Dict(map[string]rts.Value{
			"key":   rts.Str(k),
			"value": m[k],
		})
	}
	return rts.List(out), nil
}

func dictSet(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictSet)
	if err := na.Count(3); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	key, err := na.Key(1)
	if err != nil {
		return rts.Null(), err
	}

	out := rts.CloneDict(m)
	out[key] = na.Arg(2)
	if err := rts.CheckDict(ctx, pos, len(out)); err != nil {
		return rts.Null(), err
	}
	return rts.Dict(out), nil
}

func dictMerge(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictMerge)
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

	out := rts.CloneDict(a)
	maps.Copy(out, b)

	if err := rts.CheckDict(ctx, pos, len(out)); err != nil {
		return rts.Null(), err
	}
	return rts.Dict(out), nil
}

func dictRemove(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictRemove)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	key, err := na.Key(1)
	if err != nil {
		return rts.Null(), err
	}

	out := rts.CloneDict(m)
	delete(out, key)
	if err := rts.CheckDict(ctx, pos, len(out)); err != nil {
		return rts.Null(), err
	}
	return rts.Dict(out), nil
}

func dictGet(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictGet)
	if err := na.CountRange(2, 3); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	if m == nil {
		if na.Has(2) {
			return na.Arg(2), nil
		}
		return rts.Null(), nil
	}

	key, err := na.Key(1)
	if err != nil {
		return rts.Null(), err
	}

	v, ok := m[key]
	if ok {
		return v, nil
	}
	if na.Has(2) {
		return na.Arg(2), nil
	}
	return rts.Null(), nil
}

func dictHas(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictHas)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil || m == nil {
		return rts.Bool(false), err
	}

	key, err := na.Key(1)
	if err != nil {
		return rts.Null(), err
	}

	_, ok := m[key]
	return rts.Bool(ok), nil
}

func dictPick(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictPick)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	keys, err := keyList(ctx, pos, na.Arg(1), sigDictPick)
	if err != nil {
		return rts.Null(), err
	}
	if len(keys) == 0 || len(m) == 0 {
		return rts.Dict(map[string]rts.Value{}), nil
	}

	out := make(map[string]rts.Value)
	for k := range setOfStrings(keys) {
		if err := rts.Tick(ctx, pos); err != nil {
			return rts.Null(), err
		}
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}

	if err := rts.CheckDict(ctx, pos, len(out)); err != nil {
		return rts.Null(), err
	}

	return rts.Dict(out), nil
}

func dictOmit(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDictOmit)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	m, err := na.Dict(0)
	if err != nil {
		return rts.Null(), err
	}

	out := rts.CloneDict(m)
	keys, err := keyList(ctx, pos, na.Arg(1), sigDictOmit)
	if err != nil {
		return rts.Null(), err
	}

	if len(keys) == 0 || len(out) == 0 {
		return rts.Dict(out), nil
	}

	for k := range setOfStrings(keys) {
		if err := rts.Tick(ctx, pos); err != nil {
			return rts.Null(), err
		}
		delete(out, k)
	}

	if err := rts.CheckDict(ctx, pos, len(out)); err != nil {
		return rts.Null(), err
	}

	return rts.Dict(out), nil
}

func sortedDictKeys(ctx *rts.Ctx, pos rts.Pos, m map[string]rts.Value) ([]string, error) {
	if len(m) == 0 {
		return nil, nil
	}
	if err := rts.CheckList(ctx, pos, len(m)); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, nil
}

func keyList(ctx *rts.Ctx, pos rts.Pos, v rts.Value, sig string) ([]string, error) {
	switch v.K {
	case rts.VNull:
		return nil, nil
	case rts.VStr:
		k, err := rts.KeyArg(ctx, pos, v, sig)
		if err != nil {
			return nil, err
		}
		return []string{k}, nil
	case rts.VList:
		if err := rts.CheckList(ctx, pos, len(v.L)); err != nil {
			return nil, err
		}
		if len(v.L) == 0 {
			return nil, nil
		}
		out := make([]string, 0, len(v.L))
		for _, it := range v.L {
			if err := rts.Tick(ctx, pos); err != nil {
				return nil, err
			}
			k, err := rts.KeyArg(ctx, pos, it, sig)
			if err != nil {
				return nil, err
			}
			out = append(out, k)
		}
		return out, nil
	default:
		return nil, rts.Errf(ctx, pos, "%s expects list or string", sig)
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
