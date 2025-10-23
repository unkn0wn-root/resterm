package ui

import "testing"

func TestColorizeProfileStatsReport(t *testing.T) {
	input := "Load Test\nMeasured runs: 3\nWarmup runs: 1\nFailures: 0\nLatency Summary:\n  Min: 10ms\n"
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
