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
	latCap             = 10
	latPlaceholderBars = 4
	latWarmN           = 3
	latWarmDiv         = 5
	latGamma           = 0.75
)

var (
	latencyLevels      = []rune("▁▂▄▆█")
	latencyPlaceholder = latPlaceholder(latPlaceholderBars)
)

func newLatencySeries(capacity int) *latencySeries {
	if capacity < 1 {
		capacity = 1
	}
	return &latencySeries{cap: capacity}
}

func (s *latencySeries) add(d time.Duration) {
	if s == nil || d <= 0 {
		return
	}
	if s.cap < 1 {
		s.cap = 1
	}

	s.vals = append(s.vals, d)
	if len(s.vals) > s.cap {
		delta := len(s.vals) - s.cap
		s.vals = s.vals[delta:]
	}
}

func (s *latencySeries) empty() bool {
	return s == nil || len(s.vals) == 0
}

func (s *latencySeries) last() (time.Duration, bool) {
	if s == nil || len(s.vals) == 0 {
		return 0, false
	}
	return s.vals[len(s.vals)-1], true
}

func (s *latencySeries) render() string {
	if s == nil {
		return ""
	}
	if len(s.vals) == 0 {
		return latencyPlaceholder
	}

	min, max := s.bounds()
	width := latWidth(s.cap, len(s.vals))
	bars := sparkline(s.vals, min, max)
	if pad := width - len(s.vals); pad > 0 {
		bars = latFill(pad) + bars
	}
	v, _ := s.last()
	rounded := v.Round(time.Millisecond)
	if rounded <= 0 {
		rounded = v
	}
	return bars + " " + formatDurationShort(rounded)
}

func (s *latencySeries) bounds() (time.Duration, time.Duration) {
	if len(s.vals) == 0 {
		return 0, 0
	}
	if len(s.vals) == 1 {
		v := s.vals[0]
		return 0, v
	}

	sorted := append([]time.Duration(nil), s.vals...)
	slices.Sort(sorted)
	lo := percentile(sorted, 10)
	hi := percentile(sorted, 90)
	if hi <= lo {
		return sorted[0], sorted[len(sorted)-1]
	}
	return latClamp(lo, hi, len(s.vals))
}

func percentile(vals []time.Duration, pct int) time.Duration {
	if len(vals) == 0 {
		return 0
	}
	if pct <= 0 {
		return vals[0]
	}
	if pct >= 100 {
		return vals[len(vals)-1]
	}

	pos := (float64(pct) / 100.0) * float64(len(vals)-1)
	idx := int(pos + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(vals) {
		idx = len(vals) - 1
	}
	return vals[idx]
}

func sparkline(vals []time.Duration, min, max time.Duration) string {
	if len(vals) == 0 {
		return ""
	}

	levels := latencyLevels
	if max <= min {
		return strings.Repeat(string(levels[0]), len(vals))
	}

	scale := float64(max - min)
	out := make([]rune, len(vals))
	for i, v := range vals {
		if v < min {
			v = min
		}
		if v > max {
			v = max
		}

		n := latCurve(float64(v-min) / scale)
		idx := int(n*float64(len(levels)-1) + 0.5)
		if idx < 0 {
			idx = 0
		}
		if idx >= len(levels) {
			idx = len(levels) - 1
		}
		out[i] = levels[idx]
	}
	return string(out)
}

func latFill(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(string(latencyLevels[0]), n)
}

func latPlaceholder(n int) string {
	if n < 1 {
		n = 1
	}
	return latFill(n) + " ms"
}

func latWidth(capacity, count int) int {
	if capacity < 1 {
		capacity = 1
	}
	if count < 0 {
		count = 0
	}

	width := count
	if width < latPlaceholderBars {
		width = latPlaceholderBars
	}
	if width > capacity {
		width = capacity
	}
	return width
}

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
	if lo < 0 {
		lo = 0
	}
	return lo, hi
}

func latCurve(n float64) float64 {
	if n <= 0 {
		return 0
	}
	if n >= 1 {
		return 1
	}
	return math.Pow(n, latGamma)
}
