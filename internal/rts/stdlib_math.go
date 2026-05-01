package rts

import "math"

const (
	sigMathAbs   = "math.abs(x)"
	sigMathMin   = "math.min(a, b)"
	sigMathMax   = "math.max(a, b)"
	sigMathClamp = "math.clamp(x, min, max)"
	sigMathFloor = "math.floor(x)"
	sigMathCeil  = "math.ceil(x)"
	sigMathRound = "math.round(x)"
)

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
	na := newNativeArgs(ctx, pos, args, sigMathAbs)
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
	na := newNativeArgs(ctx, pos, args, sigMathMin)
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
	na := newNativeArgs(ctx, pos, args, sigMathMax)
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
	na := newNativeArgs(ctx, pos, args, sigMathClamp)
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
		return Null(), rtErr(ctx, pos, "%s expects min <= max", sigMathClamp)
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
	na := newNativeArgs(ctx, pos, args, sigMathFloor)
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
	na := newNativeArgs(ctx, pos, args, sigMathCeil)
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
	na := newNativeArgs(ctx, pos, args, sigMathRound)
	if err := na.count(1); err != nil {
		return Null(), err
	}

	n, err := na.num(0)
	if err != nil {
		return Null(), err
	}
	return Num(math.Round(n)), nil
}
