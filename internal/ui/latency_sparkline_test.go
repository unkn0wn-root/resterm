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
