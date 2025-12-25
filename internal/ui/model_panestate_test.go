package ui

import (
	"strings"
	"testing"
)

func TestTogglePaneCollapseBlocksLastPane(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 140
	model.height = 50
	model.ready = true
	_ = model.applyLayout()
	// Collapse editor allowed.
	if res := model.setCollapseState(paneRegionEditor, true); res.blocked {
		t.Fatalf("expected editor collapse to be allowed")
	}
	// Collapse sidebar allowed while response visible.
	if res := model.setCollapseState(paneRegionSidebar, true); res.blocked {
		t.Fatalf("expected sidebar collapse to be allowed")
	}
	// Attempt to collapse response (last visible) should be blocked.
	if cmd := model.togglePaneCollapse(paneRegionResponse); cmd != nil {
		t.Fatalf("expected no layout command when blocking final collapse")
	}
	if model.responseCollapsed {
		t.Fatalf("expected response to remain visible when it is the last pane")
	}
	if !strings.Contains(strings.ToLower(model.statusMessage.text), "need at least one pane") {
		t.Fatalf("expected warning about keeping a pane visible, got %q", model.statusMessage.text)
	}
}

func TestToggleZoomForRegionTogglesState(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 140
	model.height = 50
	model.ready = true
	_ = model.applyLayout()
	_ = model.toggleZoomForRegion(paneRegionEditor)
	if !model.zoomActive || model.zoomRegion != paneRegionEditor {
		t.Fatalf("expected zoom to activate for editor")
	}
	_ = model.toggleZoomForRegion(paneRegionEditor)
	if model.zoomActive {
		t.Fatalf("expected second zoom toggle to clear state")
	}
}

func TestTogglePaneCollapseSidebarRestores(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 140
	model.height = 50
	model.ready = true
	model.sidebarCollapsed = true
	_ = model.applyLayout()

	_ = model.togglePaneCollapse(paneRegionSidebar)
	if model.sidebarCollapsed {
		t.Fatalf("expected sidebar to restore from collapsed state")
	}
	if !strings.Contains(strings.ToLower(model.statusMessage.text), "sidebar restored") {
		t.Fatalf("expected sidebar restored status, got %q", model.statusMessage.text)
	}
}

func TestToggleZoomForSidebarDisabled(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 140
	model.height = 50
	model.ready = true
	_ = model.applyLayout()

	if cmd := model.toggleZoomForRegion(paneRegionSidebar); cmd != nil {
		t.Fatalf("expected no command when zooming requests pane")
	}
	if model.zoomActive {
		t.Fatalf("expected zoom to remain inactive when targeting requests pane")
	}
	if !strings.Contains(strings.ToLower(model.statusMessage.text), "cannot be zoomed") {
		t.Fatalf("expected warning about zoom restriction, got %q", model.statusMessage.text)
	}
}

func TestTogglePaneCollapseSidebarMinimizes(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 140
	model.height = 50
	model.ready = true
	_ = model.applyLayout()

	if model.sidebarCollapsed {
		t.Fatalf("expected sidebar to start expanded")
	}
	_ = model.togglePaneCollapse(paneRegionSidebar)
	if !model.sidebarCollapsed {
		t.Fatalf("expected sidebar to collapse after toggle")
	}
	if !strings.Contains(strings.ToLower(model.statusMessage.text), "sidebar minimized") {
		t.Fatalf("expected sidebar minimized status, got %q", model.statusMessage.text)
	}
}

func TestTogglePaneCollapseMovesFocusFromHiddenPane(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 140
	model.height = 50
	model.ready = true
	_ = model.applyLayout()
	_ = model.setFocus(focusResponse)

	_ = model.togglePaneCollapse(paneRegionResponse)
	if model.focus == focusResponse {
		t.Fatalf("expected focus to move away from collapsed response pane")
	}
	if model.focus != focusRequests {
		t.Fatalf("expected focus to land on navigator, got %v", model.focus)
	}
}

func TestTogglePaneCollapsePreventsDoubleMainCollapse(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 140
	model.height = 50
	model.ready = true
	_ = model.applyLayout()

	if res := model.setCollapseState(paneRegionResponse, true); res.blocked {
		t.Fatalf("expected first collapse to be allowed")
	}
	if cmd := model.togglePaneCollapse(paneRegionEditor); cmd != nil {
		t.Fatalf("expected no layout command when blocking second main collapse")
	}
	if model.editorCollapsed {
		t.Fatalf("expected editor to remain visible when response already minimized")
	}
	if !strings.Contains(
		strings.ToLower(model.statusMessage.text),
		"keep editor or response visible",
	) {
		t.Fatalf(
			"expected friendly warning about double collapse, got %q",
			model.statusMessage.text,
		)
	}
}
