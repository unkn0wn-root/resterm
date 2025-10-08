package ui

import (
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

func TestTruncateStatusMaintainsUTF8(t *testing.T) {
	input := "Env: 東京dev"
	truncated := truncateStatus(input, 9)
	if !utf8.ValidString(truncated) {
		t.Fatalf("truncateStatus returned invalid UTF-8: %q", truncated)
	}
}

func TestTruncateStatusTooSmallForContent(t *testing.T) {
	width := 2
	got := truncateStatus("abcd", width)
	if got != "…" {
		t.Fatalf("expected ellipsis when width %d is too small, got %q", width, got)
	}
}

func TestTruncateStatusStaysWithinWidth(t *testing.T) {
	width := 6
	got := truncateStatus("Env: prod", width)
	maxWidth := maxInt(width-2, 1)
	if lipgloss.Width(got) > maxWidth {
		t.Fatalf("truncateStatus exceeded max width %d: %q (width %d)", maxWidth, got, lipgloss.Width(got))
	}
}
