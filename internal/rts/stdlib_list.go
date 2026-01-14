package rts

import (
	"math"
	"sort"
	"strconv"
)

var listSpec = nsSpec{name: "list", fns: map[string]NativeFunc{
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

func listAppend(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "list.append(list, item)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	items, err := na.list(0)
	if err != nil {
		return Null(), err
	}

	out := make([]Value, 0, len(items)+1)
	out = append(out, items...)
	out = append(out, na.arg(1))
	if err := chkList(ctx, pos, len(out)); err != nil {
		return Null(), err
	}
	return List(out), nil
}

func listConcat(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "list.concat(a, b)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	a, err := na.list(0)
	if err != nil {
		return Null(), err
	}

	b, err := na.list(1)
	if err != nil {
		return Null(), err
	}

	out := make([]Value, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	if err := chkList(ctx, pos, len(out)); err != nil {
		return Null(), err
	}
	return List(out), nil
}

func listSort(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "list.sort(list)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	items, err := na.list(0)
	if err != nil {
		return Null(), err
	}
	if len(items) <= 1 {
		if len(items) == 0 {
			return List(nil), nil
		}
		out := make([]Value, 0, len(items))
		out = append(out, items...)
		return List(out), nil
	}

	kind := items[0].K
	for i := 0; i < len(items); i++ {
		if items[i].K != kind {
			return Null(), rtErr(ctx, pos, "list.sort(list) expects numbers or strings")
		}
	}

	out := make([]Value, 0, len(items))
	out = append(out, items...)
	switch kind {
	case VNum:
		sort.Slice(out, func(i, j int) bool { return out[i].N < out[j].N })
	case VStr:
		sort.Slice(out, func(i, j int) bool { return out[i].S < out[j].S })
	default:
		return Null(), rtErr(ctx, pos, "list.sort(list) expects numbers or strings")
	}
	return List(out), nil
}

func listMap(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "list.map(list, fn)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	items, err := na.list(0)
	if err != nil {
		return Null(), err
	}

	fn := na.arg(1)
	if err := fnChk(ctx, pos, fn, na.sig); err != nil {
		return Null(), err
	}
	if len(items) == 0 {
		return List(nil), nil
	}

	out := make([]Value, 0, len(items))
	for _, it := range items {
		if err := ctxTick(ctx, pos); err != nil {
			return Null(), err
		}
		v, err := callFn(ctx, pos, fn, []Value{it})
		if err != nil {
			return Null(), err
		}
		out = append(out, v)
		if err := chkList(ctx, pos, len(out)); err != nil {
			return Null(), err
		}
	}
	return List(out), nil
}

func listFilter(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "list.filter(list, fn)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	items, err := na.list(0)
	if err != nil {
		return Null(), err
	}

	fn := na.arg(1)
	if err := fnChk(ctx, pos, fn, na.sig); err != nil {
		return Null(), err
	}
	if len(items) == 0 {
		return List(nil), nil
	}

	out := make([]Value, 0, len(items))
	for _, it := range items {
		if err := ctxTick(ctx, pos); err != nil {
			return Null(), err
		}
		v, err := callFn(ctx, pos, fn, []Value{it})
		if err != nil {
			return Null(), err
		}
		if v.IsTruthy() {
			out = append(out, it)
			if err := chkList(ctx, pos, len(out)); err != nil {
				return Null(), err
			}
		}
	}
	return List(out), nil
}

func listAny(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "list.any(list, fn)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	items, err := na.list(0)
	if err != nil {
		return Null(), err
	}

	fn := na.arg(1)
	if err := fnChk(ctx, pos, fn, na.sig); err != nil {
		return Null(), err
	}
	for _, it := range items {
		if err := ctxTick(ctx, pos); err != nil {
			return Null(), err
		}
		v, err := callFn(ctx, pos, fn, []Value{it})
		if err != nil {
			return Null(), err
		}
		if v.IsTruthy() {
			return Bool(true), nil
		}
	}
	return Bool(false), nil
}

func listAll(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "list.all(list, fn)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	items, err := na.list(0)
	if err != nil {
		return Null(), err
	}

	fn := na.arg(1)
	if err := fnChk(ctx, pos, fn, na.sig); err != nil {
		return Null(), err
	}
	for _, it := range items {
		if err := ctxTick(ctx, pos); err != nil {
			return Null(), err
		}
		v, err := callFn(ctx, pos, fn, []Value{it})
		if err != nil {
			return Null(), err
		}
		if !v.IsTruthy() {
			return Bool(false), nil
		}
	}
	return Bool(true), nil
}

func listSlice(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "list.slice(list, start[, end])")
	if err := na.countRange(2, 3); err != nil {
		return Null(), err
	}

	items, err := na.list(0)
	if err != nil {
		return Null(), err
	}
	if len(items) == 0 {
		return List(nil), nil
	}

	st, err := intNum(ctx, pos, na.arg(1), na.sig)
	if err != nil {
		return Null(), err
	}

	en := len(items)
	if len(args) == 3 {
		en, err = intNum(ctx, pos, na.arg(2), na.sig)
		if err != nil {
			return Null(), err
		}
	}

	st, en = sliceIdx(len(items), st, en)
	if en <= st {
		return List(nil), nil
	}

	out := make([]Value, 0, en-st)
	out = append(out, items[st:en]...)
	if err := chkList(ctx, pos, len(out)); err != nil {
		return Null(), err
	}
	return List(out), nil
}

func listUnique(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "list.unique(list)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	items, err := na.list(0)
	if err != nil {
		return Null(), err
	}
	if len(items) == 0 {
		return List(nil), nil
	}

	seen := map[string]struct{}{}
	out := make([]Value, 0, len(items))
	for _, it := range items {
		if err := ctxTick(ctx, pos); err != nil {
			return Null(), err
		}

		k, err := keyVal(ctx, pos, it, na.sig)
		if err != nil {
			return Null(), err
		}
		if _, ok := seen[k]; ok {
			continue
		}

		seen[k] = struct{}{}
		out = append(out, it)
		if err := chkList(ctx, pos, len(out)); err != nil {
			return Null(), err
		}
	}
	return List(out), nil
}

func intNum(ctx *Ctx, pos Pos, v Value, sig string) (int, error) {
	n, err := numF(ctx, pos, v, sig)
	if err != nil {
		return 0, err
	}
	if math.Trunc(n) != n {
		return 0, rtErr(ctx, pos, "%s expects integer", sig)
	}

	max := float64(int(^uint(0) >> 1))
	min := -max - 1
	if n > max || n < min {
		return 0, rtErr(ctx, pos, "%s out of range", sig)
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

func keyVal(ctx *Ctx, pos Pos, v Value, sig string) (string, error) {
	switch v.K {
	case VNull:
		return "n:null", nil
	case VBool:
		if v.B {
			return "b:true", nil
		}
		return "b:false", nil
	case VNum:
		if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
			return "", rtErr(ctx, pos, "%s expects finite numbers", sig)
		}
		return "f:" + strconv.FormatFloat(v.N, 'g', -1, 64), nil
	case VStr:
		return "s:" + v.S, nil
	default:
		return "", rtErr(ctx, pos, "%s expects list of primitives", sig)
	}
}
