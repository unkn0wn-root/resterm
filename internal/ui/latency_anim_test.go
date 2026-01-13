package ui

import (
	"strings"
	"testing"
	"time"
)

func TestLatencyAnimTextFinal(t *testing.T) {
	got := latencyAnimText(latAnimTotalDuration-time.Millisecond, latCap)
	if got != latencyPlaceholder {
		t.Fatalf("expected placeholder, got %q", got)
	}
	got = latencyAnimText(latAnimTotalDuration, latCap)
	if got != latencyPlaceholder {
		t.Fatalf("expected placeholder, got %q", got)
	}
}

func TestLatencyAnimTextShape(t *testing.T) {
	got := latencyAnimText(0, latCap)
	bars, val := splitAnim(t, got)
	if n := len([]rune(bars)); n != latCap {
		t.Fatalf("expected %d bars, got %d (%q)", latCap, n, bars)
	}
	if !latAnimHasUnit(val) {
		t.Fatalf("expected duration suffix, got %q", val)
	}
}

func TestLatencyAnimTextFade(t *testing.T) {
	mid := latAnimDuration + latAnimFadeDuration/2
	got := latencyAnimText(mid, latCap)
	if got == "" {
		t.Fatalf("expected fade text, got empty")
	}
	bars, val := splitAnim(t, got)
	if n := len([]rune(bars)); n != latCap {
		t.Fatalf("expected %d bars, got %d (%q)", latCap, n, bars)
	}
	if !latAnimHasUnit(val) {
		t.Fatalf("expected duration suffix, got %q", val)
	}
}

func TestLatencyAnimTextDown(t *testing.T) {
	el := latAnimFadeEnd + latAnimPlaceDuration/2
	got := latencyAnimText(el, latCap)
	bars, val := splitAnim(t, got)
	if n := len([]rune(bars)); n >= latCap || n < latPlaceholderBars {
		t.Fatalf("expected bars shrink, got %d (%q)", n, bars)
	}
	if !strings.HasSuffix(val, "ms") {
		t.Fatalf("expected ms suffix, got %q", val)
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
