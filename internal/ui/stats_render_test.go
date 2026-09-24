package ui

import "testing"

func TestColorizeWorkflowStatsReport(t *testing.T) {
	input := "Workflow: sample\nStarted: 2025-01-01T00:00:00Z\nEnded: 2025-01-01T00:01:00Z\nSteps: 3\n\n1. Seed [PASS] (201 Created) [120ms]\n2. Verify [FAIL] (500)\n    expected 200\n3. Cleanup [CANCELED]\n"
	colored := colorizeWorkflowStats(input)
	if stripped := ansiSequenceRegex.ReplaceAllString(colored, ""); stripped != input {
		t.Fatalf("expected workflow colorization to preserve text, got %q", stripped)
	}
}
