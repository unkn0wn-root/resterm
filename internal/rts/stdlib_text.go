package rts

import "strings"

func stdlibTextLower(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "text.lower(s)"); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], "text.lower(s)")
	if err != nil {
		return Null(), err
	}

	out := strings.ToLower(s)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func stdlibTextUpper(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "text.upper(s)"); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], "text.upper(s)")
	if err != nil {
		return Null(), err
	}

	out := strings.ToUpper(s)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func stdlibTextTrim(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "text.trim(s)"); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], "text.trim(s)")
	if err != nil {
		return Null(), err
	}

	out := strings.TrimSpace(s)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func stdlibTextSplit(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "text.split(s, sep)"); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], "text.split(s, sep)")
	if err != nil {
		return Null(), err
	}

	sep, err := strArg(ctx, pos, args[1], "text.split(s, sep)")
	if err != nil {
		return Null(), err
	}

	parts := strings.Split(s, sep)
	if err := chkList(ctx, pos, len(parts)); err != nil {
		return Null(), err
	}

	out := make([]Value, 0, len(parts))
	for _, p := range parts {
		if err := chkStr(ctx, pos, p); err != nil {
			return Null(), err
		}
		out = append(out, Str(p))
	}
	return List(out), nil
}

func stdlibTextJoin(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "text.join(list, sep)"); err != nil {
		return Null(), err
	}

	var items []Value
	src := args[0]
	if src.K == VNull {
		items = nil
	} else if src.K != VList {
		return Null(), rtErr(ctx, pos, "text.join(list, sep) expects list")
	} else {
		items = src.L
	}

	sep, err := strArg(ctx, pos, args[1], "text.join(list, sep)")
	if err != nil {
		return Null(), err
	}
	if err := chkList(ctx, pos, len(items)); err != nil {
		return Null(), err
	}

	out := make([]string, 0, len(items))
	for _, it := range items {
		s, err := scalarStr(ctx, pos, it, "text.join(list, sep)")
		if err != nil {
			return Null(), err
		}
		if err := chkStr(ctx, pos, s); err != nil {
			return Null(), err
		}
		out = append(out, s)
	}

	res := strings.Join(out, sep)
	if err := chkStr(ctx, pos, res); err != nil {
		return Null(), err
	}
	return Str(res), nil
}

func stdlibTextReplace(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 3, "text.replace(s, old, new)"); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], "text.replace(s, old, new)")
	if err != nil {
		return Null(), err
	}

	old, err := strArg(ctx, pos, args[1], "text.replace(s, old, new)")
	if err != nil {
		return Null(), err
	}

	nw, err := strArg(ctx, pos, args[2], "text.replace(s, old, new)")
	if err != nil {
		return Null(), err
	}

	out := strings.ReplaceAll(s, old, nw)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func stdlibTextStartsWith(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "text.startsWith(s, prefix)"); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], "text.startsWith(s, prefix)")
	if err != nil {
		return Null(), err
	}

	p, err := strArg(ctx, pos, args[1], "text.startsWith(s, prefix)")
	if err != nil {
		return Null(), err
	}
	return Bool(strings.HasPrefix(s, p)), nil
}

func stdlibTextEndsWith(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "text.endsWith(s, suffix)"); err != nil {
		return Null(), err
	}

	s, err := strArg(ctx, pos, args[0], "text.endsWith(s, suffix)")
	if err != nil {
		return Null(), err
	}

	suf, err := strArg(ctx, pos, args[1], "text.endsWith(s, suffix)")
	if err != nil {
		return Null(), err
	}
	return Bool(strings.HasSuffix(s, suf)), nil
}
