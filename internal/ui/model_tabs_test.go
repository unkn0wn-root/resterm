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
