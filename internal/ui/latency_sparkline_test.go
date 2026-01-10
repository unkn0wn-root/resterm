package ui

import (
	"strings"
	"testing"
	"time"
)

func TestLatencySeriesRenderPlaceholder(t *testing.T) {
	s := newLatencySeries(4)
	if got := s.render(); got != latencyPlaceholder {
		t.Fatalf("expected placeholder, got %q", got)
	}
}

func TestLatencySeriesRenderPadsToCap(t *testing.T) {
	s := newLatencySeries(4)
	s.add(1 * time.Millisecond)
	s.add(4 * time.Millisecond)
	got := s.render()
	parts := strings.SplitN(got, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected latency format, got %q", got)
	}

	bars := parts[0]
	if n := len([]rune(bars)); n != 4 {
		t.Fatalf("expected 4 bars, got %d (%q)", n, bars)
	}
	if !strings.HasPrefix(bars, latFill(2)) {
		t.Fatalf("expected padded bars, got %q", bars)
	}
	if !strings.HasSuffix(got, "4ms") {
		t.Fatalf("expected last duration suffix, got %q", got)
	}
}

func TestLatencySeriesRenderSingleSample(t *testing.T) {
	s := newLatencySeries(4)
	s.add(50 * time.Millisecond)
	got := s.render()
	parts := strings.SplitN(got, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected latency format, got %q", got)
	}

	bars := []rune(parts[0])
	if n := len(bars); n != 4 {
		t.Fatalf("expected 4 bars, got %d (%q)", n, parts[0])
	}
	if bars[len(bars)-1] == latencyLevels[0] {
		t.Fatalf("expected bar for first sample, got %q", parts[0])
	}
}

func TestLatencySeriesRenderGrowsFromPlaceholder(t *testing.T) {
	s := newLatencySeries(10)
	s.add(10 * time.Millisecond)
	got := s.render()
	parts := strings.SplitN(got, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected latency format, got %q", got)
	}
	if n := len([]rune(parts[0])); n != latPlaceholderBars {
		t.Fatalf("expected %d bars, got %d (%q)", latPlaceholderBars, n, parts[0])
	}

	for i := 0; i < 5; i++ {
		s.add(10 * time.Millisecond)
	}

	got = s.render()
	parts = strings.SplitN(got, " ", 2)
	if len(parts) != 2 {
		t.Fatalf("expected latency format, got %q", got)
	}
	if n := len([]rune(parts[0])); n != 6 {
		t.Fatalf("expected 6 bars, got %d (%q)", n, parts[0])
	}
}

func TestLatCurveLiftsMid(t *testing.T) {
	if got := latCurve(0.25); got <= 0.25 {
		t.Fatalf("expected mid values lifted, got %f", got)
	}
	if got := latCurve(0); got != 0 {
		t.Fatalf("expected 0 to remain, got %f", got)
	}
	if got := latCurve(1); got != 1 {
		t.Fatalf("expected 1 to remain, got %f", got)
	}
}

func TestLatencySeriesRolls(t *testing.T) {
	s := newLatencySeries(2)
	s.add(1 * time.Millisecond)
	s.add(2 * time.Millisecond)
	s.add(3 * time.Millisecond)
	if len(s.vals) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(s.vals))
	}
	if s.vals[0] != 2*time.Millisecond || s.vals[1] != 3*time.Millisecond {
		t.Fatalf("unexpected samples: %v", s.vals)
	}
}

func TestLatencySeriesRenderSparkline(t *testing.T) {
	s := newLatencySeries(5)
	for i := 1; i <= 5; i++ {
		s.add(time.Duration(i) * time.Millisecond)
	}

	got := s.render()
	if !strings.HasPrefix(got, "▁▂▄▆█ ") {
		t.Fatalf("expected sparkline prefix, got %q", got)
	}
	if !strings.HasSuffix(got, "5ms") {
		t.Fatalf("expected last duration suffix, got %q", got)
	}
}
