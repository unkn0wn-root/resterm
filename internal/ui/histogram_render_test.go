package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
)

func TestRenderHistogramAlignment(t *testing.T) {
	bins := []analysis.HistogramBucket{
		{From: 1 * time.Millisecond, To: 2 * time.Millisecond, Count: 6},
		{From: 3 * time.Millisecond, To: 5 * time.Millisecond, Count: 0},
	}
	output := renderHistogram(bins)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 { // header + 2 rows
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	// Compare positions of separators
	first := lines[1]
	second := lines[2]
	pipeIdx := strings.Index(first, "|")
	if pipeIdx == -1 || strings.Index(second, "|") != pipeIdx {
		t.Fatalf("expected pipes to align: %q vs %q", first, second)
	}
	parenIdx := strings.Index(first, "(")
	if parenIdx == -1 || strings.Index(second, "(") != parenIdx {
		t.Fatalf("expected counts to align: %q vs %q", first, second)
	}
}
