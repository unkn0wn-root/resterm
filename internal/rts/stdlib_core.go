package rts

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var coreSpec = map[string]NativeFunc{
	"fail":     coreFail,
	"len":      coreLen,
	"contains": coreContains,
	"match":    coreMatch,
	"str":      coreStr,
	"default":  coreDefault,
	"num":      coreNum,
	"int":      coreInt,
	"bool":     coreBool,
	"typeof":   coreTypeof,
	"uuid":     coreUUID,
}

func coreFail(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	msg := "fail()"
	if len(args) == 1 {
		s, err := toStr(ctx, pos, args[0])
		if err != nil {
			return Null(), err
		}
		msg = s
	} else if len(args) > 1 {
		msg = fmt.Sprintf("fail(%d args)", len(args))
	}
	return Null(), rtErr(ctx, pos, "%s", msg)
}

func coreLen(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "len(x)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	switch na.arg(0).K {
	case VStr:
		return Num(float64(len(na.arg(0).S))), nil
	case VList:
		return Num(float64(len(na.arg(0).L))), nil
	case VDict:
		return Num(float64(len(na.arg(0).M))), nil
	default:
		return Null(), rtErr(ctx, pos, "len(x) unsupported")
	}
}

func coreContains(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "contains(a, b)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	h := na.arg(0)
	n := na.arg(1)
	s, err := na.toStr(1)
	if err != nil {
		return Null(), err
	}

	switch h.K {
	case VStr:
		return Bool(strings.Contains(h.S, s)), nil
	case VList:
		for _, it := range h.L {
			if eq(it, n) {
				return Bool(true), nil
			}
		}
		return Bool(false), nil
	case VDict:
		_, ok := h.M[s]
		return Bool(ok), nil
	default:
		return Null(), rtErr(ctx, pos, "contains unsupported")
	}
}

func coreMatch(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "match(pattern, text)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	pat, err := na.toStr(0)
	if err != nil {
		return Null(), err
	}

	txt, err := na.toStr(1)
	if err != nil {
		return Null(), err
	}

	if ctx != nil && ctx.Lim.MaxStr > 0 && len(pat) > ctx.Lim.MaxStr {
		return Null(), rtErr(ctx, pos, "pattern too long")
	}

	re, err := regexp.Compile(pat)
	if err != nil {
		return Null(), rtErr(ctx, pos, "invalid regex")
	}
	return Bool(re.MatchString(txt)), nil
}

func coreStr(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "str(x)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	s, err := na.toStr(0)
	if err != nil {
		return Null(), err
	}
	return Str(s), nil
}

func coreDefault(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "default(a, b)")
	if err := na.count(2); err != nil {
		return Null(), err
	}
	if na.arg(0).K != VNull {
		return na.arg(0), nil
	}
	return na.arg(1), nil
}

func coreUUID(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "uuid()")
	if err := na.count(0); err != nil {
		return Null(), err
	}

	if ctx != nil && ctx.UUID != nil {
		id, err := ctx.UUID()
		if err != nil {
			return Null(), rtErr(ctx, pos, "uuid failed")
		}
		return Str(id), nil
	}

	if ctx != nil && !ctx.AllowRandom {
		return Null(), rtErr(ctx, pos, "uuid not allowed")
	}

	id, err := randUUID()
	if err != nil {
		return Null(), rtErr(ctx, pos, "uuid failed")
	}
	return Str(id), nil
}

func randUUID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
