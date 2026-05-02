package stdlib

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigFail     = "fail()"
	sigLen      = "len(x)"
	sigContains = "contains(a, b)"
	sigMatch    = "match(pattern, text)"
	sigStr      = "str(x)"
	sigDefault  = "default(a, b)"
	sigUUID     = "uuid()"
)

var coreSpec = map[string]rts.NativeFunc{
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

func coreFail(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigFail)
	msg := sigFail
	if na.Len() == 1 {
		s, err := na.ToStr(0)
		if err != nil {
			return rts.Null(), err
		}
		msg = s
	} else if na.Len() > 1 {
		msg = fmt.Sprintf("fail(%d args)", na.Len())
	}
	return rts.Null(), rts.Errf(ctx, pos, "%s", msg)
}

func coreLen(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigLen)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	switch na.Arg(0).K {
	case rts.VStr:
		return rts.Num(float64(len(na.Arg(0).S))), nil
	case rts.VList:
		return rts.Num(float64(len(na.Arg(0).L))), nil
	case rts.VDict:
		return rts.Num(float64(len(na.Arg(0).M))), nil
	default:
		return rts.Null(), rts.Errf(ctx, pos, "%s unsupported", sigLen)
	}
}

func coreContains(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigContains)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	h := na.Arg(0)
	n := na.Arg(1)
	s, err := na.ToStr(1)
	if err != nil {
		return rts.Null(), err
	}

	switch h.K {
	case rts.VStr:
		return rts.Bool(strings.Contains(h.S, s)), nil
	case rts.VList:
		for _, it := range h.L {
			if rts.ValueEqual(it, n) {
				return rts.Bool(true), nil
			}
		}
		return rts.Bool(false), nil
	case rts.VDict:
		_, ok := h.M[s]
		return rts.Bool(ok), nil
	default:
		return rts.Null(), rts.Errf(ctx, pos, "contains unsupported")
	}
}

func coreMatch(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigMatch)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	pat, err := na.ToStr(0)
	if err != nil {
		return rts.Null(), err
	}

	txt, err := na.ToStr(1)
	if err != nil {
		return rts.Null(), err
	}

	if ctx != nil && ctx.Lim.MaxStr > 0 && len(pat) > ctx.Lim.MaxStr {
		return rts.Null(), rts.Errf(ctx, pos, "pattern too long")
	}

	re, err := regexp.Compile(pat)
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "invalid regex")
	}
	return rts.Bool(re.MatchString(txt)), nil
}

func coreStr(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigStr)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	s, err := na.ToStr(0)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Str(s), nil
}

func coreDefault(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigDefault)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}
	if na.Arg(0).K != rts.VNull {
		return na.Arg(0), nil
	}
	return na.Arg(1), nil
}

func coreUUID(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigUUID)
	if err := na.Count(0); err != nil {
		return rts.Null(), err
	}

	if ctx != nil && ctx.UUID != nil {
		id, err := ctx.UUID()
		if err != nil {
			return rts.Null(), rts.Errf(ctx, pos, "uuid failed")
		}
		return rts.Str(id), nil
	}

	if ctx != nil && !ctx.AllowRandom {
		return rts.Null(), rts.Errf(ctx, pos, "uuid not allowed")
	}

	id, err := randUUID()
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "uuid failed")
	}
	return rts.Str(id), nil
}

func randUUID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
