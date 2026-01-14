package ui

import (
	"strings"
	"testing"
)

func TestLatencyAnimTextFinal(t *testing.T) {
	got := latencyAnimText(latAnimTotal(), latCap)
	if got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestLatencyAnimTextBurst(t *testing.T) {
	got := latencyAnimText(0, latCap)
	bars, val := splitAnim(t, got)
	if n := len([]rune(bars)); n != len(latAnimSeq(0)) {
		t.Fatalf("expected %d bars, got %d (%q)", len(latAnimSeq(0)), n, bars)
	}
	if !latAnimHasUnit(val) {
		t.Fatalf("expected duration suffix, got %q", val)
	}
}

func TestLatencyAnimTextCollapse(t *testing.T) {
	start := latAnimColStart()
	base := latencyAnimText(start, latCap)
	bars, val := splitAnim(t, base)
	if !latAnimHasUnit(val) {
		t.Fatalf("expected duration suffix, got %q", val)
	}

	mid := latencyAnimText(start+latAnimCol/2, latCap)
	midBars, midVal := splitAnim(t, mid)
	if !latAnimHasUnit(midVal) {
		t.Fatalf("expected duration suffix, got %q", midVal)
	}
	if midBars == "" {
		t.Fatalf("expected bars during collapse, got empty")
	}
	if midBars == bars {
		t.Fatalf("expected bars to collapse, got %q", midBars)
	}
	if n := len([]rune(midBars)); n != len([]rune(bars)) {
		t.Fatalf("expected collapse to keep width, got %d", n)
	}
}

func splitAnim(t *testing.T, s string) (string, string) {
	t.Helper()
	bars, val, ok := strings.Cut(s, " ")
	if !ok || bars == "" || val == "" {
		t.Fatalf("expected bars and value, got %q", s)
	}
	return bars, val
}

func latAnimHasUnit(s string) bool {
	if strings.HasSuffix(s, "ms") {
		return strings.TrimSuffix(s, "ms") != ""
	}
	if strings.HasSuffix(s, "s") {
		return strings.TrimSuffix(s, "s") != ""
	}
	return false
}
