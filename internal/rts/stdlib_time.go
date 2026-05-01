package rts

import (
	"math"
	"strconv"
	"time"

	"github.com/unkn0wn-root/resterm/internal/duration"
)

const (
	sigTimeNowISO        = "time.nowISO()"
	sigTimeNowUnix       = "time.nowUnix()"
	sigTimeNowUnixString = "time.nowUnixString()"
	sigTimeNowUnixMs     = "time.nowUnixMs()"
	sigTimeFormat        = "time.format(layout)"
	sigTimeParse         = "time.parse(layout, value)"
	sigTimeFormatUnix    = "time.formatUnix(ts, layout)"
	sigTimeAddUnix       = "time.addUnix(ts, seconds)"
	sigTimeDuration      = "time.duration(value)"

	nsSec = int64(time.Second)
)

var timeSpec = nsSpec{name: "time", top: true, fns: map[string]NativeFunc{
	"nowISO":        timeNowISO,
	"nowUnix":       timeNowUnix,
	"nowUnixString": timeNowUnixStr,
	"nowUnixMs":     timeNowUnixMs,
	"format":        timeFormat,
	"parse":         timeParse,
	"formatUnix":    timeFormatUnix,
	"addUnix":       timeAddUnix,
	"duration":      timeDuration,
}}

func timeNowISO(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTimeNowISO)
	if err := na.none(); err != nil {
		return Null(), err
	}
	t, err := nowT(ctx, pos)
	if err != nil {
		return Null(), err
	}
	return fmtTime(ctx, pos, t.UTC(), time.RFC3339)
}

func timeNowUnix(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTimeNowUnix)
	if err := na.none(); err != nil {
		return Null(), err
	}

	t, err := nowT(ctx, pos)
	if err != nil {
		return Null(), err
	}
	return Num(float64(t.Unix())), nil
}

func timeNowUnixStr(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTimeNowUnixString)
	if err := na.none(); err != nil {
		return Null(), err
	}

	t, err := nowT(ctx, pos)
	if err != nil {
		return Null(), err
	}
	s := strconv.FormatInt(t.Unix(), 10)
	if err := chkStr(ctx, pos, s); err != nil {
		return Null(), err
	}
	return Str(s), nil
}

func timeNowUnixMs(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTimeNowUnixMs)
	if err := na.none(); err != nil {
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
	na := newNativeArgs(ctx, pos, args, sigTimeFormat)
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
	na := newNativeArgs(ctx, pos, args, sigTimeParse)
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
	na := newNativeArgs(ctx, pos, args, sigTimeFormatUnix)
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
	na := newNativeArgs(ctx, pos, args, sigTimeAddUnix)
	if err := na.count(2); err != nil {
		return Null(), err
	}

	a, err := na.finiteNum(0)
	if err != nil {
		return Null(), err
	}

	b, err := durationSecondsArg(ctx, pos, na.arg(1), na.sig)
	if err != nil {
		return Null(), err
	}

	out := a + b
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return Null(), rtErr(ctx, pos, "%s expects finite number", na.sig)
	}

	return Num(out), nil
}

func timeDuration(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTimeDuration)
	if err := na.count(1); err != nil {
		return Null(), err
	}
	sec, err := durationSecondsArg(ctx, pos, na.arg(0), na.sig)
	if err != nil {
		return Null(), err
	}
	return Num(sec), nil
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

func durationSecondsArg(ctx *Ctx, pos Pos, v Value, sig string) (float64, error) {
	switch v.K {
	case VNum:
		return numF(ctx, pos, v, sig)
	case VStr:
		dur, ok := duration.Parse(v.S)
		if !ok {
			return 0, rtErr(ctx, pos, "%s expects duration string", sig)
		}
		return dur.Seconds(), nil
	default:
		return 0, rtErr(ctx, pos, "%s expects number or duration string", sig)
	}
}

func splitUnix(ctx *Ctx, pos Pos, v Value, sig string) (int64, int64, error) {
	n, err := numF(ctx, pos, v, sig)
	if err != nil {
		return 0, 0, err
	}
	const (
		maxInt64Float = float64(^uint64(0) >> 1)
		minInt64Float = -maxInt64Float - 1
	)
	if n > maxInt64Float || n < minInt64Float {
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
