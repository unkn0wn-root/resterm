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

	initialFiles := model.sidebarFilesHeight
	initialRequests := model.sidebarRequestsHeight
	if initialFiles <= 0 || initialRequests <= 0 {
		t.Fatalf("expected initial sidebar heights to be positive, got %d and %d", initialFiles, initialRequests)
	}

	if changed, _, _ := model.adjustSidebarSplit(sidebarSplitStep); !changed {
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

	model.sidebarSplit = maxSidebarSplit
	if changed, bounded, _ := model.adjustSidebarSplit(sidebarSplitStep); changed {
		t.Fatalf("expected split at max to remain unchanged")
	} else if !bounded {
		t.Fatalf("expected upper bound to be reported when at maximum")
	}

	model.sidebarSplit = minSidebarSplit
	if changed, bounded, _ := model.adjustSidebarSplit(-sidebarSplitStep); changed {
		t.Fatalf("expected split at min to remain unchanged")
	} else if !bounded {
		t.Fatalf("expected lower bound to be reported when at minimum")
	}

	model.sidebarSplit = sidebarSplitDefault
	model.focus = focusEditor
	if changed, _, _ := model.adjustSidebarSplit(sidebarSplitStep); !changed {
		t.Fatalf("expected adjustment to apply regardless of focus")
	}
}

func TestAdjustEditorSplitReallocatesWidths(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 160
	model.height = 60
	model.ready = true
	_ = model.applyLayout()

	initialEditor := model.editor.Width()
	initialResponse := model.responseViewport.Width
	if initialEditor <= 0 || initialResponse <= 0 {
		t.Fatalf("expected initial widths to be positive, got %d and %d", initialEditor, initialResponse)
	}

	if changed, _, _ := model.adjustEditorSplit(-editorSplitStep); !changed {
		t.Fatalf("expected editor split decrease to apply")
	}
	if model.editor.Width() >= initialEditor {
		t.Fatalf("expected editor width to shrink, initial %d new %d", initialEditor, model.editor.Width())
	}
	if model.responseViewport.Width <= initialResponse {
		t.Fatalf("expected response width to grow, initial %d new %d", initialResponse, model.responseViewport.Width)
	}

	if changed, _, _ := model.adjustEditorSplit(editorSplitStep * 2); !changed {
		t.Fatalf("expected editor split increase to apply")
	}
	if model.editor.Width() <= initialEditor {
		t.Fatalf("expected editor width to exceed original, initial %d new %d", initialEditor, model.editor.Width())
	}
	if model.responseViewport.Width >= initialResponse {
		t.Fatalf("expected response width to shrink, initial %d new %d", initialResponse, model.responseViewport.Width)
	}
}

func TestAdjustEditorSplitClampsBounds(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 160
	model.height = 60
	model.ready = true
	_ = model.applyLayout()

	model.editorSplit = minEditorSplit
	_ = model.applyLayout()
	if changed, bounded, _ := model.adjustEditorSplit(-editorSplitStep); changed {
		t.Fatalf("expected split at minimum to remain unchanged")
	} else if !bounded {
		t.Fatalf("expected lower bound to be reported when at minimum width")
	}

	model.editorSplit = maxEditorSplit
	_ = model.applyLayout()
	if changed, bounded, _ := model.adjustEditorSplit(editorSplitStep); changed {
		t.Fatalf("expected split at maximum to remain unchanged")
	} else if !bounded {
		t.Fatalf("expected upper bound to be reported when at maximum width")
	}

	model.editorSplit = editorSplitDefault
	_ = model.applyLayout()
	if changed, _, _ := model.adjustEditorSplit(editorSplitStep); !changed {
		t.Fatalf("expected adjustment to apply when within bounds")
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
