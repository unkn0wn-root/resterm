package rts

import "math"

var mathSpec = nsSpec{name: "math", fns: map[string]NativeFunc{
	"abs":   mathAbs,
	"min":   mathMin,
	"max":   mathMax,
	"clamp": mathClamp,
	"floor": mathFloor,
	"ceil":  mathCeil,
	"round": mathRound,
}}

func mathAbs(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "math.abs(x)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	n, err := na.num(0)
	if err != nil {
		return Null(), err
	}
	return Num(math.Abs(n)), nil
}

func mathMin(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "math.min(a, b)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	a, err := na.num(0)
	if err != nil {
		return Null(), err
	}

	b, err := na.num(1)
	if err != nil {
		return Null(), err
	}
	return Num(math.Min(a, b)), nil
}

func mathMax(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "math.max(a, b)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	a, err := na.num(0)
	if err != nil {
		return Null(), err
	}

	b, err := na.num(1)
	if err != nil {
		return Null(), err
	}
	return Num(math.Max(a, b)), nil
}

func mathClamp(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "math.clamp(x, min, max)")
	if err := na.count(3); err != nil {
		return Null(), err
	}

	x, err := na.num(0)
	if err != nil {
		return Null(), err
	}

	lo, err := na.num(1)
	if err != nil {
		return Null(), err
	}

	hi, err := na.num(2)
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

func mathFloor(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "math.floor(x)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	n, err := na.num(0)
	if err != nil {
		return Null(), err
	}
	return Num(math.Floor(n)), nil
}

func mathCeil(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "math.ceil(x)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	n, err := na.num(0)
	if err != nil {
		return Null(), err
	}
	return Num(math.Ceil(n)), nil
}

func mathRound(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "math.round(x)")
	if err := na.count(1); err != nil {
		return Null(), err
	}

	n, err := na.num(0)
	if err != nil {
		return Null(), err
	}
	return Num(math.Round(n)), nil
}
