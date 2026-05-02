package stdlib

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

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

var textSpec = nsSpec{name: "text", fns: map[string]rts.NativeFunc{
	"lower":      textLower,
	"upper":      textUpper,
	"trim":       textTrim,
	"split":      textSplit,
	"join":       textJoin,
	"replace":    textReplace,
	"startsWith": textStartsWith,
	"endsWith":   textEndsWith,
}}

func textLower(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTextLower)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	out := strings.ToLower(s)
	if err := rts.CheckStr(ctx, pos, out); err != nil {
		return rts.Null(), err
	}
	return rts.Str(out), nil
}

func textUpper(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTextUpper)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	out := strings.ToUpper(s)
	if err := rts.CheckStr(ctx, pos, out); err != nil {
		return rts.Null(), err
	}
	return rts.Str(out), nil
}

func textTrim(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTextTrim)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	out := strings.TrimSpace(s)
	if err := rts.CheckStr(ctx, pos, out); err != nil {
		return rts.Null(), err
	}
	return rts.Str(out), nil
}

func textSplit(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTextSplit)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	sep, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}

	parts := strings.Split(s, sep)
	if err := rts.CheckList(ctx, pos, len(parts)); err != nil {
		return rts.Null(), err
	}

	out := make([]rts.Value, 0, len(parts))
	for _, p := range parts {
		if err := rts.CheckStr(ctx, pos, p); err != nil {
			return rts.Null(), err
		}
		out = append(out, rts.Str(p))
	}
	return rts.List(out), nil
}

func textJoin(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTextJoin)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	var items []rts.Value
	src := na.Arg(0)
	if src.K == rts.VNull {
		items = nil
	} else if src.K != rts.VList {
		return rts.Null(), rts.Errf(ctx, pos, "%s expects list", sigTextJoin)
	} else {
		items = src.L
	}

	sep, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}
	if err := rts.CheckList(ctx, pos, len(items)); err != nil {
		return rts.Null(), err
	}

	out := make([]string, 0, len(items))
	for _, it := range items {
		s, err := rts.ScalarStr(ctx, pos, it, sigTextJoin)
		if err != nil {
			return rts.Null(), err
		}
		if err := rts.CheckStr(ctx, pos, s); err != nil {
			return rts.Null(), err
		}
		out = append(out, s)
	}

	res := strings.Join(out, sep)
	if err := rts.CheckStr(ctx, pos, res); err != nil {
		return rts.Null(), err
	}
	return rts.Str(res), nil
}

func textReplace(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTextReplace)
	if err := na.Count(3); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	old, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}

	nw, err := na.Str(2)
	if err != nil {
		return rts.Null(), err
	}

	out := strings.ReplaceAll(s, old, nw)
	if err := rts.CheckStr(ctx, pos, out); err != nil {
		return rts.Null(), err
	}
	return rts.Str(out), nil
}

func textStartsWith(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTextStartsWith)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	p, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Bool(strings.HasPrefix(s, p)), nil
}

func textEndsWith(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTextEndsWith)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	s, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	suf, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Bool(strings.HasSuffix(s, suf)), nil
}
