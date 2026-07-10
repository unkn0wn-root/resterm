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

type latencySummary struct {
	hist string
	last rune
	cur  time.Duration
	p95  time.Duration
}

const (
	// Keep the statistics window wider than the sparkline so p95 remains useful
	// without allowing the header visualization to grow indefinitely.
	latCap     = 100
	latBarsCap = 10
	latMinBars = 5
	latWarmN   = 3
	latWarmDiv = 5
	latGamma   = 0.75
	latP95     = 95
	latP95Sep  = " · p95 "
)

var (
	latLevels      = []rune("▁▂▃▄▅▆▇█")
	latRamp        = []rune("▁▂▄▆█")
	latPlaceholder = string(latRamp) + " ms"
)

func (m Model) latencyText() string {
	if s, ok := m.latencySeries.summary(); ok {
		return formatLatencySummary(s)
	}
	if m.latAnimOn {
		return latClimb(m.latAnimP()) + " ms"
	}
	return latPlaceholder
}

func newLatencySeries(cap int) *latencySeries {
	if cap < 1 {
		cap = 1
	}
	return &latencySeries{cap: cap}
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

func (s *latencySeries) summary() (latencySummary, bool) {
	if s.empty() {
		return latencySummary{}, false
	}

	vals := slices.Clone(s.vals)
	slices.Sort(vals)
	lo, hi := latBounds(vals)

	tail := s.vals
	if len(tail) > latBarsCap {
		tail = tail[len(tail)-latBarsCap:]
	}
	bars := sparkline(tail, lo, hi)
	if pad := latMinBars - len(tail); pad > 0 {
		bars = latFill(pad) + bars
	}
	rs := []rune(bars)
	last := len(rs) - 1

	return latencySummary{
		hist: string(rs[:last]),
		last: rs[last],
		cur:  s.vals[len(s.vals)-1],
		p95:  percentile(vals, latP95),
	}, true
}

func (s latencySummary) bars() string {
	return s.hist + string(s.last)
}

func formatLatencySummary(s latencySummary) string {
	return s.bars() + " " + formatLatencyDuration(s.cur) +
		latP95Sep + formatLatencyDuration(s.p95)
}

func latBounds(vals []time.Duration) (time.Duration, time.Duration) {
	if len(vals) == 1 {
		return 0, vals[0]
	}

	lo := percentile(vals, 10)
	hi := percentile(vals, 90)
	if hi <= lo {
		return vals[0], vals[len(vals)-1]
	}
	return latClamp(lo, hi, len(vals))
}

// percentile expects vals sorted in ascending order and pct in [1, 100].
func percentile(vals []time.Duration, pct int) time.Duration {
	rank := int(math.Ceil(float64(pct)/100*float64(len(vals)))) - 1
	return vals[rank]
}

func formatLatencyDuration(d time.Duration) string {
	if rounded := d.Round(time.Millisecond); rounded > 0 {
		d = rounded
	}
	return formatDurationShort(d)
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
		level := int(n*float64(len(latLevels)-1) + 0.5)
		out[i] = latLevels[level]
	}
	return string(out)
}

func latFill(n int) string {
	return strings.Repeat(string(latLevels[0]), n)
}

// latClamp widens a too-narrow percentile span while the series is
// young so the first bars don't exaggerate tiny differences.
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
