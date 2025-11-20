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
	output := renderHistogram(bins, "  ")
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 histogram rows, got %d", len(lines))
	}
	// Compare positions of separators
	first := lines[0]
	second := lines[1]
	pipeIdx := strings.Index(first, "|")
	if pipeIdx == -1 || strings.Index(second, "|") != pipeIdx {
		t.Fatalf("expected pipes to align: %q vs %q", first, second)
	}
	parenIdx := strings.Index(first, "(")
	if parenIdx == -1 || strings.Index(second, "(") != parenIdx {
		t.Fatalf("expected counts to align: %q vs %q", first, second)
	}
}
