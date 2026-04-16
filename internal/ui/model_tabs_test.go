package ui

import (
	"testing"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func containsResponseTab(tabs []responseTab, target responseTab) bool {
	for _, tab := range tabs {
		if tab == target {
			return true
		}
	}
	return false
}

func TestAvailableResponseTabsIncludesTimelineForTraceSpec(t *testing.T) {
	model := New(Config{})
	snapshot := &responseSnapshot{
		ready:     true,
		traceSpec: &restfile.TraceSpec{Enabled: true},
	}
	model.responsePanes[responsePanePrimary].snapshot = snapshot

	tabs := model.availableResponseTabs()
	if !containsResponseTab(tabs, responseTabTimeline) {
		t.Fatalf("expected timeline tab when trace spec enabled, got %v", tabs)
	}
}

func TestAvailableResponseTabsSkipsTimelineWhenTraceDisabled(t *testing.T) {
	model := New(Config{})
	snapshot := &responseSnapshot{
		ready:     true,
		traceSpec: &restfile.TraceSpec{Enabled: false},
	}
	model.responsePanes[responsePanePrimary].snapshot = snapshot

	tabs := model.availableResponseTabs()
	if containsResponseTab(tabs, responseTabTimeline) {
		t.Fatalf("expected timeline tab to be omitted when trace spec disabled")
	}
}

func TestAvailableResponseTabsIncludesExplainWhenSnapshotHasReport(t *testing.T) {
	model := New(Config{})
	snapshot := &responseSnapshot{
		ready: true,
		explain: explainState{
			report: &xplain.Report{Status: xplain.StatusReady},
		},
	}
	model.responsePanes[responsePanePrimary].snapshot = snapshot

	tabs := model.availableResponseTabs()
	if !containsResponseTab(tabs, responseTabExplain) {
		t.Fatalf("expected explain tab when explain report exists, got %v", tabs)
	}
}

func TestResponseTabLabelForProfileStats(t *testing.T) {
	label := responseTabLabelForSnapshot(responseTabStats, &responseSnapshot{
		statsKind: statsReportKindProfile,
	})
	if label != "Profile" {
		t.Fatalf("expected profile stats label, got %q", label)
	}
}

func TestResponseTabLabelForWorkflowStats(t *testing.T) {
	label := responseTabLabelForSnapshot(responseTabStats, &responseSnapshot{
		statsKind: statsReportKindWorkflow,
	})
	if label != "Workflow" {
		t.Fatalf("expected workflow stats label, got %q", label)
	}
}

func TestResponseTabLabelFallsBackToStats(t *testing.T) {
	label := responseTabLabelForSnapshot(responseTabStats, nil)
	if label != "Stats" {
		t.Fatalf("expected generic stats label, got %q", label)
	}
}
