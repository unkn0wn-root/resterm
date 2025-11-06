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
	if res := model.setCollapseState(paneRegionEditor, true); res.blocked {
		t.Fatalf("expected editor collapse to be allowed")
	}
	if res := model.setCollapseState(paneRegionResponse, true); res.blocked {
		t.Fatalf("expected response collapse to be allowed")
	}
	if cmd := model.togglePaneCollapse(paneRegionSidebar); cmd != nil {
		t.Fatalf("expected no layout command when blocking final collapse")
	}
	if model.sidebarCollapsed {
		t.Fatalf("expected sidebar to remain visible when it is the last pane")
	}
	if !strings.Contains(strings.ToLower(model.statusMessage.text), "cannot hide") {
		t.Fatalf("expected warning when attempting to minimize requests pane, got %q", model.statusMessage.text)
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
