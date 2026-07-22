package ui

import (
	"testing"
	"time"
)

func TestLatencySeriesSummaryEmpty(t *testing.T) {
	s := newLatencySeries(4)
	if _, ok := s.summary(); ok {
		t.Fatal("expected no summary without samples")
	}
}

func TestLatencySeriesSummaryPadsYoungSeries(t *testing.T) {
	s := newLatencySeries(10)
	s.add(1 * time.Millisecond)
	s.add(4 * time.Millisecond)

	sum := requireLatencySummary(t, s)
	bars := []rune(sum.bars)
	if got := len(bars); got != latMinBars {
		t.Fatalf("expected %d bars, got %d (%q)", latMinBars, got, sum.bars)
	}
	for _, bar := range bars[:latMinBars-2] {
		if bar != latLevels[0] {
			t.Fatalf("expected flat padding, got %q", sum.bars)
		}
	}
	if sum.cur != 4*time.Millisecond {
		t.Fatalf("expected current latency 4ms, got %s", sum.cur)
	}
	if sum.p95 != 4*time.Millisecond {
		t.Fatalf("expected p95 latency 4ms, got %s", sum.p95)
	}
}

func TestLatencySeriesSummarySingleSample(t *testing.T) {
	s := newLatencySeries(4)
	s.add(50 * time.Millisecond)

	sum := requireLatencySummary(t, s)
	bars := []rune(sum.bars)
	if got := len(bars); got != latMinBars {
		t.Fatalf("expected %d bars, got %d (%q)", latMinBars, got, sum.bars)
	}
	if bars[len(bars)-1] == latLevels[0] {
		t.Fatalf("expected a raised bar for the sample, got %q", sum.bars)
	}
}

func TestLatencySeriesSummaryGrowsWithSamples(t *testing.T) {
	s := newLatencySeries(10)
	s.add(10 * time.Millisecond)
	if got := len([]rune(requireLatencySummary(t, s).bars)); got != latMinBars {
		t.Fatalf("expected %d bars, got %d", latMinBars, got)
	}

	for range 5 {
		s.add(10 * time.Millisecond)
	}
	if got := len([]rune(requireLatencySummary(t, s).bars)); got != 6 {
		t.Fatalf("expected 6 bars, got %d", got)
	}
}

func TestLatencySeriesSummaryLimitsVisibleBars(t *testing.T) {
	s := newLatencySeries(20)
	for i := 1; i <= 20; i++ {
		s.add(time.Duration(i) * time.Millisecond)
	}

	sum := requireLatencySummary(t, s)
	if got := len([]rune(sum.bars)); got != latBarsCap {
		t.Fatalf("expected %d visible bars, got %d", latBarsCap, got)
	}
	if sum.cur != 20*time.Millisecond {
		t.Fatalf("expected current latency 20ms, got %s", sum.cur)
	}
	if sum.p95 != 19*time.Millisecond {
		t.Fatalf("expected p95 latency 19ms, got %s", sum.p95)
	}
}

func TestLatencySeriesSummaryScalesBarsToVisibleWindow(t *testing.T) {
	s := newLatencySeries(latCap)
	for range 90 {
		s.add(10 * time.Millisecond)
	}
	for i := 1; i <= 10; i++ {
		s.add(time.Duration(i*100) * time.Millisecond)
	}

	sum := requireLatencySummary(t, s)
	bars := []rune(sum.bars)
	if bars[0] != latLevels[0] || bars[len(bars)-1] != latLevels[len(latLevels)-1] {
		t.Fatalf("expected bars scaled to the visible tail, got %q", sum.bars)
	}
	if sum.p95 != 500*time.Millisecond {
		t.Fatalf("expected p95 over the full window, got %s", sum.p95)
	}
}

func TestLatencySeriesReset(t *testing.T) {
	s := newLatencySeries(4)
	s.add(time.Millisecond)
	gen := s.gen
	s.reset()

	if _, ok := s.summary(); ok {
		t.Fatal("expected no summary after reset")
	}
	if s.gen == gen {
		t.Fatal("expected reset to start a new generation")
	}
}

func TestLatCurveLiftsMidrange(t *testing.T) {
	if got := latCurve(0.25); got <= 0.25 {
		t.Fatalf("expected midrange values to be lifted, got %f", got)
	}
	if got := latCurve(0); got != 0 {
		t.Fatalf("expected 0 to remain unchanged, got %f", got)
	}
	if got := latCurve(1); got != 1 {
		t.Fatalf("expected 1 to remain unchanged, got %f", got)
	}
}

func TestLatencySeriesDiscardsOldestSamples(t *testing.T) {
	s := newLatencySeries(2)
	s.add(1 * time.Millisecond)
	s.add(2 * time.Millisecond)
	s.add(3 * time.Millisecond)

	want := []time.Duration{2 * time.Millisecond, 3 * time.Millisecond}
	if len(s.vals) != len(want) {
		t.Fatalf("expected %d samples, got %d", len(want), len(s.vals))
	}
	for i := range want {
		if s.vals[i] != want[i] {
			t.Fatalf("expected samples %v, got %v", want, s.vals)
		}
	}
}

func TestLatencySeriesSummaryRendersSparkline(t *testing.T) {
	s := newLatencySeries(5)
	for i := 1; i <= 5; i++ {
		s.add(time.Duration(i) * time.Millisecond)
	}

	sum := requireLatencySummary(t, s)
	if sum.bars != "▁▃▅▇█" {
		t.Fatalf("expected sparkline %q, got %q", "▁▃▅▇█", sum.bars)
	}
	if sum.cur != 5*time.Millisecond || sum.p95 != 5*time.Millisecond {
		t.Fatalf("expected 5ms current and p95, got %s and %s", sum.cur, sum.p95)
	}
}

func TestLatencySeriesSummaryUsesRollingP95(t *testing.T) {
	s := newLatencySeries(10)
	for i := 2; i <= 10; i++ {
		s.add(time.Duration(i*10) * time.Millisecond)
	}
	s.add(10 * time.Millisecond)

	sum := requireLatencySummary(t, s)
	if sum.cur != 10*time.Millisecond {
		t.Fatalf("expected current latency 10ms, got %s", sum.cur)
	}
	if sum.p95 != 100*time.Millisecond {
		t.Fatalf("expected p95 latency 100ms, got %s", sum.p95)
	}
}

func TestPercentile(t *testing.T) {
	vals := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
	}
	if got := percentile(vals, 50); got != 20*time.Millisecond {
		t.Fatalf("expected median rank 20ms, got %s", got)
	}
	if got := percentile(vals, 95); got != 40*time.Millisecond {
		t.Fatalf("expected p95 40ms, got %s", got)
	}
}

func requireLatencySummary(t *testing.T, s *latencySeries) latencySummary {
	t.Helper()
	sum, ok := s.summary()
	if !ok {
		t.Fatal("expected latency summary")
	}
	return sum
}
