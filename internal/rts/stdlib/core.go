package stdlib

import (
	"fmt"
	"math"
	"regexp"
	"slices"

	"github.com/google/uuid"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigFail     = "fail()"
	sigLen      = "len(x)"
	sigAppend   = "append(list, items...)"
	sigMin      = "min(values...)"
	sigMax      = "max(values...)"
	sigContains = "contains(a, b)"
	sigMatch    = "match(pattern, text)"
	sigStr      = "str(x)"
	sigUUID     = "uuid()"
)

var coreSpec = map[string]rts.NativeFunc{
	"fail":     coreFail,
	"len":      coreLen,
	"append":   coreAppend,
	"min":      coreMin,
	"max":      coreMax,
	"contains": coreContains,
	"match":    coreMatch,
	"str":      coreStr,
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

func coreAppend(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	return appendList(ctx, pos, args, sigAppend)
}

func coreMin(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	return minMax(ctx, pos, args, sigMin, math.Min, slices.MinFunc)
}

func coreMax(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	return minMax(ctx, pos, args, sigMax, math.Max, slices.MaxFunc)
}

func minMax(
	ctx *rts.Ctx,
	pos rts.Pos,
	args []rts.Value,
	sig string,
	num func(x, y float64) float64,
	pick func([]rts.Value, func(a, b rts.Value) int) rts.Value,
) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sig)
	if err := na.CountMin(1); err != nil {
		return rts.Null(), err
	}

	vals := args
	if na.Len() == 1 {
		items, err := na.List(0)
		if err != nil {
			return rts.Null(), err
		}
		if len(items) == 0 {
			return rts.Null(), nil
		}
		vals = items
	}

	if err := checkOrdered(ctx, pos, vals, sig); err != nil {
		return rts.Null(), err
	}
	if vals[0].K == rts.VStr {
		return pick(vals, compare), nil
	}

	// Numbers use math.Min and math.Max, as math.min and math.max do. compare
	// would put NaN first and treat -0 as equal to 0.
	n := vals[0].N
	for _, v := range vals[1:] {
		n = num(n, v.N)
	}
	return rts.Num(n), nil
}

func coreContains(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigContains)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	ok, err := rts.ValueContains(ctx, pos, na.Arg(0), na.Arg(1))
	if err != nil {
		return rts.Null(), err
	}
	return rts.Bool(ok), nil
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
