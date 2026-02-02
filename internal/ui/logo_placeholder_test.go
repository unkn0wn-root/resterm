package ui

import (
	"strings"
	"testing"
)

func TestLogoPlaceholderContentCenters(t *testing.T) {
	lines := strings.Split(noResponseMessage, "\n")
	if len(lines) == 0 {
		t.Fatalf("expected logo lines")
	}

	lineWidth := visibleWidth(strings.TrimRight(lines[0], " "))
	if lineWidth == 0 {
		t.Fatalf("expected non-zero logo width")
	}

	width := lineWidth + 10
	content := logoPlaceholder(width, 0)
	gotLines := strings.Split(content, "\n")
	if len(gotLines) != len(lines) {
		t.Fatalf("expected %d lines, got %d", len(lines), len(gotLines))
	}

	for i, line := range gotLines {
		orig := strings.TrimRight(lines[i], " ")
		origWidth := visibleWidth(orig)
		wantPadding := (width - origWidth) / 2
		gotPadding := visibleWidth(leadingIndent(line))
		if gotPadding != wantPadding {
			t.Fatalf("line %d padding: want %d, got %d", i, wantPadding, gotPadding)
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
