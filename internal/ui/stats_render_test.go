package ui

import "testing"

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
	colored := colorizeStatsReport(input, statsReportKindProfile)
	if stripped := ansiSequenceRegex.ReplaceAllString(colored, ""); stripped != input {
		t.Fatalf("expected colorization to preserve text, got %q", stripped)
	}
}

func TestColorizeWorkflowStatsReport(t *testing.T) {
	input := "Workflow: sample\nStarted: 2025-01-01T00:00:00Z\nEnded: 2025-01-01T00:01:00Z\nSteps: 2\n\n1. Seed [PASS] (201 Created) [120ms]\n2. Verify [FAIL] (500)\n    expected 200\n"
	colored := colorizeStatsReport(input, statsReportKindWorkflow)
	if stripped := ansiSequenceRegex.ReplaceAllString(colored, ""); stripped != input {
		t.Fatalf("expected workflow colorization to preserve text, got %q", stripped)
	}
}

func TestColorizeStatsReportNone(t *testing.T) {
	input := "plain text"
	colored := colorizeStatsReport(input, statsReportKindNone)
	if colored != input {
		t.Fatalf("expected no changes for statsReportKindNone")
	}
}
