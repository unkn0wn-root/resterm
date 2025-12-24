package rts

import "math"

func builtinMathAbs(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "math.abs(x)"); err != nil {
		return Null(), err
	}
	n, err := numArg(ctx, pos, args[0], "math.abs(x)")
	if err != nil {
		return Null(), err
	}
	return Num(math.Abs(n)), nil
}

func builtinMathMin(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "math.min(a, b)"); err != nil {
		return Null(), err
	}
	a, err := numArg(ctx, pos, args[0], "math.min(a, b)")
	if err != nil {
		return Null(), err
	}
	b, err := numArg(ctx, pos, args[1], "math.min(a, b)")
	if err != nil {
		return Null(), err
	}
	return Num(math.Min(a, b)), nil
}

func builtinMathMax(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 2, "math.max(a, b)"); err != nil {
		return Null(), err
	}
	a, err := numArg(ctx, pos, args[0], "math.max(a, b)")
	if err != nil {
		return Null(), err
	}
	b, err := numArg(ctx, pos, args[1], "math.max(a, b)")
	if err != nil {
		return Null(), err
	}
	return Num(math.Max(a, b)), nil
}

func builtinMathClamp(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 3, "math.clamp(x, min, max)"); err != nil {
		return Null(), err
	}
	x, err := numArg(ctx, pos, args[0], "math.clamp(x, min, max)")
	if err != nil {
		return Null(), err
	}
	lo, err := numArg(ctx, pos, args[1], "math.clamp(x, min, max)")
	if err != nil {
		return Null(), err
	}
	hi, err := numArg(ctx, pos, args[2], "math.clamp(x, min, max)")
	if err != nil {
		return Null(), err
	}
	if lo > hi {
		return Null(), rtErr(ctx, pos, "math.clamp(x, min, max) expects min <= max")
	}
	if x < lo {
		return Num(lo), nil
	}
	if x > hi {
		return Num(hi), nil
	}
	return Num(x), nil
}

func builtinMathFloor(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "math.floor(x)"); err != nil {
		return Null(), err
	}
	n, err := numArg(ctx, pos, args[0], "math.floor(x)")
	if err != nil {
		return Null(), err
	}
	return Num(math.Floor(n)), nil
}

func builtinMathCeil(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "math.ceil(x)"); err != nil {
		return Null(), err
	}
	n, err := numArg(ctx, pos, args[0], "math.ceil(x)")
	if err != nil {
		return Null(), err
	}
	return Num(math.Ceil(n)), nil
}

func builtinMathRound(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := argCount(ctx, pos, args, 1, "math.round(x)"); err != nil {
		return Null(), err
	}
	n, err := numArg(ctx, pos, args[0], "math.round(x)")
	if err != nil {
		return Null(), err
	}
	return Num(math.Round(n)), nil
}
