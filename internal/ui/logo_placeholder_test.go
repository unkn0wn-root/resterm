package ui

import (
	"strings"
	"testing"
)

func TestLogoPlaceholderContentCenters(t *testing.T) {
	width := noResponseWordmarkWidth + 10
	lines := strings.Split(logoPlaceholder(width, 0), "\n")
	if len(lines) != len(noResponseLogo) {
		t.Fatalf("expected %d lines, got %d", len(noResponseLogo), len(lines))
	}

	for i, line := range lines {
		want := (width-noResponseWordmarkWidth)/2 + visibleWidth(leadingIndent(noResponseLogo[i]))
		got := visibleWidth(leadingIndent(line))
		if got != want {
			t.Fatalf("line %d padding: want %d, got %d", i, want, got)
		}
	}
}

func TestNoResponseWordmarkRows(t *testing.T) {
	if end := noResponseWordmarkRow + noResponseWordmarkHeight; end > len(noResponseLogo) {
		t.Fatalf("wordmark ends on row %d, but the logo only has %d rows", end, len(noResponseLogo))
	}

	for i, line := range noResponseLogo {
		want := i >= noResponseWordmarkRow && i < noResponseWordmarkRow+noResponseWordmarkHeight
		if strings.HasPrefix(line, "░") != want {
			t.Fatalf("logo row %d is %q, but the row constants say wordmark=%v", i, line, want)
		}
	}
}

func TestLogoPlaceholderCentersWordmarkVertically(t *testing.T) {
	const height = 15
	lines := strings.Split(logoPlaceholder(noResponseWordmarkWidth+10, height), "\n")
	mark := noResponseLogo[noResponseWordmarkRow]

	got := -1
	for i, line := range lines {
		if strings.Contains(line, mark) {
			got = i
			break
		}
	}
	want := (height - noResponseWordmarkHeight) / 2
	if got != want {
		t.Fatalf("wordmark starts on row %d, want %d", got, want)
	}
}

func TestLogoPlaceholderFitsNarrowPanes(t *testing.T) {
	for width := 1; width <= noResponseWordmarkWidth+10; width++ {
		for i, line := range strings.Split(logoPlaceholder(width, 12), "\n") {
			if got := visibleWidth(line); got > width {
				t.Fatalf("pane width %d: line %d is %d columns: %q", width, i, got, line)
			}
		}
	}
}

func TestLogoPlaceholderCacheMapping(t *testing.T) {
	width := 80
	cache := logoPlaceholderCache(width, 0)
	if !cache.valid {
		t.Fatalf("expected cache to be valid")
	}
	if cache.width != width {
		t.Fatalf("expected cache width %d, got %d", width, cache.width)
	}
	lines := strings.Split(cache.content, "\n")
	if len(lines) != len(cache.spans) || len(lines) != len(cache.rev) {
		t.Fatalf(
			"expected cache mappings for %d lines, got spans=%d rev=%d",
			len(lines),
			len(cache.spans),
			len(cache.rev),
		)
	}
	for i := range lines {
		if cache.spans[i].start != i || cache.spans[i].end != i {
			t.Fatalf(
				"span %d: want %d..%d, got %d..%d",
				i,
				i,
				i,
				cache.spans[i].start,
				cache.spans[i].end,
			)
		}
		if cache.rev[i] != i {
			t.Fatalf("rev %d: want %d, got %d", i, i, cache.rev[i])
		}
	}
}
