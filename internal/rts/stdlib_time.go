package rts

import (
	"math"
	"time"
)

const (
	maxI  = float64(^uint64(0) >> 1)
	minI  = -maxI - 1
	nsSec = int64(time.Second)
)

func stdlibTimeNowUnix(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "time.nowUnix()"
	if err := argCount(ctx, pos, args, 0, sig); err != nil {
		return Null(), err
	}

	t, err := nowT(ctx, pos)
	if err != nil {
		return Null(), err
	}
	return Num(float64(t.Unix())), nil
}

func stdlibTimeNowUnixMs(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "time.nowUnixMs()"
	if err := argCount(ctx, pos, args, 0, sig); err != nil {
		return Null(), err
	}

	t, err := nowT(ctx, pos)
	if err != nil {
		return Null(), err
	}

	n := t.UnixNano() / int64(time.Millisecond)
	return Num(float64(n)), nil
}

func stdlibTimeParse(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "time.parse(layout, value)"
	if err := argCount(ctx, pos, args, 2, sig); err != nil {
		return Null(), err
	}

	layout, err := strArg(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}

	val, err := strArg(ctx, pos, args[1], sig)
	if err != nil {
		return Null(), err
	}

	t, err := time.Parse(layout, val)
	if err != nil {
		return Null(), rtErr(ctx, pos, "time parse failed")
	}

	sec := float64(t.UnixNano()) / float64(time.Second)
	return Num(sec), nil
}

func stdlibTimeFormatUnix(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "time.formatUnix(ts, layout)"
	if err := argCount(ctx, pos, args, 2, sig); err != nil {
		return Null(), err
	}

	layout, err := strArg(ctx, pos, args[1], sig)
	if err != nil {
		return Null(), err
	}

	sec, ns, err := splitUnix(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}

	t := time.Unix(sec, ns).UTC()
	return fmtTime(ctx, pos, t, layout)
}

func stdlibTimeAddUnix(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := "time.addUnix(ts, seconds)"
	if err := argCount(ctx, pos, args, 2, sig); err != nil {
		return Null(), err
	}

	a, err := numF(ctx, pos, args[0], sig)
	if err != nil {
		return Null(), err
	}

	b, err := numF(ctx, pos, args[1], sig)
	if err != nil {
		return Null(), err
	}

	out := a + b
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return Null(), rtErr(ctx, pos, "%s expects finite number", sig)
	}

	return Num(out), nil
}

func nowT(ctx *Ctx, pos Pos) (time.Time, error) {
	if ctx == nil || ctx.Now == nil {
		return time.Time{}, rtErr(ctx, pos, "time not available")
	}
	return ctx.Now(), nil
}

func fmtTime(ctx *Ctx, pos Pos, t time.Time, layout string) (Value, error) {
	out := t.Format(layout)
	if err := chkStr(ctx, pos, out); err != nil {
		return Null(), err
	}
	return Str(out), nil
}

func numF(ctx *Ctx, pos Pos, v Value, sig string) (float64, error) {
	n, err := numArg(ctx, pos, v, sig)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, rtErr(ctx, pos, "%s expects finite number", sig)
	}
	return n, nil
}

func splitUnix(ctx *Ctx, pos Pos, v Value, sig string) (int64, int64, error) {
	n, err := numF(ctx, pos, v, sig)
	if err != nil {
		return 0, 0, err
	}
	if n > maxI || n < minI {
		return 0, 0, rtErr(ctx, pos, "%s out of range", sig)
	}

	sec, frac := math.Modf(n)
	ns := int64(math.Round(frac * float64(nsSec)))
	if ns >= nsSec || ns <= -nsSec {
		adj := int64(1)
		if ns < 0 {
			adj = -1
		}
		sec += float64(adj)
		ns -= adj * nsSec
	}
	return int64(sec), ns, nil
}
