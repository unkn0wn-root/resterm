package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
)

func TestSelectCompareFocusPinsSnapshots(t *testing.T) {
	model := New(Config{})
	model.responseSplit = true
	model.responsePaneFocus = responsePanePrimary
	model.responsePanes[0] = newResponsePaneState(viewport.New(80, 10), true)
	model.responsePanes[1] = newResponsePaneState(viewport.New(80, 10), false)
	model.responsePanes[0].activeTab = responseTabCompare

	devSnap := &responseSnapshot{ready: true, environment: "dev", pretty: "dev"}
	stageSnap := &responseSnapshot{ready: true, environment: "stage", pretty: "stage"}
	model.setCompareSnapshot("dev", devSnap)
	model.setCompareSnapshot("stage", stageSnap)

	model.compareBundle = &compareBundle{
		Baseline: "dev",
		Rows: []compareRow{
			{Result: &compareResult{Environment: "dev"}},
			{Result: &compareResult{Environment: "stage"}},
		},
	}
	model.compareFocusedEnv = "stage"
	model.compareRowIndex = 1

	cmd := model.selectCompareFocus()
	collectMsgs(cmd)

	if model.compareSelectedEnv != "stage" {
		t.Fatalf("expected selected env to be stage, got %q", model.compareSelectedEnv)
	}
	primary := model.pane(responsePanePrimary)
	secondary := model.pane(responsePaneSecondary)
	if primary == nil || primary.snapshot != stageSnap {
		t.Fatalf("expected primary pane to show stage snapshot")
	}
	if secondary == nil || secondary.snapshot != devSnap {
		t.Fatalf("expected secondary pane to show dev snapshot")
	}
	if primary.activeTab != responseTabDiff {
		t.Fatalf("expected primary pane to switch to diff tab")
	}
	if model.compareRowIndex != 1 {
		t.Fatalf(
			"expected compareRowIndex to remain at selected row, got %d",
			model.compareRowIndex,
		)
	}
}
