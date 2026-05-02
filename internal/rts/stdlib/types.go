package stdlib

import (
	"math"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigNum    = "num(x[, def])"
	sigInt    = "int(x[, def])"
	sigBool   = "bool(x[, def])"
	sigTypeof = "typeof(x)"
)

func coreNum(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	return conv(ctx, pos, args, sigNum, "expects number/string/bool", numTry, rts.Num)
}

func coreInt(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	return conv(ctx, pos, args, sigInt, "expects int/string/bool", intTry, rts.Num)
}

func coreBool(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	return conv(ctx, pos, args, sigBool, "expects bool/number/string", boolTry, rts.Bool)
}

func coreTypeof(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTypeof)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}
	return rts.Str(typeName(na.Arg(0))), nil
}

type cfn[T any] func(rts.Value) (T, bool)

func conv[T any](
	ctx *rts.Ctx,
	pos rts.Pos,
	args []rts.Value,
	sig, em string,
	f cfn[T],
	mk func(T) rts.Value,
) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sig)
	if err := na.CountRange(1, 2); err != nil {
		return rts.Null(), err
	}
	if v, ok := f(na.Arg(0)); ok {
		return mk(v), nil
	}
	if na.Has(1) {
		if v, ok := f(na.Arg(1)); ok {
			return mk(v), nil
		}
	}
	return rts.Null(), rts.Errf(ctx, pos, "%s %s", sig, em)
}

func numTry(v rts.Value) (float64, bool) {
	switch v.K {
	case rts.VNum:
		if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
			return 0, false
		}
		return v.N, true
	case rts.VBool:
		if v.B {
			return 1, true
		}
		return 0, true
	case rts.VStr:
		s := strings.TrimSpace(v.S)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	case rts.VNull:
		return 0, false
	default:
		return 0, false
	}
}

func intTry(v rts.Value) (float64, bool) {
	switch v.K {
	case rts.VNum:
		if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
			return 0, false
		}
		if math.Trunc(v.N) != v.N {
			return 0, false
		}
		return v.N, true
	case rts.VBool:
		if v.B {
			return 1, true
		}
		return 0, true
	case rts.VStr:
		s := strings.TrimSpace(v.S)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false
		}
		return float64(n), true
	case rts.VNull:
		return 0, false
	default:
		return 0, false
	}
}

func boolTry(v rts.Value) (bool, bool) {
	switch v.K {
	case rts.VBool:
		return v.B, true
	case rts.VNum:
		if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
			return false, false
		}
		return v.N != 0, true
	case rts.VStr:
		s := strings.TrimSpace(v.S)
		if s == "" {
			return false, false
		}
		s = strings.ToLower(s)
		switch s {
		case "true", "t", "yes", "y", "on", "1":
			return true, true
		case "false", "f", "no", "n", "off", "0":
			return false, true
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return false, false
		}
		return n != 0, true
	case rts.VNull:
		return false, false
	default:
		return false, false
	}
}

func typeName(v rts.Value) string {
	switch v.K {
	case rts.VNull:
		return "null"
	case rts.VBool:
		return "bool"
	case rts.VNum:
		return "number"
	case rts.VStr:
		return "string"
	case rts.VList:
		return "list"
	case rts.VDict:
		return "dict"
	case rts.VFunc:
		return "function"
	case rts.VNative:
		return "native"
	case rts.VObj:
		if v.O == nil {
			return "object"
		}
		name := v.O.TypeName()
		if strings.TrimSpace(name) == "" {
			return "object"
		}
		return name
	default:
		return "unknown"
	}
}
