package rts

import "strings"

const (
	sigTextLower      = "text.lower(s)"
	sigTextUpper      = "text.upper(s)"
	sigTextTrim       = "text.trim(s)"
	sigTextSplit      = "text.split(s, sep)"
	sigTextJoin       = "text.join(list, sep)"
	sigTextReplace    = "text.replace(s, old, new)"
	sigTextStartsWith = "text.startsWith(s, prefix)"
	sigTextEndsWith   = "text.endsWith(s, suffix)"
)

var textSpec = nsSpec{name: "text", fns: map[string]NativeFunc{
	"lower":      textLower,
	"upper":      textUpper,
	"trim":       textTrim,
	"split":      textSplit,
	"join":       textJoin,
	"replace":    textReplace,
	"startsWith": textStartsWith,
	"endsWith":   textEndsWith,
}}

func textLower(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTextLower)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	out := strings.ToLower(s)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func textUpper(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTextUpper)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	out := strings.ToUpper(s)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func textTrim(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTextTrim)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	out := strings.TrimSpace(s)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func textSplit(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTextSplit)
	if err := na.count(2); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	sep, err := na.str(1)
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

func textJoin(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTextJoin)
	if err := na.count(2); err != nil {
		return Null(), err
	}

	var items []Value
	src := na.arg(0)
	if src.K == VNull {
		items = nil
	} else if src.K != VList {
		return Null(), rtErr(ctx, pos, "%s expects list", sigTextJoin)
	} else {
		items = src.L
	}

	sep, err := na.str(1)
	if err != nil {
		return Null(), err
	}
	if err := chkList(ctx, pos, len(items)); err != nil {
		return Null(), err
	}

	out := make([]string, 0, len(items))
	for _, it := range items {
		s, err := scalarStr(ctx, pos, it, sigTextJoin)
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

func textReplace(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTextReplace)
	if err := na.count(3); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	old, err := na.str(1)
	if err != nil {
		return Null(), err
	}

	nw, err := na.str(2)
	if err != nil {
		return Null(), err
	}

	out := strings.ReplaceAll(s, old, nw)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func textStartsWith(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTextStartsWith)
	if err := na.count(2); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	p, err := na.str(1)
	if err != nil {
		return Null(), err
	}
	return Bool(strings.HasPrefix(s, p)), nil
}

func textEndsWith(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTextEndsWith)
	if err := na.count(2); err != nil {
		return Null(), err
	}

	s, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	suf, err := na.str(1)
	if err != nil {
		return Null(), err
	}
	return Bool(strings.HasSuffix(s, suf)), nil
}
