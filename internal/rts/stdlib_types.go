package rts

import (
	"math"
	"strconv"
	"strings"
)

func coreNum(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return conv(ctx, pos, args, "num(x[, def])", "expects number/string/bool", numTry, Num)
}

func coreInt(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return conv(ctx, pos, args, "int(x[, def])", "expects int/string/bool", intTry, Num)
}

func coreBool(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	return conv(ctx, pos, args, "bool(x[, def])", "expects bool/number/string", boolTry, Bool)
}

func coreTypeof(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "typeof(x)")
	if err := na.count(1); err != nil {
		return Null(), err
	}
	return Str(typeName(na.arg(0))), nil
}

type cfn[T any] func(Value) (T, bool)

func conv[T any](
	ctx *Ctx,
	pos Pos,
	args []Value,
	sig, em string,
	f cfn[T],
	mk func(T) Value,
) (Value, error) {
	if err := argCountRange(ctx, pos, args, 1, 2, sig); err != nil {
		return Null(), err
	}
	if v, ok := f(args[0]); ok {
		return mk(v), nil
	}
	if len(args) == 2 {
		if v, ok := f(args[1]); ok {
			return mk(v), nil
		}
	}
	return Null(), rtErr(ctx, pos, "%s %s", sig, em)
}

func numTry(v Value) (float64, bool) {
	switch v.K {
	case VNum:
		if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
			return 0, false
		}
		return v.N, true
	case VBool:
		if v.B {
			return 1, true
		}
		return 0, true
	case VStr:
		s := strings.TrimSpace(v.S)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	case VNull:
		return 0, false
	default:
		return 0, false
	}
}

func intTry(v Value) (float64, bool) {
	switch v.K {
	case VNum:
		if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
			return 0, false
		}
		if math.Trunc(v.N) != v.N {
			return 0, false
		}
		return v.N, true
	case VBool:
		if v.B {
			return 1, true
		}
		return 0, true
	case VStr:
		s := strings.TrimSpace(v.S)
		if s == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, false
		}
		return float64(n), true
	case VNull:
		return 0, false
	default:
		return 0, false
	}
}

func boolTry(v Value) (bool, bool) {
	switch v.K {
	case VBool:
		return v.B, true
	case VNum:
		if math.IsNaN(v.N) || math.IsInf(v.N, 0) {
			return false, false
		}
		return v.N != 0, true
	case VStr:
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
	case VNull:
		return false, false
	default:
		return false, false
	}
}

func typeName(v Value) string {
	switch v.K {
	case VNull:
		return "null"
	case VBool:
		return "bool"
	case VNum:
		return "number"
	case VStr:
		return "string"
	case VList:
		return "list"
	case VDict:
		return "dict"
	case VFunc:
		return "function"
	case VNative:
		return "native"
	case VObj:
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
