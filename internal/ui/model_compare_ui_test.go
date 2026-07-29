package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"

	"github.com/unkn0wn-root/resterm/internal/history"
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

func TestSelectCompareFocusPinsGroupedHistoryBaseline(t *testing.T) {
	model := New(Config{})
	model.responseSplit = true
	model.responsePaneFocus = responsePanePrimary
	model.responsePanes[0] = newResponsePaneState(viewport.New(80, 10), true)
	model.responsePanes[1] = newResponsePaneState(viewport.New(80, 10), false)
	model.responsePanes[0].activeTab = responseTabCompare

	entry := history.Entry{Compare: &history.CompareEntry{
		Baseline: "prod",
		Group:    "api",
		Results: []history.CompareResult{
			{
				Environment: "api=dev, auth=ci",
				Profile:     "dev",
				Status:      "200 OK",
				BodySnippet: `{"env":"dev"}`,
			},
			{
				Environment: "api=prod, auth=ci",
				Profile:     "prod",
				Status:      "200 OK",
				BodySnippet: `{"env":"prod"}`,
			},
		},
	}}
	bundle := bundleFromHistory(entry)
	if bundle == nil {
		t.Fatal("expected compare bundle")
	}
	model.populateCompareSnapshotsFromHistory(entry, bundle, "")
	model.compareBundle = bundle
	model.compareFocusedEnv = "api=dev, auth=ci"
	model.compareRowIndex = 0

	cmd := model.selectCompareFocus()
	collectMsgs(cmd)

	primary := model.pane(responsePanePrimary)
	secondary := model.pane(responsePaneSecondary)
	if primary == nil || primary.snapshot == nil {
		t.Fatal("expected primary pane snapshot")
	}
	if secondary == nil || secondary.snapshot == nil {
		t.Fatal("expected secondary pane snapshot")
	}
	if got, want := primary.snapshot.environment, "api=dev, auth=ci"; got != want {
		t.Fatalf("primary environment = %q, want %q", got, want)
	}
	if got, want := secondary.snapshot.environment, "api=prod, auth=ci"; got != want {
		t.Fatalf("secondary environment = %q, want %q", got, want)
	}
	if primary.snapshot == secondary.snapshot {
		t.Fatal("target and baseline snapshots must be distinct")
	}
}

func TestSelectCompareFocusRejectsMissingBaselineSnapshot(t *testing.T) {
	model := New(Config{})
	model.responseSplit = true
	model.responsePaneFocus = responsePanePrimary
	model.responsePanes[0] = newResponsePaneState(viewport.New(80, 10), true)
	model.responsePanes[1] = newResponsePaneState(viewport.New(80, 10), false)
	model.responsePanes[0].activeTab = responseTabCompare

	primarySnap := &responseSnapshot{ready: true, environment: "current"}
	secondarySnap := &responseSnapshot{ready: true, environment: "previous"}
	model.responsePanes[0].snapshot = primarySnap
	model.responsePanes[1].snapshot = secondarySnap
	model.setCompareSnapshot(
		"api=dev, auth=ci",
		&responseSnapshot{ready: true, environment: "api=dev, auth=ci"},
	)
	model.compareBundle = &compareBundle{
		Baseline: "api=prod, auth=ci",
		Rows: []compareRow{
			{Result: &compareResult{Environment: "api=dev, auth=ci"}},
			{Result: &compareResult{Environment: "api=prod, auth=ci"}},
		},
	}
	model.compareFocusedEnv = "api=dev, auth=ci"

	if cmd := model.selectCompareFocus(); cmd != nil {
		t.Fatal("missing baseline snapshot should not schedule a diff")
	}
	if got, want := model.statusMessage.text,
		"Baseline response for api=prod, auth=ci unavailable"; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if model.statusMessage.level != statusWarn {
		t.Fatalf("status level = %v, want warning", model.statusMessage.level)
	}
	if model.responsePanes[0].snapshot != primarySnap {
		t.Fatal("missing baseline must not replace the primary snapshot")
	}
	if model.responsePanes[1].snapshot != secondarySnap {
		t.Fatal("missing baseline must not replace the secondary snapshot")
	}
}

func TestBuildCompareBundleMatchesGroupedBaselineProfile(t *testing.T) {
	results := []compareResult{
		{Environment: "api=dev, auth=ci", Profile: "dev"},
		{Environment: "api=prod, auth=ci", Profile: "prod"},
	}
	bundle := buildCompareBundle(results, "prod")
	if bundle == nil {
		t.Fatal("expected compare bundle")
	}
	if got, want := bundle.Baseline, "api=prod, auth=ci"; got != want {
		t.Fatalf("baseline = %q, want %q", got, want)
	}
	if got := bundle.Rows[1].Summary; got != "baseline" {
		t.Fatalf("baseline row summary = %q, want baseline", got)
	}
}
