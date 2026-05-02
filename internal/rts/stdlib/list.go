package stdlib

import (
	"math"
	"sort"
	"strconv"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigListAppend = "list.append(list, item)"
	sigListConcat = "list.concat(a, b)"
	sigListSort   = "list.sort(list)"
	sigListMap    = "list.map(list, fn)"
	sigListFilter = "list.filter(list, fn)"
	sigListAny    = "list.any(list, fn)"
	sigListAll    = "list.all(list, fn)"
	sigListSlice  = "list.slice(list, start[, end])"
	sigListUnique = "list.unique(list)"
)

var listSpec = nsSpec{name: "list", fns: map[string]rts.NativeFunc{
	"append": listAppend,
	"concat": listConcat,
	"sort":   listSort,
	"map":    listMap,
	"filter": listFilter,
	"any":    listAny,
	"all":    listAll,
	"slice":  listSlice,
	"unique": listUnique,
}}

var (
	maxNativeIntFloat = float64(int(^uint(0) >> 1))
	minNativeIntFloat = -maxNativeIntFloat - 1
)

func listAppend(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigListAppend)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	items, err := na.List(0)
	if err != nil {
		return rts.Null(), err
	}

	out := make([]rts.Value, 0, len(items)+1)
	out = append(out, items...)
	out = append(out, na.Arg(1))
	if err := rts.CheckList(ctx, pos, len(out)); err != nil {
		return rts.Null(), err
	}
	return rts.List(out), nil
}

func listConcat(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigListConcat)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	a, err := na.List(0)
	if err != nil {
		return rts.Null(), err
	}

	b, err := na.List(1)
	if err != nil {
		return rts.Null(), err
	}

	out := make([]rts.Value, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	if err := rts.CheckList(ctx, pos, len(out)); err != nil {
		return rts.Null(), err
	}
	return rts.List(out), nil
}

func listSort(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigListSort)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	items, err := na.List(0)
	if err != nil {
		return rts.Null(), err
	}
	if len(items) <= 1 {
		if len(items) == 0 {
			return rts.List(nil), nil
		}
		out := make([]rts.Value, 0, len(items))
		out = append(out, items...)
		return rts.List(out), nil
	}

	kind := items[0].K
	for i := 0; i < len(items); i++ {
		if items[i].K != kind {
			return rts.Null(), rts.Errf(ctx, pos, "%s expects numbers or strings", sigListSort)
		}
	}

	out := make([]rts.Value, 0, len(items))
	out = append(out, items...)
	switch kind {
	case rts.VNum:
		sort.Slice(out, func(i, j int) bool { return out[i].N < out[j].N })
	case rts.VStr:
		sort.Slice(out, func(i, j int) bool { return out[i].S < out[j].S })
	default:
		return rts.Null(), rts.Errf(ctx, pos, "%s expects numbers or strings", sigListSort)
	}
	return rts.List(out), nil
}

func listMap(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigListMap)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	items, err := na.List(0)
	if err != nil {
		return rts.Null(), err
	}

	fn, err := na.Fn(1)
	if err != nil {
		return rts.Null(), err
	}
	if len(items) == 0 {
		return rts.List(nil), nil
	}

	out := make([]rts.Value, 0, len(items))
	for _, it := range items {
		if err := rts.Tick(ctx, pos); err != nil {
			return rts.Null(), err
		}
		v, err := rts.CallValue(ctx, pos, fn, []rts.Value{it})
		if err != nil {
			return rts.Null(), err
		}
		out = append(out, v)
		if err := rts.CheckList(ctx, pos, len(out)); err != nil {
			return rts.Null(), err
		}
	}
	return rts.List(out), nil
}

func listFilter(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigListFilter)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	items, err := na.List(0)
	if err != nil {
		return rts.Null(), err
	}

	fn, err := na.Fn(1)
	if err != nil {
		return rts.Null(), err
	}
	if len(items) == 0 {
		return rts.List(nil), nil
	}

	out := make([]rts.Value, 0, len(items))
	for _, it := range items {
		if err := rts.Tick(ctx, pos); err != nil {
			return rts.Null(), err
		}
		v, err := rts.CallValue(ctx, pos, fn, []rts.Value{it})
		if err != nil {
			return rts.Null(), err
		}
		if v.IsTruthy() {
			out = append(out, it)
			if err := rts.CheckList(ctx, pos, len(out)); err != nil {
				return rts.Null(), err
			}
		}
	}
	return rts.List(out), nil
}

func listAny(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigListAny)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	items, err := na.List(0)
	if err != nil {
		return rts.Null(), err
	}

	fn, err := na.Fn(1)
	if err != nil {
		return rts.Null(), err
	}
	for _, it := range items {
		if err := rts.Tick(ctx, pos); err != nil {
			return rts.Null(), err
		}
		v, err := rts.CallValue(ctx, pos, fn, []rts.Value{it})
		if err != nil {
			return rts.Null(), err
		}
		if v.IsTruthy() {
			return rts.Bool(true), nil
		}
	}
	return rts.Bool(false), nil
}

func listAll(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigListAll)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	items, err := na.List(0)
	if err != nil {
		return rts.Null(), err
	}

	fn, err := na.Fn(1)
	if err != nil {
		return rts.Null(), err
	}
	for _, it := range items {
		if err := rts.Tick(ctx, pos); err != nil {
			return rts.Null(), err
		}
		v, err := rts.CallValue(ctx, pos, fn, []rts.Value{it})
		if err != nil {
			return rts.Null(), err
		}
		if !v.IsTruthy() {
			return rts.Bool(false), nil
		}
	}
	return rts.Bool(true), nil
}

func listSlice(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigListSlice)
	if err := na.CountRange(2, 3); err != nil {
		return rts.Null(), err
	}

	items, err := na.List(0)
	if err != nil {
		return rts.Null(), err
	}
	if len(items) == 0 {
		return rts.List(nil), nil
	}

	st, err := intNum(ctx, pos, na.Arg(1), sigListSlice)
	if err != nil {
		return rts.Null(), err
	}

	en := len(items)
	if na.Has(2) {
		en, err = intNum(ctx, pos, na.Arg(2), sigListSlice)
		if err != nil {
			return rts.Null(), err
		}
	}

	st, en = sliceIdx(len(items), st, en)
	if en <= st {
		return rts.List(nil), nil
	}

	out := make([]rts.Value, 0, en-st)
	out = append(out, items[st:en]...)
	if err := rts.CheckList(ctx, pos, len(out)); err != nil {
		return rts.Null(), err
	}
	return rts.List(out), nil
}

func listUnique(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigListUnique)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	items, err := na.List(0)
	if err != nil {
		return rts.Null(), err
	}
	if len(items) == 0 {
		return rts.List(nil), nil
	}

	seen := map[string]struct{}{}
	out := make([]rts.Value, 0, len(items))
	for _, it := range items {
		if err := rts.Tick(ctx, pos); err != nil {
			return rts.Null(), err
		}

		k, err := keyVal(ctx, pos, it, sigListUnique)
		if err != nil {
			return rts.Null(), err
		}
		if _, ok := seen[k]; ok {
			continue
		}

		seen[k] = struct{}{}
		out = append(out, it)
		if err := rts.CheckList(ctx, pos, len(out)); err != nil {
			return rts.Null(), err
		}
	}
	return rts.List(out), nil
}

func intNum(ctx *rts.Ctx, pos rts.Pos, v rts.Value, sig string) (int, error) {
	n, err := numF(ctx, pos, v, sig)
	if err != nil {
		return 0, err
	}
	if math.Trunc(n) != n {
		return 0, rts.Errf(ctx, pos, "%s expects integer", sig)
	}

	if n > maxNativeIntFloat || n < minNativeIntFloat {
		return 0, rts.Errf(ctx, pos, "%s out of range", sig)
	}
	return int(n), nil
}

func sliceIdx(n, st, en int) (int, int) {
	st = clampIdx(n, st)
	en = clampIdx(n, en)
	if en < st {
		en = st
	}
	return st, en
}

func clampIdx(n, i int) int {
	if i < 0 {
		i = n + i
	}
	if i < 0 {
		return 0
	}
	if i > n {
		return n
	}
	return i
}

func keyVal(ctx *rts.Ctx, pos rts.Pos, v rts.Value, sig string) (string, error) {
	switch v.K {
	case rts.VNull:
		return "n:null", nil
	case rts.VBool:
		if v.B {
			return "b:true", nil
		}
		return "b:false", nil
	case rts.VNum:
		if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
			return "", rts.Errf(ctx, pos, "%s expects finite numbers", sig)
		}
		return "f:" + strconv.FormatFloat(v.N, 'g', -1, 64), nil
	case rts.VStr:
		return "s:" + v.S, nil
	default:
		return "", rts.Errf(ctx, pos, "%s expects list of primitives", sig)
	}
}
