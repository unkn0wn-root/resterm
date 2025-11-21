package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
)

func TestColorizeProfileStatsReport(t *testing.T) {
	bins := []analysis.HistogramBucket{
		{From: 10 * time.Millisecond, To: 15 * time.Millisecond, Count: 2},
		{From: 15 * time.Millisecond, To: 20 * time.Millisecond, Count: 1},
	}
	hist := renderHistogram(bins, histogramDefaultIndent)
	input := "" +
		"Profiling Load Test\n" +
		"───────────────────\n\n" +
		"Summary:\n" +
		"  Runs: 5 total | 3 success | 1 failure | 1 warmup\n" +
		"  Success: 75% (3/4)\n" +
		"  Window: 4 runs in 1.2s\n" +
		"  Throughput: 3.3 rps | no-delay: 4.0 rps\n\n" +
		"Latency (3 samples):\n" +
		"  min   p50   p90   p95   p99   max\n" +
		"  10ms  12ms  18ms  18ms  18ms  20ms\n" +
		"  mean: 14ms | median: 12ms | stddev: 4ms\n\n" +
		"Distribution:\n" +
		hist
	colored := colorizeStatsReport(input, statsReportKindProfile, nil)
	stripped := ansiSequenceRegex.ReplaceAllString(colored, "")
	if normalizeSpaces(stripped) != normalizeSpaces(input) {
		t.Fatalf("expected colorization to preserve text, got %q", stripped)
	}
}

func TestColorizeProfileHistogramBarsColored(t *testing.T) {
	bins := []analysis.HistogramBucket{
		{From: 10 * time.Millisecond, To: 15 * time.Millisecond, Count: 2},
		{From: 15 * time.Millisecond, To: 20 * time.Millisecond, Count: 1},
	}
	hist := renderHistogram(bins, histogramDefaultIndent)
	input := "" +
		"Profiling Load Test\n" +
		"───────────────────\n\n" +
		"Distribution:\n" +
		hist
	colored := colorizeStatsReport(input, statsReportKindProfile, nil)
	stripped := ansiSequenceRegex.ReplaceAllString(colored, "")
	lines := strings.Split(strings.TrimSpace(hist), "\n")
	for _, line := range lines {
		if !strings.Contains(stripped, strings.TrimSpace(line)) {
			t.Fatalf("expected histogram line %q to be preserved, got %q", line, stripped)
		}
	}
}

func TestColorizeProfileHistogramUsesStatsPercentiles(t *testing.T) {
	bins := []analysis.HistogramBucket{
		{From: 35 * time.Millisecond, To: 45 * time.Millisecond, Count: 5},
		{From: 45 * time.Millisecond, To: 55 * time.Millisecond, Count: 5},
	}
	rows := strings.Split(strings.TrimSpace(renderHistogram(bins, histogramDefaultIndent)), "\n")
	stats := analysis.LatencyStats{
		Count: 10,
		Percentiles: map[int]time.Duration{
			50: 20 * time.Millisecond,
			90: 40 * time.Millisecond,
		},
		Histogram: bins,
	}
	ctx := buildHistogramContext(rows, &stats)
	foundWarn := false
	for idx, row := range ctx.lines {
		style := histogramBarStyle(idx, row, ctx)
		if bucketTouchesOrExceeds(row, ctx.p90) {
			foundWarn = true
			if style.Render("x") != statsWarnStyle.Render("x") {
				t.Fatalf("expected p90 bucket to render in warn style")
			}
		}
	}
	if !foundWarn {
		t.Fatalf("expected at least one bucket to touch p90 threshold")
	}
}

func TestParseHistogramLineKeepsOriginalBarWidth(t *testing.T) {
	bins := []analysis.HistogramBucket{
		{From: 10 * time.Millisecond, To: 15 * time.Millisecond, Count: 2},
		{From: 15 * time.Millisecond, To: 20 * time.Millisecond, Count: 1},
	}
	rendered := renderHistogram(bins, histogramDefaultIndent)
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected histogram output")
	}
	row, ok := parseHistogramLine(lines[0])
	if !ok {
		t.Fatalf("failed to parse histogram line: %q", lines[0])
	}
	if row.barWidth != histogramBarWidth {
		t.Fatalf("expected bar width 22, got %d (line: %q)", row.barWidth, lines[0])
	}
}

func TestP90BucketNotFadedWhenSmall(t *testing.T) {
	bins := []analysis.HistogramBucket{
		{From: 100 * time.Millisecond, To: 110 * time.Millisecond, Count: 8},
		{From: 138 * time.Millisecond, To: 142 * time.Millisecond, Count: 1},
	}
	rows := strings.Split(strings.TrimSpace(renderHistogram(bins, histogramDefaultIndent)), "\n")
	stats := analysis.LatencyStats{
		Count: 9,
		Percentiles: map[int]time.Duration{
			50: 103 * time.Millisecond,
			90: 140 * time.Millisecond,
		},
		Histogram: bins,
	}
	ctx := buildHistogramContext(rows, &stats)
	foundWarn := false
	for idx, row := range ctx.lines {
		if bucketTouchesOrExceeds(row, ctx.p90) {
			foundWarn = true
			if histogramBarStyle(idx, row, ctx).Render("x") != statsWarnStyle.Render("x") {
				t.Fatalf("expected small p90 bucket to render in warn style")
			}
		}
	}
	if !foundWarn {
		t.Fatalf("expected histogram bucket to touch p90 threshold")
	}
}

func TestColorizeWorkflowStatsReport(t *testing.T) {
	input := "Workflow: sample\nStarted: 2025-01-01T00:00:00Z\nEnded: 2025-01-01T00:01:00Z\nSteps: 2\n\n1. Seed [PASS] (201 Created) [120ms]\n2. Verify [FAIL] (500)\n    expected 200\n"
	colored := colorizeStatsReport(input, statsReportKindWorkflow, nil)
	if stripped := ansiSequenceRegex.ReplaceAllString(colored, ""); stripped != input {
		t.Fatalf("expected workflow colorization to preserve text, got %q", stripped)
	}
}

func TestColorizeStatsReportNone(t *testing.T) {
	input := "plain text"
	colored := colorizeStatsReport(input, statsReportKindNone, nil)
	if colored != input {
		t.Fatalf("expected no changes for statsReportKindNone")
	}
}

func normalizeSpaces(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
