package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func newMouseTestModel(t *testing.T) *Model {
	t.Helper()
	tmp := t.TempDir()
	file := filepath.Join(tmp, "sample.http")
	content := "### first\nGET https://example.com/one\n\n### second\nGET https://example.com/two\n"
	writeSampleFile(t, file, content)

	model := New(Config{WorkspaceRoot: tmp, FilePath: file, InitialContent: content})
	model.frameWidth = 120
	model.frameHeight = 40
	model.width = 118
	model.height = 38
	model.ready = true
	if cmd := model.applyLayout(); cmd != nil {
		_ = cmd()
	}
	return &model
}

func clickNavigatorNode(t *testing.T, model *Model, id string) tea.MouseMsg {
	t.Helper()
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.file, model.sidebarFrameStyle(model.navigatorPaneFocused()))
	if body.Empty() {
		t.Fatalf("expected navigator body rect")
	}
	row := -1
	for idx, visible := range model.navigator.VisibleRows() {
		if visible.Node != nil && visible.Node.ID == id {
			row = idx
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected visible navigator node %s", id)
	}
	return tea.MouseMsg{
		X:      body.x + 2,
		Y:      body.y + row,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
}

func TestCurrentMouseLayoutMatchesRenderedPaneGeometry(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*testing.T, *Model)
	}{
		{name: "vertical"},
		{
			name: "horizontal",
			configure: func(t *testing.T, m *Model) {
				m.mainSplitOrientation = mainSplitHorizontal
			},
		},
		{
			name: "response collapsed",
			configure: func(t *testing.T, m *Model) {
				if res := m.setCollapseState(paneRegionResponse, true); res.blocked {
					t.Fatalf("collapse response: %s", res.reason)
				}
			},
		},
		{
			name: "editor collapsed",
			configure: func(t *testing.T, m *Model) {
				if res := m.setCollapseState(paneRegionEditor, true); res.blocked {
					t.Fatalf("collapse editor: %s", res.reason)
				}
			},
		},
		{
			name: "wide editor",
			configure: func(t *testing.T, m *Model) {
				m.width = 900
				m.frameWidth = 902
			},
		},
		{
			name: "wide line numbers",
			configure: func(t *testing.T, m *Model) {
				var b strings.Builder
				for i := 0; i < 120; i++ {
					b.WriteString("GET https://example.com\n")
				}
				m.editor.SetValue(b.String())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := newMouseTestModel(t)
			if tt.configure != nil {
				tt.configure(t, model)
			}
			if cmd := model.applyLayout(); cmd != nil {
				_ = cmd()
			}

			got := normalizeEmptyMouseRects(model.currentMouseLayout())
			want := normalizeEmptyMouseRects(renderedMouseLayout(model))
			if got != want {
				t.Fatalf("layout mismatch\ngot:  %#v\nwant: %#v", got, want)
			}
		})
	}
}

func normalizeEmptyMouseRects(ly mouseLayout) mouseLayout {
	if ly.file.w <= 0 {
		ly.file.h = 0
	}
	if ly.editor.w <= 0 {
		ly.editor.h = 0
	}
	if ly.response.w <= 0 {
		ly.response.h = 0
	}
	return ly
}

func renderedMouseLayout(m *Model) mouseLayout {
	originX, originY := m.appFrameContentOrigin()
	headerH := lipgloss.Height(m.renderHeader())
	commandH := lipgloss.Height(m.renderCommandBar())
	panesY := originY + headerH + commandH

	filePane := m.renderFilePane(renderContext{})
	editorPane := m.renderEditorPane(renderContext{})
	fileW := lipgloss.Width(filePane)
	fileH := lipgloss.Height(filePane)
	editorW := lipgloss.Width(editorPane)
	editorH := lipgloss.Height(editorPane)

	ly := mouseLayout{
		file: mouseRect{x: originX, y: panesY, w: fileW, h: fileH},
	}

	if m.mainSplitOrientation == mainSplitHorizontal {
		rightX := originX + fileW
		ly.editor = mouseRect{x: rightX, y: panesY, w: editorW, h: editorH}

		responseW := max(editorW, m.width-fileW)
		if responseW < 0 {
			responseW = 0
		}
		responsePane := m.renderResponsePane(responseW, renderContext{})
		ly.response = mouseRect{
			x: rightX,
			y: panesY + editorH,
			w: lipgloss.Width(responsePane),
			h: lipgloss.Height(responsePane),
		}
		return ly
	}

	ly.editor = mouseRect{x: originX + fileW, y: panesY, w: editorW, h: editorH}
	responseW := m.responseTargetWidth(fileW, editorW)
	responsePane := ""
	if responseW > 0 {
		responsePane = m.renderResponsePane(responseW, renderContext{})
	}
	ly.response = mouseRect{
		x: originX + fileW + editorW,
		y: panesY,
		w: lipgloss.Width(responsePane),
		h: lipgloss.Height(responsePane),
	}
	return ly
}

func TestMouseClickNavigatorSelectsVisibleRow(t *testing.T) {
	model := newMouseTestModel(t)
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.file, model.sidebarFrameStyle(model.navigatorPaneFocused()))
	if body.Empty() {
		t.Fatalf("expected navigator body rect")
	}

	model.handleMouse(tea.MouseMsg{
		X:      body.x + 2,
		Y:      body.y + 2,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	wantID := navigatorRequestID(model.currentFile, 1)
	if sel := model.navigator.Selected(); sel == nil || sel.ID != wantID {
		t.Fatalf("expected navigator selection %s, got %#v", wantID, sel)
	}
	if model.focus != focusRequests {
		t.Fatalf("expected request focus after clicking request row, got %v", model.focus)
	}
}

func TestMouseWheelNavigatorMovesThreeRows(t *testing.T) {
	model := newMouseTestModel(t)
	model.navigator.SelectFirst()
	rows := model.navigator.VisibleRows()
	wantIndex := min(mouseWheelLines, len(rows)-1)
	if wantIndex <= 0 || rows[wantIndex].Node == nil {
		t.Fatalf("expected enough visible navigator rows, got %#v", rows)
	}
	wantID := rows[wantIndex].Node.ID

	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.file, model.sidebarFrameStyle(model.navigatorPaneFocused()))
	if body.Empty() {
		t.Fatalf("expected navigator body rect")
	}

	updated, _ := model.Update(tea.MouseMsg{
		X:      body.x,
		Y:      body.y,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	gotModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model from Update, got %T", updated)
	}
	if sel := gotModel.navigator.Selected(); sel == nil || sel.ID != wantID {
		t.Fatalf("expected wheel down to select %q, got %#v", wantID, sel)
	}
}

func TestMouseDoubleClickRequestFileOpensAndExpands(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "a.http")
	fileB := filepath.Join(tmp, "b.http")
	writeSampleFile(t, fileA, "### alpha\nGET https://example.com/a\n")
	writeSampleFile(t, fileB, "### beta\nGET https://example.com/b\n")

	model := New(Config{
		WorkspaceRoot:   tmp,
		FilePath:        fileB,
		InitialContent:  "### beta\nGET https://example.com/b\n",
		Recursive:       false,
		EnableUpdate:    false,
		CompareTargets:  nil,
		EnvironmentName: "",
	})
	model.frameWidth = 120
	model.frameHeight = 40
	model.width = 118
	model.height = 38
	model.ready = true
	if cmd := model.applyLayout(); cmd != nil {
		_ = cmd()
	}

	id := "file:" + fileA
	msg := clickNavigatorNode(t, &model, id)
	if cmd, _ := model.handleMouse(msg); cmd != nil {
		_ = cmd()
	}
	if cmd, _ := model.handleMouse(msg); cmd != nil {
		_ = cmd()
	}

	if model.currentFile != fileA {
		t.Fatalf("expected current file %q, got %q", fileA, model.currentFile)
	}
	node := model.navigator.Find(id)
	if node == nil || !node.Expanded || len(node.Children) == 0 {
		t.Fatalf("expected opened request file to be expanded with children, got %#v", node)
	}
}

func TestMouseDoubleClickNavigatorRequestJumpsToEditor(t *testing.T) {
	model := newMouseTestModel(t)
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.file, model.sidebarFrameStyle(model.navigatorPaneFocused()))
	if body.Empty() {
		t.Fatalf("expected navigator body rect")
	}

	msg := tea.MouseMsg{
		X:      body.x + 2,
		Y:      body.y + 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	if cmd, _ := model.handleMouse(msg); cmd != nil {
		_ = cmd()
	}
	if cmd, _ := model.handleMouse(msg); cmd != nil {
		_ = cmd()
	}

	if model.focus != focusEditor {
		t.Fatalf("expected double-clicked request to focus editor, got %v", model.focus)
	}
	if len(model.doc.Requests) == 0 || model.doc.Requests[0] == nil {
		t.Fatalf("expected parsed request")
	}
	wantLine := model.doc.Requests[0].LineRange.Start
	if got := currentCursorLine(model.editor); got != wantLine {
		t.Fatalf("expected cursor to jump to first request line %d, got %d", wantLine, got)
	}
	if model.responseLatest != nil {
		t.Fatalf("expected double-clicked request not to preview response, got %#v", model.responseLatest)
	}
}

func TestMouseClickEditorFocusesAndMovesCursor(t *testing.T) {
	model := newMouseTestModel(t)
	_ = model.setFocus(focusResponse)
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.editor, model.editorFrameStyle(model.focus == focusEditor))
	if body.Empty() {
		t.Fatalf("expected editor body rect")
	}

	model.handleMouse(tea.MouseMsg{
		X:      body.x + model.editorMouseReservedWidth() + 2,
		Y:      body.y + 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if model.focus != focusEditor {
		t.Fatalf("expected editor focus, got %v", model.focus)
	}
	if got := currentCursorLine(model.editor); got != 2 {
		t.Fatalf("expected cursor on line 2, got %d", got)
	}
}

func TestMouseDragEditorGutterSelectsLines(t *testing.T) {
	model := newMouseTestModel(t)
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.editor, model.editorFrameStyle(model.focus == focusEditor))
	if body.Empty() {
		t.Fatalf("expected editor body rect")
	}

	model.handleMouse(tea.MouseMsg{
		X:      body.x,
		Y:      body.y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	model.handleMouse(tea.MouseMsg{
		X:      body.x,
		Y:      body.y + 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	})
	model.handleMouse(tea.MouseMsg{
		X:      body.x,
		Y:      body.y + 1,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	if !model.editor.hasSelection() {
		t.Fatalf("expected editor line selection")
	}
	selected := model.editor.selectedText()
	if !strings.Contains(selected, "### first") || !strings.Contains(selected, "GET https://example.com/one") {
		t.Fatalf("expected first request lines selected, got %q", selected)
	}
}

func TestMouseDragEditorIncludesCharacterUnderCursor(t *testing.T) {
	model := newMouseTestModel(t)
	model.editor.SetValue("standard\n")
	if cmd := model.applyLayout(); cmd != nil {
		_ = cmd()
	}
	_ = model.setFocus(focusEditor)
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.editor, model.editorFrameStyle(model.focus == focusEditor))
	if body.Empty() {
		t.Fatalf("expected editor body rect")
	}

	x := body.x + model.editorMouseReservedWidth()
	y := body.y
	dragEditorMouse(model, x, y, x+len("standard")-1, y)

	if got := model.editor.selectedText(); got != "standard" {
		t.Fatalf("expected selected word, got %q", got)
	}
}

func TestMouseDragEditorCanEndInWhitespaceAfterText(t *testing.T) {
	model := newMouseTestModel(t)
	model.editor.SetValue("standard\n")
	if cmd := model.applyLayout(); cmd != nil {
		_ = cmd()
	}
	_ = model.setFocus(focusEditor)
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.editor, model.editorFrameStyle(model.focus == focusEditor))
	if body.Empty() {
		t.Fatalf("expected editor body rect")
	}

	x := body.x + model.editorMouseReservedWidth()
	y := body.y
	dragEditorMouse(model, x, y, x+len("standard")+4, y)

	if got := model.editor.selectedText(); got != "standard" {
		t.Fatalf("expected selected word, got %q", got)
	}
}

func TestMouseDragEditorBackwardIncludesAnchorCharacter(t *testing.T) {
	model := newMouseTestModel(t)
	model.editor.SetValue("standard\n")
	if cmd := model.applyLayout(); cmd != nil {
		_ = cmd()
	}
	_ = model.setFocus(focusEditor)
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.editor, model.editorFrameStyle(model.focus == focusEditor))
	if body.Empty() {
		t.Fatalf("expected editor body rect")
	}

	x := body.x + model.editorMouseReservedWidth()
	y := body.y
	dragEditorMouse(model, x+len("standard")-1, y, x, y)

	if got := model.editor.selectedText(); got != "standard" {
		t.Fatalf("expected selected word, got %q", got)
	}
}

func TestMouseDragEditorCanReverseDirection(t *testing.T) {
	model := newMouseTestModel(t)
	model.editor.SetValue("standard\n")
	if cmd := model.applyLayout(); cmd != nil {
		_ = cmd()
	}
	_ = model.setFocus(focusEditor)
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.editor, model.editorFrameStyle(model.focus == focusEditor))
	if body.Empty() {
		t.Fatalf("expected editor body rect")
	}

	x := body.x + model.editorMouseReservedWidth()
	y := body.y
	model.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	model.handleMouse(tea.MouseMsg{
		X:      x + len("standard") - 1,
		Y:      y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	})
	model.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	})
	model.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonNone,
		Action: tea.MouseActionRelease,
	})

	if got := model.editor.selectedText(); got != "s" {
		t.Fatalf("expected selection to follow reversed drag, got %q", got)
	}
}

func TestMouseClickEditorDoesNotSelectCharacter(t *testing.T) {
	model := newMouseTestModel(t)
	model.editor.SetValue("standard\n")
	if cmd := model.applyLayout(); cmd != nil {
		_ = cmd()
	}
	_ = model.setFocus(focusEditor)
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.editor, model.editorFrameStyle(model.focus == focusEditor))
	if body.Empty() {
		t.Fatalf("expected editor body rect")
	}

	x := body.x + model.editorMouseReservedWidth()
	y := body.y
	model.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	model.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})

	if model.editor.hasSelection() || model.editor.selectedText() != "" {
		t.Fatalf("expected plain click not to select text, got %q", model.editor.selectedText())
	}
}

func dragEditorMouse(model *Model, startX, startY, endX, endY int) {
	model.handleMouse(tea.MouseMsg{
		X:      startX,
		Y:      startY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	model.handleMouse(tea.MouseMsg{
		X:      endX,
		Y:      endY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	})
	model.handleMouse(tea.MouseMsg{
		X:      endX,
		Y:      endY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})
}

func TestMouseClickResponseTabActivatesTab(t *testing.T) {
	model := newMouseTestModel(t)
	ly := model.currentMouseLayout()
	hit := model.responseMouseHit(ly.response, ly.response.x+2, ly.response.y+2)
	if !hit.ok {
		t.Fatalf("expected response pane hit")
	}

	row := firstLine(ansi.Strip(model.renderPaneTabs(hit.id, true, hit.rect.w)))
	tabX := strings.Index(row, "History")
	if tabX < 0 {
		t.Fatalf("expected History tab in %q", row)
	}
	model.handleMouse(tea.MouseMsg{
		X:      hit.rect.x + tabX,
		Y:      hit.rect.y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if pane := model.pane(hit.id); pane == nil || pane.activeTab != responseTabHistory {
		t.Fatalf("expected history tab to be active, got %#v", pane)
	}
	if model.focus != focusResponse {
		t.Fatalf("expected response focus, got %v", model.focus)
	}
}

func TestFindVisibleTabRangeUsesCellsAfterUnicodeIndicator(t *testing.T) {
	line := tabIndicatorPrefix + "Pretty Raw Headers"

	pretty := findVisibleTabRange(line, "Pretty", 0)
	if !pretty.ok {
		t.Fatalf("expected Pretty tab range")
	}
	if pretty.start != 0 {
		t.Fatalf("expected Pretty to start at cell 0, got %d", pretty.start)
	}
	if want := lipgloss.Width(tabIndicatorPrefix + "Pretty"); pretty.end != want {
		t.Fatalf("expected Pretty to end at cell %d, got %d", want, pretty.end)
	}

	raw := findVisibleTabRange(line, "Raw", pretty.next)
	if !raw.ok {
		t.Fatalf("expected Raw tab range")
	}
	wantStart := lipgloss.Width(tabIndicatorPrefix + "Pretty ")
	wantEnd := wantStart + lipgloss.Width("Raw")
	if raw.start != wantStart || raw.end != wantEnd {
		t.Fatalf("expected Raw cells [%d,%d), got [%d,%d)", wantStart, wantEnd, raw.start, raw.end)
	}
}

func TestMouseOutsidePanesNotConsumed(t *testing.T) {
	model := newMouseTestModel(t)

	cmd, consumed := model.handleMouse(tea.MouseMsg{
		X:      model.frameWidth + 10,
		Y:      model.frameHeight + 10,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if consumed {
		t.Fatalf("expected outside click not to be consumed")
	}
	if cmd != nil {
		t.Fatalf("expected no command for outside click")
	}
}

func TestMouseModalDoesNotConsume(t *testing.T) {
	model := newMouseTestModel(t)
	model.showHelp = true
	ly := model.currentMouseLayout()
	body := model.paneBodyRect(ly.editor, model.editorFrameStyle(model.focus == focusEditor))
	if body.Empty() {
		t.Fatalf("expected editor body rect")
	}

	cmd, consumed := model.handleMouse(tea.MouseMsg{
		X:      body.x + model.editorMouseReservedWidth(),
		Y:      body.y,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	if consumed {
		t.Fatalf("expected modal mouse click not to be consumed")
	}
	if cmd != nil {
		t.Fatalf("expected no command while modal is active")
	}
}

func TestMouseWheelHelpModalScrollsViewport(t *testing.T) {
	model := newMouseTestModel(t)
	model.showHelp = true
	_, body := model.helpOverlayBox()
	if body.Empty() {
		t.Fatalf("expected help body rect")
	}

	updated, _ := model.Update(tea.MouseMsg{
		X:      body.x + 1,
		Y:      body.y,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	gotModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model from Update, got %T", updated)
	}
	if gotModel.helpViewport == nil {
		t.Fatalf("expected help viewport")
	}
	if got := gotModel.helpViewport.YOffset; got != mouseWheelLines {
		t.Fatalf("expected help wheel to scroll %d lines, got %d", mouseWheelLines, got)
	}
}

func TestMouseWheelHelpModalOutsideBodyIsConsumed(t *testing.T) {
	model := newMouseTestModel(t)
	model.showHelp = true
	_, body := model.helpOverlayBox()
	if body.Empty() {
		t.Fatalf("expected help body rect")
	}

	cmd, consumed := model.handleMouse(tea.MouseMsg{
		X:      body.x + body.w + 4,
		Y:      body.y,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	if !consumed {
		t.Fatalf("expected help wheel outside body to be consumed")
	}
	if cmd != nil {
		t.Fatalf("expected no command for help wheel")
	}
	if got := model.helpViewport.YOffset; got != 0 {
		t.Fatalf("expected help viewport not to scroll outside body, got %d", got)
	}
}

func TestResponseMouseHitSplitSecondaryPane(t *testing.T) {
	tests := []struct {
		name        string
		orientation responseSplitOrientation
	}{
		{name: "vertical", orientation: responseSplitVertical},
		{name: "horizontal", orientation: responseSplitHorizontal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := newMouseTestModel(t)
			model.responseSplit = true
			model.responseSplitOrientation = tt.orientation
			if cmd := model.applyLayout(); cmd != nil {
				_ = cmd()
			}

			ly := model.currentMouseLayout()
			body := model.paneBodyRect(ly.response, model.respFrameStyle(model.focus == focusResponse))
			if body.Empty() {
				t.Fatalf("expected response body rect")
			}

			primary := model.pane(responsePanePrimary)
			secondary := model.pane(responsePaneSecondary)
			if primary == nil || secondary == nil {
				t.Fatalf("expected split response panes")
			}

			primaryRect := responsePaneOuter(body.x, body.y, primary.viewport)
			secondaryRect := responsePaneOuter(
				primaryRect.x+primaryRect.w+responseSplitSeparatorWidth,
				body.y,
				secondary.viewport,
			)
			if tt.orientation == responseSplitHorizontal {
				secondaryRect = responsePaneOuter(
					body.x,
					primaryRect.y+primaryRect.h+responseSplitSeparatorHeight,
					secondary.viewport,
				)
			}

			hit := model.responseMouseHit(ly.response, secondaryRect.x, secondaryRect.y)
			if !hit.ok || hit.id != responsePaneSecondary {
				t.Fatalf("expected secondary pane hit, got %#v", hit)
			}
		})
	}
}

func TestMouseDoubleClickHistoryLoadsSelection(t *testing.T) {
	model := newMouseTestModel(t)
	entry := history.Entry{
		ID:          "hist-1",
		Method:      "GET",
		URL:         "https://example.com/history",
		RequestText: "### hist\nGET https://example.com/history\n",
	}
	model.historyEntries = []history.Entry{entry}
	model.historyList.SetItems(makeHistoryItems(model.historyEntries, model.historyScope))
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary response pane")
	}
	pane.setActiveTab(responseTabHistory)

	ly := model.currentMouseLayout()
	hit := model.responseMouseHit(ly.response, ly.response.x+2, ly.response.y+2)
	if !hit.ok {
		t.Fatalf("expected response pane hit")
	}
	msg := tea.MouseMsg{
		X:      hit.rect.x + 1,
		Y:      hit.rect.y + responseTabsHeight + model.historyHeaderHeight(),
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	if cmd, _ := model.handleMouse(msg); cmd != nil {
		_ = cmd()
	}
	if cmd, _ := model.handleMouse(msg); cmd != nil {
		_ = cmd()
	}

	if !strings.Contains(model.editor.Value(), "https://example.com/history") {
		t.Fatalf("expected history request to load into editor, got %q", model.editor.Value())
	}
}

func TestMouseWheelHistoryMovesThreeRows(t *testing.T) {
	model := newMouseTestModel(t)
	model.historyEntries = []history.Entry{
		{ID: "hist-1", Method: "GET", URL: "https://example.com/1"},
		{ID: "hist-2", Method: "GET", URL: "https://example.com/2"},
		{ID: "hist-3", Method: "GET", URL: "https://example.com/3"},
		{ID: "hist-4", Method: "GET", URL: "https://example.com/4"},
	}
	model.historyList.SetItems(makeHistoryItems(model.historyEntries, model.historyScope))
	model.historyList.Select(0)
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary response pane")
	}
	pane.setActiveTab(responseTabHistory)

	ly := model.currentMouseLayout()
	hit := model.responseMouseHit(ly.response, ly.response.x+2, ly.response.y+2)
	if !hit.ok {
		t.Fatalf("expected response pane hit")
	}
	updated, _ := model.Update(tea.MouseMsg{
		X:      hit.rect.x + 1,
		Y:      hit.rect.y + responseTabsHeight + model.historyHeaderHeight(),
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	gotModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model from Update, got %T", updated)
	}
	if got := gotModel.historyList.Index(); got != mouseWheelLines {
		t.Fatalf("expected history wheel down to move %d rows, got index %d", mouseWheelLines, got)
	}
}

func TestHistoryLoadDoesNotPoisonNavigatorRequests(t *testing.T) {
	model := newMouseTestModel(t)
	originalFile := model.currentFile
	originalReqID := navigatorRequestID(originalFile, 0)

	entry := history.Entry{
		ID:          "hist-2",
		Method:      "GET",
		URL:         "https://example.com/history",
		RequestText: "### hist\nGET https://example.com/history\n",
	}
	model.historyEntries = []history.Entry{entry}
	model.historyList.SetItems(makeHistoryItems(model.historyEntries, model.historyScope))
	model.historyList.Select(0)

	if cmd := model.loadHistorySelection(false); cmd != nil {
		_ = cmd()
	}
	if model.currentFile != "" {
		t.Fatalf("expected history load to leave editor as temporary document, got %q", model.currentFile)
	}
	if !model.navigator.SelectByID(originalReqID) {
		t.Fatalf("expected original request node to remain selectable")
	}
	if cmd := model.sendNavigatorRequest(false); cmd != nil {
		_ = cmd()
	}

	if model.currentFile != originalFile {
		t.Fatalf("expected navigator preview to reopen %q, got %q", originalFile, model.currentFile)
	}
	if model.responseLatest == nil ||
		!strings.Contains(model.responseLatest.pretty, "GET https://example.com/one") {
		t.Fatalf("expected original request preview after history load, got %#v", model.responseLatest)
	}
	if strings.Contains(strings.ToLower(model.statusMessage.text), "request not found") {
		t.Fatalf("unexpected stale navigator status: %q", model.statusMessage.text)
	}
}

func TestMouseWheelResponseScrollsViewport(t *testing.T) {
	model := newMouseTestModel(t)
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary response pane")
	}
	pane.setActiveTab(responseTabPretty)
	pane.viewport.SetContent(strings.Repeat("line\n", 80))

	ly := model.currentMouseLayout()
	hit := model.responseMouseHit(ly.response, ly.response.x+2, ly.response.y+2)
	if !hit.ok {
		t.Fatalf("expected response pane hit")
	}
	model.handleMouse(tea.MouseMsg{
		X:      hit.rect.x + 1,
		Y:      hit.rect.y + responseTabsHeight,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	if pane.viewport.YOffset == 0 {
		t.Fatalf("expected wheel down to scroll response viewport")
	}
}

func TestUpdateMouseWheelResponseScrollsOnce(t *testing.T) {
	model := newMouseTestModel(t)
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary response pane")
	}
	pane.setActiveTab(responseTabPretty)
	pane.viewport.SetContent(strings.Repeat("line\n", 80))

	ly := model.currentMouseLayout()
	hit := model.responseMouseHit(ly.response, ly.response.x+2, ly.response.y+2)
	if !hit.ok {
		t.Fatalf("expected response pane hit")
	}
	updated, _ := model.Update(tea.MouseMsg{
		X:      hit.rect.x + 1,
		Y:      hit.rect.y + responseTabsHeight,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	gotModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model from Update, got %T", updated)
	}
	pane = gotModel.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary response pane after update")
	}
	if got := pane.viewport.YOffset; got != mouseWheelLines {
		t.Fatalf("expected one wheel scroll (%d lines), got offset %d", mouseWheelLines, got)
	}
}
