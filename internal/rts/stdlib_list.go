package rts

import "sort"

func stdlibListAppend(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "list.append(list, item)"); err != nil {
		return Null(), err
	}

	items, err := listArg(ctx, pos, args[0], "list.append(list, item)")
	if err != nil {
		return Null(), err
	}

	out := make([]Value, 0, len(items)+1)
	out = append(out, items...)
	out = append(out, args[1])
	if err := chkList(ctx, pos, len(out)); err != nil {
		return Null(), err
	}
	return List(out), nil
}

func stdlibListConcat(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "list.concat(a, b)"); err != nil {
		return Null(), err
	}

	a, err := listArg(ctx, pos, args[0], "list.concat(a, b)")
	if err != nil {
		return Null(), err
	}

	b, err := listArg(ctx, pos, args[1], "list.concat(a, b)")
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

func stdlibListSort(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "list.sort(list)"); err != nil {
		return Null(), err
	}

	items, err := listArg(ctx, pos, args[0], "list.sort(list)")
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
