package stdlib

import (
	"math"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

const (
	sigMathAbs   = "math.abs(x)"
	sigMathMin   = "math.min(a, b)"
	sigMathMax   = "math.max(a, b)"
	sigMathClamp = "math.clamp(x, min, max)"
	sigMathFloor = "math.floor(x)"
	sigMathCeil  = "math.ceil(x)"
	sigMathRound = "math.round(x)"
)

var mathSpec = nsSpec{name: "math", fns: map[string]rts.NativeFunc{
	"abs":   mathAbs,
	"min":   mathMin,
	"max":   mathMax,
	"clamp": mathClamp,
	"floor": mathFloor,
	"ceil":  mathCeil,
	"round": mathRound,
}}

func mathAbs(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigMathAbs)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	n, err := na.Num(0)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Num(math.Abs(n)), nil
}

func mathMin(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigMathMin)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	a, err := na.Num(0)
	if err != nil {
		return rts.Null(), err
	}

	b, err := na.Num(1)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Num(math.Min(a, b)), nil
}

func mathMax(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigMathMax)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	a, err := na.Num(0)
	if err != nil {
		return rts.Null(), err
	}

	b, err := na.Num(1)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Num(math.Max(a, b)), nil
}

func mathClamp(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigMathClamp)
	if err := na.Count(3); err != nil {
		return rts.Null(), err
	}

	x, err := na.Num(0)
	if err != nil {
		return rts.Null(), err
	}

	lo, err := na.Num(1)
	if err != nil {
		return rts.Null(), err
	}

	hi, err := na.Num(2)
	if err != nil {
		return rts.Null(), err
	}
	if lo > hi {
		return rts.Null(), rts.Errf(ctx, pos, "%s expects min <= max", sigMathClamp)
	}
	if x < lo {
		return rts.Num(lo), nil
	}
	if x > hi {
		return rts.Num(hi), nil
	}
	return rts.Num(x), nil
}

func mathFloor(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigMathFloor)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	n, err := na.Num(0)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Num(math.Floor(n)), nil
}

func mathCeil(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigMathCeil)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	n, err := na.Num(0)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Num(math.Ceil(n)), nil
}

func mathRound(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigMathRound)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}

	n, err := na.Num(0)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Num(math.Round(n)), nil
}
