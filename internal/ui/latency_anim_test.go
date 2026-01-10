package ui

import (
	"strings"
	"testing"
)

func TestLatencyAnimTextFinal(t *testing.T) {
	got := latencyAnimText(latAnimDuration, latCap)
	if got != latencyPlaceholder {
		t.Fatalf("expected placeholder, got %q", got)
	}
}

func TestLatencyAnimTextShape(t *testing.T) {
	got := latencyAnimText(0, latCap)
	if !strings.HasSuffix(got, " ms") {
		t.Fatalf("expected ms suffix, got %q", got)
	}

	trimmed := strings.TrimSuffix(got, " ms")
	if n := len([]rune(trimmed)); n != latCap {
		t.Fatalf("expected %d bars, got %d (%q)", latCap, n, trimmed)
	}
}

func TestLatencyAnimBarsShrink(t *testing.T) {
	max := latCap
	if got := latAnimBars(0, max); got != max {
		t.Fatalf("expected max bars, got %d", got)
	}

	mid := latAnimBars(latAnimDuration/2, max)
	if mid >= max || mid < latPlaceholderBars {
		t.Fatalf("expected mid bars between %d and %d, got %d", latPlaceholderBars, max, mid)
	}
	if got := latAnimBars(latAnimDuration, max); got != latPlaceholderBars {
		t.Fatalf("expected placeholder bars, got %d", got)
	}
}
