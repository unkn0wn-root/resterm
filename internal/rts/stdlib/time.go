package stdlib

import (
	"math"
	"strconv"
	"time"

	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/rts"
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

var timeSpec = nsSpec{name: "time", top: true, fns: map[string]rts.NativeFunc{
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

func timeNowISO(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTimeNowISO)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	t, err := nowT(ctx, pos)
	if err != nil {
		return rts.Null(), err
	}
	return fmtTime(ctx, pos, t.UTC(), time.RFC3339)
}

func timeNowUnix(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTimeNowUnix)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}

	t, err := nowT(ctx, pos)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Num(float64(t.Unix())), nil
}

func timeNowUnixStr(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTimeNowUnixString)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}

	t, err := nowT(ctx, pos)
	if err != nil {
		return rts.Null(), err
	}
	s := strconv.FormatInt(t.Unix(), 10)
	if err := rts.CheckStr(ctx, pos, s); err != nil {
		return rts.Null(), err
	}
	return rts.Str(s), nil
}

func timeNowUnixMs(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTimeNowUnixMs)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}

	t, err := nowT(ctx, pos)
	if err != nil {
		return rts.Null(), err
	}

	n := t.UnixNano() / int64(time.Millisecond)
	return rts.Num(float64(n)), nil
}

func timeFormat(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTimeFormat)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}
	t, err := nowT(ctx, pos)
	if err != nil {
		return rts.Null(), err
	}
	layout, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}
	return fmtTime(ctx, pos, t, layout)
}

func timeParse(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTimeParse)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	layout, err := na.Str(0)
	if err != nil {
		return rts.Null(), err
	}

	val, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}

	t, err := time.Parse(layout, val)
	if err != nil {
		return rts.Null(), rts.Errf(ctx, pos, "time parse failed")
	}

	sec := float64(t.UnixNano()) / float64(time.Second)
	return rts.Num(sec), nil
}

func timeFormatUnix(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTimeFormatUnix)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	layout, err := na.Str(1)
	if err != nil {
		return rts.Null(), err
	}

	sec, ns, err := splitUnix(ctx, pos, na.Arg(0), sigTimeFormatUnix)
	if err != nil {
		return rts.Null(), err
	}

	t := time.Unix(sec, ns).UTC()
	return fmtTime(ctx, pos, t, layout)
}

func timeAddUnix(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTimeAddUnix)
	if err := na.Count(2); err != nil {
		return rts.Null(), err
	}

	a, err := na.FiniteNum(0)
	if err != nil {
		return rts.Null(), err
	}

	b, err := durationSecondsArg(ctx, pos, na.Arg(1), sigTimeAddUnix)
	if err != nil {
		return rts.Null(), err
	}

	out := a + b
	if math.IsNaN(out) || math.IsInf(out, 0) {
		return rts.Null(), rts.Errf(ctx, pos, "%s expects finite number", sigTimeAddUnix)
	}

	return rts.Num(out), nil
}

func timeDuration(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTimeDuration)
	if err := na.Count(1); err != nil {
		return rts.Null(), err
	}
	sec, err := durationSecondsArg(ctx, pos, na.Arg(0), sigTimeDuration)
	if err != nil {
		return rts.Null(), err
	}
	return rts.Num(sec), nil
}

func nowT(ctx *rts.Ctx, pos rts.Pos) (time.Time, error) {
	if ctx == nil || ctx.Now == nil {
		return time.Time{}, rts.Errf(ctx, pos, "time not available")
	}
	return ctx.Now(), nil
}

func fmtTime(ctx *rts.Ctx, pos rts.Pos, t time.Time, layout string) (rts.Value, error) {
	out := t.Format(layout)
	if err := rts.CheckStr(ctx, pos, out); err != nil {
		return rts.Null(), err
	}
	return rts.Str(out), nil
}

func numF(ctx *rts.Ctx, pos rts.Pos, v rts.Value, sig string) (float64, error) {
	if v.K != rts.VNum {
		return 0, rts.Errf(ctx, pos, "%s expects number", sig)
	}
	n := v.N
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, rts.Errf(ctx, pos, "%s expects finite number", sig)
	}
	return n, nil
}

func durationSecondsArg(ctx *rts.Ctx, pos rts.Pos, v rts.Value, sig string) (float64, error) {
	switch v.K {
	case rts.VNum:
		return numF(ctx, pos, v, sig)
	case rts.VStr:
		dur, ok := duration.Parse(v.S)
		if !ok {
			return 0, rts.Errf(ctx, pos, "%s expects duration string", sig)
		}
		return dur.Seconds(), nil
	default:
		return 0, rts.Errf(ctx, pos, "%s expects number or duration string", sig)
	}
}

func splitUnix(ctx *rts.Ctx, pos rts.Pos, v rts.Value, sig string) (int64, int64, error) {
	n, err := numF(ctx, pos, v, sig)
	if err != nil {
		return 0, 0, err
	}
	const (
		maxInt64Float = float64(^uint64(0) >> 1)
		minInt64Float = -maxInt64Float - 1
	)
	if n > maxInt64Float || n < minInt64Float {
		return 0, 0, rts.Errf(ctx, pos, "%s out of range", sig)
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
