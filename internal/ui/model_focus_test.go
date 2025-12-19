package ui

import "testing"

func TestCycleFocusSkipsCollapsedPane(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 140
	model.height = 50
	model.ready = true
	_ = model.applyLayout()
	_ = model.setFocus(focusEditor)

	if res := model.setCollapseState(paneRegionResponse, true); res.blocked {
		t.Fatalf("expected response collapse to be allowed")
	}
	_ = model.applyLayout()

	_ = model.cycleFocus(true)
	if model.focus != focusRequests {
		t.Fatalf("expected focus to skip collapsed response pane, got %v", model.focus)
	}
}
