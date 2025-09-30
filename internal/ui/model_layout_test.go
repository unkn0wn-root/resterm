package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestAdjustSidebarSplitModifiesHeights(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 120
	model.height = 60
	model.ready = true
	_ = model.applyLayout()

	model.focus = focusFile
	initialFiles := model.sidebarFilesHeight
	initialRequests := model.sidebarRequestsHeight
	if initialFiles <= 0 || initialRequests <= 0 {
		t.Fatalf("expected initial sidebar heights to be positive, got %d and %d", initialFiles, initialRequests)
	}

	if changed, _ := model.adjustSidebarSplit(sidebarSplitStep); !changed {
		t.Fatalf("expected sidebar split adjustment to apply")
	}
	if model.sidebarFilesHeight <= initialFiles {
		t.Fatalf("expected file pane height to grow, initial %d new %d", initialFiles, model.sidebarFilesHeight)
	}
	if model.sidebarRequestsHeight >= initialRequests {
		t.Fatalf("expected request pane height to shrink, initial %d new %d", initialRequests, model.sidebarRequestsHeight)
	}
}

func TestAdjustSidebarSplitClampsBounds(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 120
	model.height = 60
	model.ready = true
	_ = model.applyLayout()

	model.focus = focusFile
	model.sidebarSplit = maxSidebarSplit
	if changed, _ := model.adjustSidebarSplit(sidebarSplitStep); changed {
		t.Fatalf("expected split at max to remain unchanged")
	}

	model.focus = focusRequests
	model.sidebarSplit = minSidebarSplit
	if changed, _ := model.adjustSidebarSplit(-sidebarSplitStep); changed {
		t.Fatalf("expected split at min to remain unchanged")
	}

	model.focus = focusEditor
	if changed, _ := model.adjustSidebarSplit(sidebarSplitStep); changed {
		t.Fatalf("expected adjustment to be ignored outside sidebar focus")
	}
}

func TestViewRespectsFrameDimensions(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.frameWidth = 120
	model.frameHeight = 40
	model.width = model.frameWidth - 2
	model.height = model.frameHeight - 2
	model.ready = true
	_ = model.applyLayout()

	view := model.View()
	if got := lipgloss.Height(view); got != model.frameHeight {
		t.Fatalf("expected view height %d, got %d", model.frameHeight, got)
	}
}
