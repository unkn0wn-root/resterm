package ui

import (
	"sort"
	"strings"
	"time"
)

type latencySeries struct {
	vals []time.Duration
	cap  int
}

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

func (s *latencySeries) render() string {
	if s == nil {
		return ""
	}
	if len(s.vals) == 0 {
		return "⏱"
	}
	min, max := s.bounds()
	return sparkline(s.vals, min, max) + " " + formatDurationShort(s.vals[len(s.vals)-1])
}

func (s *latencySeries) bounds() (time.Duration, time.Duration) {
	if len(s.vals) == 0 {
		return 0, 0
	}
	if len(s.vals) == 1 {
		v := s.vals[0]
		return v, v
	}
	sorted := append([]time.Duration(nil), s.vals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	lo := percentile(sorted, 10)
	hi := percentile(sorted, 90)
	if hi <= lo {
		return sorted[0], sorted[len(sorted)-1]
	}
	return lo, hi
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
	levels := []rune("▁▂▄▆█")
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
		n := float64(v-min) / scale
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
