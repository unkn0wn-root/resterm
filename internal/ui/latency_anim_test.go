package ui

import (
	"strings"
	"testing"
)

func TestLatencyAnimTextFinal(t *testing.T) {
	got := latencyAnimText(latAnimTotal(), latCap)
	if got != latencyPlaceholder {
		t.Fatalf("expected placeholder, got %q", got)
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

func TestLatencyAnimTextSettle(t *testing.T) {
	start := latAnimSettleStart()
	first, val := splitAnim(t, latencyAnimText(start, latCap))
	if val != "ms" {
		t.Fatalf("expected placeholder unit, got %q", val)
	}
	if first != latFill(len([]rune(first))) {
		t.Fatalf("expected flat bars, got %q", first)
	}
	if n := len([]rune(first)); n != len(latAnimVals) {
		t.Fatalf("expected %d bars, got %d", len(latAnimVals), n)
	}

	mid, _ := splitAnim(t, latencyAnimText(start+latAnimSettle/2, latCap))
	if n := len([]rune(mid)); n <= latPlaceholderBars || n >= len([]rune(first)) {
		t.Fatalf("expected width between %d and %d, got %d", latPlaceholderBars, len([]rune(first)), n)
	}

	end := latencyAnimText(start+latAnimSettle, latCap)
	if end != latencyPlaceholder {
		t.Fatalf("expected placeholder, got %q", end)
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
