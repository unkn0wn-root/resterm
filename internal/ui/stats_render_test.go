package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
)

func TestColorizeProfileStatsReport(t *testing.T) {
	input := "" +
		"Profiling Load Test\n" +
		"───────────────────\n\n" +
		"Summary:\n" +
		"  Runs: 5 total | 3 success | 1 failure | 1 warmup\n" +
		"  Success: 75% (3/4)\n" +
		"  Elapsed: 1.2s | Throughput: 3.3 rps\n\n" +
		"Latency (3 samples):\n" +
		"  min   p50   p90   p95   p99   max\n" +
		"  10ms  12ms  18ms  18ms  18ms  20ms\n" +
		"  mean: 14ms | median: 12ms | stddev: 4ms\n\n" +
		"Distribution:\n" +
		"  10ms – 15ms | #### (2) 67%\n" +
		"  15ms – 20ms | ## (1) 33%\n"
	colored := colorizeStatsReport(input, statsReportKindProfile, nil)
	stripped := ansiSequenceRegex.ReplaceAllString(colored, "")
	if normalizeSpaces(stripped) != normalizeSpaces(input) {
		t.Fatalf("expected colorization to preserve text, got %q", stripped)
	}
}

func TestColorizeProfileHistogramBarsColored(t *testing.T) {
	input := "" +
		"Profiling Load Test\n" +
		"───────────────────\n\n" +
		"Distribution:\n" +
		"  10ms – 15ms | #### (2) 67%\n" +
		"  15ms – 20ms | ## (1) 33%\n"
	colored := colorizeStatsReport(input, statsReportKindProfile, nil)
	stripped := ansiSequenceRegex.ReplaceAllString(colored, "")
	if !strings.Contains(stripped, "10ms – 15ms | ####") || !strings.Contains(stripped, "15ms – 20ms | ##") {
		t.Fatalf("expected histogram lines to be preserved, got %q", stripped)
	}
}

func TestColorizeProfileHistogramUsesStatsPercentiles(t *testing.T) {
	bins := []analysis.HistogramBucket{
		{From: 35 * time.Millisecond, To: 45 * time.Millisecond, Count: 5},
		{From: 45 * time.Millisecond, To: 55 * time.Millisecond, Count: 5},
	}
	report := "" +
		"Profiling Load Test\n" +
		"───────────────────\n\n" +
		"Distribution:\n" +
		renderHistogram(bins, "  ")
	stats := analysis.LatencyStats{
		Count: 10,
		Percentiles: map[int]time.Duration{
			50: 20 * time.Millisecond,
			90: 40 * time.Millisecond,
		},
		Histogram: bins,
	}
	colored := colorizeStatsReport(report, statsReportKindProfile, &stats)
	warnBar := statsWarnStyle.Render(strings.Repeat("#", 22))
	if !strings.Contains(colored, warnBar) {
		t.Fatalf("expected bucket crossing p90 to render in warn style; output: %q", colored)
	}
}

func TestParseHistogramLineKeepsOriginalBarWidth(t *testing.T) {
	bins := []analysis.HistogramBucket{
		{From: 10 * time.Millisecond, To: 15 * time.Millisecond, Count: 2},
		{From: 15 * time.Millisecond, To: 20 * time.Millisecond, Count: 1},
	}
	rendered := renderHistogram(bins, "  ")
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected histogram output")
	}
	row, ok := parseHistogramLine(lines[0])
	if !ok {
		t.Fatalf("failed to parse histogram line: %q", lines[0])
	}
	if row.barWidth != 22 {
		t.Fatalf("expected bar width 22, got %d (line: %q)", row.barWidth, lines[0])
	}
}

func TestP90BucketNotFadedWhenSmall(t *testing.T) {
	bins := []analysis.HistogramBucket{
		{From: 100 * time.Millisecond, To: 110 * time.Millisecond, Count: 8},
		{From: 138 * time.Millisecond, To: 142 * time.Millisecond, Count: 1},
	}
	report := "" +
		"Profiling Load Test\n" +
		"───────────────────\n\n" +
		"Distribution:\n" +
		renderHistogram(bins, "  ")
	stats := analysis.LatencyStats{
		Count: 9,
		Percentiles: map[int]time.Duration{
			50: 103 * time.Millisecond,
			90: 140 * time.Millisecond,
		},
		Histogram: bins,
	}
	colored := colorizeStatsReport(report, statsReportKindProfile, &stats)
	warnBar := statsWarnStyle.Render("##")
	if !strings.Contains(colored, warnBar) {
		t.Fatalf("expected small p90 bucket to render in warn style, got %q", colored)
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
