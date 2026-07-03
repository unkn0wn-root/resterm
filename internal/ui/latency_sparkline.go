package ui

import (
	"math"
	"slices"
	"strings"
	"time"
)

type latencySeries struct {
	vals []time.Duration
	cap  int
}

const (
	latCap     = 10
	latMinBars = 5
	latWarmN   = 3
	latWarmDiv = 5
	latGamma   = 0.75
)

var (
	latencyLevels  = []rune("▁▂▄▆█")
	latPlaceholder = string(latencyLevels) + " ms"
)

func (m Model) latencyText() string {
	if m.latencySeries.empty() {
		return latPlaceholder
	}
	return m.latencySeries.render()
}

func newLatencySeries(capacity int) *latencySeries {
	if capacity < 1 {
		capacity = 1
	}
	return &latencySeries{cap: capacity}
}

func (s *latencySeries) add(d time.Duration) {
	if d <= 0 {
		return
	}
	s.vals = append(s.vals, d)
	if len(s.vals) > s.cap {
		s.vals = s.vals[len(s.vals)-s.cap:]
	}
}

func (s *latencySeries) empty() bool {
	return s == nil || len(s.vals) == 0
}

func (s *latencySeries) last() time.Duration {
	return s.vals[len(s.vals)-1]
}

func (s *latencySeries) render() string {
	if len(s.vals) == 0 {
		return ""
	}

	lo, hi := s.bounds()
	bars := sparkline(s.vals, lo, hi)
	if pad := latMinBars - len(s.vals); pad > 0 {
		bars = latFill(pad) + bars
	}

	v := s.last()
	if r := v.Round(time.Millisecond); r > 0 {
		v = r
	}
	return bars + " " + formatDurationShort(v)
}

func (s *latencySeries) bounds() (time.Duration, time.Duration) {
	if len(s.vals) == 1 {
		return 0, s.vals[0]
	}

	sorted := slices.Clone(s.vals)
	slices.Sort(sorted)
	lo := percentile(sorted, 10)
	hi := percentile(sorted, 90)
	if hi <= lo {
		return sorted[0], sorted[len(sorted)-1]
	}
	return latClamp(lo, hi, len(s.vals))
}

func percentile(vals []time.Duration, pct int) time.Duration {
	pos := float64(pct) / 100 * float64(len(vals)-1)
	return vals[int(pos+0.5)]
}

func sparkline(vals []time.Duration, lo, hi time.Duration) string {
	if hi <= lo {
		return latFill(len(vals))
	}

	scale := float64(hi - lo)
	out := make([]rune, len(vals))
	for i, v := range vals {
		v = min(max(v, lo), hi)
		n := latCurve(float64(v-lo) / scale)
		out[i] = latencyLevels[int(n*float64(len(latencyLevels)-1)+0.5)]
	}
	return string(out)
}

func latFill(n int) string {
	return strings.Repeat(string(latencyLevels[0]), n)
}

// latClamp widens a too-narrow percentile span while the series is young so
// the first bars don't exaggerate tiny differences.
func latClamp(lo, hi time.Duration, n int) (time.Duration, time.Duration) {
	if n >= latWarmN || hi <= 0 {
		return lo, hi
	}

	span := hi - lo
	minSpan := hi / latWarmDiv
	if minSpan <= 0 || span >= minSpan {
		return lo, hi
	}

	pad := (minSpan - span) / 2
	lo -= pad
	hi += minSpan - span - pad
	return max(lo, 0), hi
}

func latCurve(n float64) float64 {
	return math.Pow(n, latGamma)
}
