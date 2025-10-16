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

func TestAdjustSidebarWidthModifiesWidths(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 180
	model.height = 60
	model.ready = true
	_ = model.applyLayout()

	initialSidebar := model.sidebarWidthPx
	initialEditor := model.editor.Width()
	if initialSidebar <= 0 || initialEditor <= 0 {
		t.Fatalf("expected initial widths to be positive, got sidebar %d editor %d", initialSidebar, initialEditor)
	}

	if changed, _, _ := model.adjustSidebarWidth(sidebarWidthStep); !changed {
		t.Fatalf("expected sidebar width increase to apply")
	}
	expanded := model.sidebarWidthPx
	if expanded <= initialSidebar {
		t.Fatalf("expected sidebar width to grow, initial %d new %d", initialSidebar, expanded)
	}
	if model.editor.Width() >= initialEditor {
		t.Fatalf("expected editor width to shrink after sidebar grows, initial %d new %d", initialEditor, model.editor.Width())
	}

	if changed, _, _ := model.adjustSidebarWidth(-sidebarWidthStep * 2); !changed {
		t.Fatalf("expected sidebar width decrease to apply")
	}
	shrunken := model.sidebarWidthPx
	if shrunken >= initialSidebar {
		t.Fatalf("expected sidebar width to shrink below initial, initial %d new %d", initialSidebar, shrunken)
	}
	if model.editor.Width() <= initialEditor {
		t.Fatalf("expected editor width to grow after sidebar shrinks, initial %d new %d", initialEditor, model.editor.Width())
	}
}

func TestAdjustSidebarWidthClampsBounds(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 160
	model.height = 60
	model.ready = true

	model.sidebarWidth = maxSidebarWidthRatio
	_ = model.applyLayout()
	if changed, bounded, _ := model.adjustSidebarWidth(sidebarWidthStep); changed {
		t.Fatalf("expected width at maximum to remain unchanged")
	} else if !bounded {
		t.Fatalf("expected upper bound to be reported when at maximum width")
	}

	model.sidebarWidth = minSidebarWidthRatio
	_ = model.applyLayout()
	if changed, bounded, _ := model.adjustSidebarWidth(-sidebarWidthStep); changed {
		t.Fatalf("expected width at minimum to remain unchanged")
	} else if !bounded {
		t.Fatalf("expected lower bound to be reported when at minimum width")
	}

	model.sidebarWidth = sidebarWidthDefault
	_ = model.applyLayout()
	if changed, _, _ := model.adjustSidebarWidth(sidebarWidthStep); !changed {
		t.Fatalf("expected width adjustment within bounds to apply")
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
	initialResponse := model.responseContentWidth()
	if initialEditor <= 0 || initialResponse <= 0 {
		t.Fatalf("expected initial widths to be positive, got %d and %d", initialEditor, initialResponse)
	}

	if changed, _, _ := model.adjustEditorSplit(-editorSplitStep); !changed {
		t.Fatalf("expected editor split decrease to apply")
	}
	if model.editor.Width() >= initialEditor {
		t.Fatalf("expected editor width to shrink, initial %d new %d", initialEditor, model.editor.Width())
	}
	if model.responseContentWidth() <= initialResponse {
		t.Fatalf("expected response width to grow, initial %d new %d", initialResponse, model.responseContentWidth())
	}

	if changed, _, _ := model.adjustEditorSplit(editorSplitStep * 2); !changed {
		t.Fatalf("expected editor split increase to apply")
	}
	if model.editor.Width() <= initialEditor {
		t.Fatalf("expected editor width to exceed original, initial %d new %d", initialEditor, model.editor.Width())
	}
	if model.responseContentWidth() >= initialResponse {
		t.Fatalf("expected response width to shrink, initial %d new %d", initialResponse, model.responseContentWidth())
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

func TestApplyLayoutAccountsForWorkflowPadding(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 120
	model.height = 60
	model.ready = true
	model.workflowItems = []workflowListItem{{}}
	model.showWorkflow = true
	model.sidebarSplit = sidebarWorkflowSplit
	_ = model.applyLayout()

	gaps := sidebarSplitPadding + 1
	if total := model.sidebarFilesHeight + model.sidebarRequestsHeight + gaps; total != model.paneContentHeight {
		t.Fatalf("expected pane content height %d, got %d", model.paneContentHeight, total)
	}
	requestHeight := model.requestList.Height() + sidebarChrome
	workflowHeight := model.workflowList.Height() + sidebarChrome
	if requestHeight <= 0 || workflowHeight <= 0 {
		t.Fatalf("expected positive section heights, got %d and %d", requestHeight, workflowHeight)
	}
	if got := requestHeight + workflowHeight; got != model.sidebarRequestsHeight {
		t.Fatalf("expected combined request heights %d, got %d", model.sidebarRequestsHeight, got)
	}
	if diff := requestHeight - workflowHeight; diff > 1 || diff < -1 {
		t.Fatalf("expected request/workflow sections to remain balanced, requests=%d workflows=%d", requestHeight, workflowHeight)
	}
}

func TestApplyLayoutMaintainsHeightWithWorkflowFocus(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 120
	model.height = 60
	model.ready = true
	model.focus = focusWorkflows
	model.workflowItems = []workflowListItem{{}}
	model.showWorkflow = true
	model.sidebarSplit = sidebarWorkflowSplit
	_ = model.applyLayout()

	gaps := sidebarSplitPadding + 1
	total := model.sidebarFilesHeight + model.sidebarRequestsHeight + gaps
	if total != model.paneContentHeight {
		t.Fatalf("expected pane height %d, got %d", model.paneContentHeight, total)
	}

	expectedRequests := (model.requestList.Height() + sidebarChrome) +
		(model.workflowList.Height() + sidebarChrome) + sidebarFocusPad
	if model.sidebarRequestsHeight != expectedRequests {
		t.Fatalf("expected requests height %d, got %d", expectedRequests, model.sidebarRequestsHeight)
	}
}
