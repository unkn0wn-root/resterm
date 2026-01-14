package rts

import (
	"math"
	"time"
)

var timeSpec = nsSpec{name: "time", top: true, fns: map[string]NativeFunc{
	"nowISO":     timeNowISO,
	"nowUnix":    timeNowUnix,
	"nowUnixMs":  timeNowUnixMs,
	"format":     timeFormat,
	"parse":      timeParse,
	"formatUnix": timeFormatUnix,
	"addUnix":    timeAddUnix,
}}

const (
	maxI  = float64(^uint64(0) >> 1)
	minI  = -maxI - 1
	nsSec = int64(time.Second)
)

func timeNowISO(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "time.nowISO()")
	if err := na.count(0); err != nil {
		return Null(), err
	}
	t, err := nowT(ctx, pos)
	if err != nil {
		return Null(), err
	}
	return fmtTime(ctx, pos, t.UTC(), time.RFC3339)
}

func timeNowUnix(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "time.nowUnix()")
	if err := na.count(0); err != nil {
		return Null(), err
	}

	t, err := nowT(ctx, pos)
	if err != nil {
		return Null(), err
	}
	return Num(float64(t.Unix())), nil
}

func timeNowUnixMs(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "time.nowUnixMs()")
	if err := na.count(0); err != nil {
		return Null(), err
	}

	t, err := nowT(ctx, pos)
	if err != nil {
		return Null(), err
	}

	n := t.UnixNano() / int64(time.Millisecond)
	return Num(float64(n)), nil
}

func timeFormat(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "time.format(layout)")
	if err := na.count(1); err != nil {
		return Null(), err
	}
	t, err := nowT(ctx, pos)
	if err != nil {
		return Null(), err
	}
	layout, err := na.str(0)
	if err != nil {
		return Null(), err
	}
	return fmtTime(ctx, pos, t, layout)
}

func timeParse(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "time.parse(layout, value)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	layout, err := na.str(0)
	if err != nil {
		return Null(), err
	}

	val, err := na.str(1)
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

func timeFormatUnix(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "time.formatUnix(ts, layout)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	layout, err := na.str(1)
	if err != nil {
		return Null(), err
	}

	sec, ns, err := splitUnix(ctx, pos, na.arg(0), na.sig)
	if err != nil {
		return Null(), err
	}

	t := time.Unix(sec, ns).UTC()
	return fmtTime(ctx, pos, t, layout)
}

func timeAddUnix(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, "time.addUnix(ts, seconds)")
	if err := na.count(2); err != nil {
		return Null(), err
	}

	a, err := numF(ctx, pos, na.arg(0), na.sig)
	if err != nil {
		return Null(), err
	}

	b, err := numF(ctx, pos, na.arg(1), na.sig)
	if err != nil {
		return Null(), err
	}

	out := a + b
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return Null(), rtErr(ctx, pos, "%s expects finite number", na.sig)
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
