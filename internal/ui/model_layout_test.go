package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

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
		t.Fatalf(
			"expected initial widths to be positive, got sidebar %d editor %d",
			initialSidebar,
			initialEditor,
		)
	}

	if changed, _, _ := model.adjustSidebarWidth(sidebarWidthStep); !changed {
		t.Fatalf("expected sidebar width increase to apply")
	}
	expanded := model.sidebarWidthPx
	if expanded <= initialSidebar {
		t.Fatalf("expected sidebar width to grow, initial %d new %d", initialSidebar, expanded)
	}
	if model.editor.Width() >= initialEditor {
		t.Fatalf(
			"expected editor width to shrink after sidebar grows, initial %d new %d",
			initialEditor,
			model.editor.Width(),
		)
	}

	if changed, _, _ := model.adjustSidebarWidth(-sidebarWidthStep * 2); !changed {
		t.Fatalf("expected sidebar width decrease to apply")
	}
	shrunken := model.sidebarWidthPx
	if shrunken >= initialSidebar {
		t.Fatalf(
			"expected sidebar width to shrink below initial, initial %d new %d",
			initialSidebar,
			shrunken,
		)
	}
	if model.editor.Width() <= initialEditor {
		t.Fatalf(
			"expected editor width to grow after sidebar shrinks, initial %d new %d",
			initialEditor,
			model.editor.Width(),
		)
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
		t.Fatalf(
			"expected initial widths to be positive, got %d and %d",
			initialEditor,
			initialResponse,
		)
	}

	if changed, _, _ := model.adjustEditorSplit(-editorSplitStep); !changed {
		t.Fatalf("expected editor split decrease to apply")
	}
	if model.editor.Width() >= initialEditor {
		t.Fatalf(
			"expected editor width to shrink, initial %d new %d",
			initialEditor,
			model.editor.Width(),
		)
	}
	if model.responseContentWidth() <= initialResponse {
		t.Fatalf(
			"expected response width to grow, initial %d new %d",
			initialResponse,
			model.responseContentWidth(),
		)
	}

	if changed, _, _ := model.adjustEditorSplit(editorSplitStep * 2); !changed {
		t.Fatalf("expected editor split increase to apply")
	}
	if model.editor.Width() <= initialEditor {
		t.Fatalf(
			"expected editor width to exceed original, initial %d new %d",
			initialEditor,
			model.editor.Width(),
		)
	}
	if model.responseContentWidth() >= initialResponse {
		t.Fatalf(
			"expected response width to shrink, initial %d new %d",
			initialResponse,
			model.responseContentWidth(),
		)
	}
}

func TestResponsePaneWidthMatchesLayout(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	m := New(cfg)
	m.width = 180
	m.height = 60
	m.ready = true
	_ = m.applyLayout()

	filePane := m.renderFilePane()
	editorPane := m.renderEditorPane()
	fileWidth := lipgloss.Width(filePane)
	editorWidth := lipgloss.Width(editorPane)
	target := m.responseTargetWidth(fileWidth, editorWidth)
	var respPane string
	if target > 0 {
		respPane = m.renderResponsePane(target)
		respWidth := lipgloss.Width(respPane)
		excess := fileWidth + editorWidth + respWidth - m.width
		if excess > 0 {
			adjusted := target - excess
			if adjusted > 0 {
				respPane = m.renderResponsePane(adjusted)
				respWidth = lipgloss.Width(respPane)
				if fileWidth+editorWidth+respWidth > m.width {
					t.Fatalf(
						"expected overflow to be resolved, width %d target %d adjusted %d",
						respWidth,
						target,
						adjusted,
					)
				}
			} else {
				respPane = ""
			}
		}
	} else {
		respPane = ""
	}
	total := fileWidth + editorWidth + lipgloss.Width(respPane)
	if total != m.width {
		t.Fatalf("expected combined pane width %d, got %d", m.width, total)
	}
}

func TestZoomEditorKeepsResponseWithinBounds(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	m := New(cfg)
	m.width = 160
	m.height = 50
	m.ready = true
	if !m.setZoomRegion(paneRegionEditor) {
		t.Fatalf("expected zoom to activate")
	}
	_ = m.applyLayout()

	filePane := m.renderFilePane()
	editorPane := m.renderEditorPane()
	fileWidth := lipgloss.Width(filePane)
	editorWidth := lipgloss.Width(editorPane)
	target := m.responseTargetWidth(fileWidth, editorWidth)
	var respPane string
	if target > 0 {
		respPane = m.renderResponsePane(target)
		respWidth := lipgloss.Width(respPane)
		excess := fileWidth + editorWidth + respWidth - m.width
		if excess > 0 {
			adjusted := target - excess
			if adjusted > 0 {
				respPane = m.renderResponsePane(adjusted)
				respWidth = lipgloss.Width(respPane)
				if fileWidth+editorWidth+respWidth > m.width {
					t.Fatalf(
						"expected overflow to clear when zoomed, width %d target %d adjusted %d",
						respWidth,
						target,
						adjusted,
					)
				}
			} else {
				respPane = ""
			}
		}
	} else {
		respPane = ""
	}
	total := fileWidth + editorWidth + lipgloss.Width(respPane)
	if total > m.width {
		t.Fatalf("expected total width <= %d, got %d", m.width, total)
	}
}

func TestCollapseResponseExpandsEditorWidth(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 180
	model.height = 60
	model.ready = true
	_ = model.applyLayout()
	initialEditor := model.editor.Width()
	initialResponse := model.responseContentWidth()
	if res := model.setCollapseState(paneRegionResponse, true); res.blocked {
		t.Fatalf("expected response collapse to be allowed")
	}
	_ = model.applyLayout()
	if !model.effectiveRegionCollapsed(paneRegionResponse) {
		t.Fatalf("expected response pane to report collapsed state")
	}
	if model.editor.Width() <= initialEditor {
		t.Fatalf(
			"expected editor width to grow after collapsing response, initial %d new %d",
			initialEditor,
			model.editor.Width(),
		)
	}
	if model.responseContentWidth() >= initialResponse {
		t.Fatalf(
			"expected response width to shrink after collapse, initial %d new %d",
			initialResponse,
			model.responseContentWidth(),
		)
	}
}

func TestCollapsedResponseHidesPaneWidth(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 180
	model.height = 60
	model.ready = true
	_ = model.applyLayout()

	if res := model.setCollapseState(paneRegionResponse, true); res.blocked {
		t.Fatalf("expected response collapse to be allowed")
	}
	_ = model.applyLayout()
	filePane := model.renderFilePane()
	editorPane := model.renderEditorPane()
	fileWidth := lipgloss.Width(filePane)
	editorWidth := lipgloss.Width(editorPane)
	if tw := model.responseTargetWidth(fileWidth, editorWidth); tw != 0 {
		t.Fatalf("expected collapsed response target width 0, got %d", tw)
	}
	total := fileWidth + editorWidth
	if total != model.width {
		t.Fatalf(
			"expected visible panes to fill width %d after collapse, got %d",
			model.width,
			total,
		)
	}
}

func TestHorizontalCollapseUsesFullHeight(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 160
	model.height = 50
	model.ready = true
	model.mainSplitOrientation = mainSplitHorizontal
	_ = model.applyLayout()
	initialResp := model.responseContentHeight

	if res := model.setCollapseState(paneRegionEditor, true); res.blocked {
		t.Fatalf("expected editor collapse to be allowed")
	}
	_ = model.applyLayout()
	if model.responseContentHeight <= initialResp {
		t.Fatalf(
			"expected response height to grow after editor collapse, initial %d new %d",
			initialResp,
			model.responseContentHeight,
		)
	}
	if model.editorContentHeight != 0 {
		t.Fatalf(
			"expected editor height to be zero when collapsed horizontally, got %d",
			model.editorContentHeight,
		)
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

func TestZoomEditorHidesOtherPanes(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	model.width = 140
	model.height = 50
	model.ready = true
	_ = model.applyLayout()
	if !model.setZoomRegion(paneRegionEditor) {
		t.Fatalf("expected zoom activation to apply")
	}
	_ = model.applyLayout()
	if model.effectiveRegionCollapsed(paneRegionEditor) {
		t.Fatalf("expected zoom target to remain visible")
	}
	if !model.effectiveRegionCollapsed(paneRegionResponse) {
		t.Fatalf("expected response pane to be hidden while zoomed")
	}
	if !model.effectiveRegionCollapsed(paneRegionSidebar) {
		t.Fatalf("expected sidebar to be hidden while zoomed")
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

func TestApplyLayoutKeepsPaneWidthsWithinWindow(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	testCases := []struct {
		name  string
		width int
	}{
		{"wide", 160},
		{"medium", 120},
		{"narrow", 80},
	}

	for _, tc := range testCases {
		t := t
		t.Run(tc.name, func(t *testing.T) {
			model := New(cfg)
			model.ready = true
			model.width = tc.width
			model.height = 40
			_ = model.applyLayout()

			listWidth := lipgloss.Width(model.fileList.View())
			listConfiguredWidth := model.fileList.Width()
			editorViewWidth := lipgloss.Width(model.editor.View())
			responseView := model.renderResponseColumn(responsePanePrimary, false, 0)
			responseViewWidth := lipgloss.Width(responseView)

			editorFrame := model.theme.EditorBorder.GetHorizontalFrameSize()
			responseFrame := model.theme.ResponseBorder.GetHorizontalFrameSize()
			editorContent := editorViewWidth
			responseContent := model.responseContentWidth()
			editorOuter := paneOuterWidthFromContent(editorContent, editorFrame)
			responseOuter := paneOuterWidthFromContent(responseContent, responseFrame)
			total := model.sidebarWidthPx + editorOuter + responseOuter
			if total > model.width {
				t.Fatalf(
					"calculated pane widths sidebar=%d editor=%d (content %d, view %d) response=%d (content %d, view %d) total=%d window %d (list config %d)",
					model.sidebarWidthPx,
					editorOuter,
					editorContent,
					editorViewWidth,
					responseOuter,
					responseContent,
					responseViewWidth,
					total,
					model.width,
					listConfiguredWidth,
				)
			}

			filePane := model.renderFilePane()
			editorPane := model.renderEditorPane()
			available := model.width - lipgloss.Width(filePane) - lipgloss.Width(editorPane)
			if available < 0 {
				available = 0
			}
			responsePane := model.renderResponsePane(available)
			panesWidth := lipgloss.Width(
				lipgloss.JoinHorizontal(lipgloss.Top, filePane, editorPane, responsePane),
			)
			if panesWidth != model.width {
				t.Fatalf(
					"rendered widths file=%d editor=%d response=%d total=%d window %d (calculated sidebar=%d editor=%d response=%d list view %d list config %d editor view %d response view %d)",
					lipgloss.Width(filePane),
					lipgloss.Width(editorPane),
					lipgloss.Width(responsePane),
					panesWidth,
					model.width,
					model.sidebarWidthPx,
					editorOuter,
					responseOuter,
					listWidth,
					listConfiguredWidth,
					editorViewWidth,
					responseViewWidth,
				)
			}
		})
	}
}

func TestFrameSizes(t *testing.T) {
	cfg := Config{WorkspaceRoot: t.TempDir()}
	model := New(cfg)
	styleResponse := model.theme.ResponseBorder
	styleResponseFocused := styleResponse.BorderStyle(lipgloss.ThickBorder())

	if base := styleResponse.GetHorizontalFrameSize(); base != 2 {
		t.Fatalf("expected base frame size 2, got %d", base)
	}
	if focused := styleResponseFocused.GetHorizontalFrameSize(); focused != 2 {
		t.Fatalf("expected focused frame size 2, got %d", focused)
	}
	probe := styleResponse.Width(10).Render(strings.Repeat("X", 10))
	if w := lipgloss.Width(probe); w != 12 {
		t.Fatalf("expected width 12 with Width(10), got %d", w)
	}
	probe = styleResponse.Width(10).MaxWidth(10).Render(strings.Repeat("X", 10))
	if w := lipgloss.Width(probe); w != 10 {
		t.Fatalf("expected width 10 with Width/MaxWidth(10), got %d", w)
	}
}
