package ui

import (
	"strings"
	"testing"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
)

const sampleRequestDoc = "### example\n# @name getExample\nGET https://example.com\n"

func newTestModelWithDoc(content string) *Model {
	model := New(Config{})
	model.editor.SetValue(content)
	model.doc = parser.Parse(model.currentFile, []byte(content))
	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 0)
	return &model
}

func sendKeys(t *testing.T, model *Model, keys ...string) {
	t.Helper()
	for _, key := range keys {
		msg := keyMsgFor(key)
		if cmd := model.handleKey(msg); cmd != nil {
			_ = cmd()
		}
	}
}

func keyMsgFor(key string) tea.KeyMsg {
	switch key {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		runes := []rune(key)
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: runes}
	}
}

func TestHandleKeyEnterInViewModeSends(t *testing.T) {
	model := newTestModelWithDoc(sampleRequestDoc)
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	model.moveCursorToLine(2)
	if res := model.setCollapseState(paneRegionResponse, true); res.blocked {
		t.Fatalf("expected response collapse to be allowed")
	}
	if !model.collapseState(paneRegionResponse) {
		t.Fatalf("expected response to start collapsed")
	}

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected enter key to trigger command in view mode")
	}
	if model.collapseState(paneRegionResponse) {
		t.Fatalf("expected response pane to be restored")
	}
}

func TestHandleKeyEnterInInsertModeDoesNotSend(t *testing.T) {
	model := newTestModelWithDoc(sampleRequestDoc)
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(true, false)
	model.moveCursorToLine(2)

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected enter key to be ignored in insert mode")
	}
}

func TestCancelShortcutStopsInFlightSend(t *testing.T) {
	model := New(Config{})
	model.sending = true
	canceled := false
	model.sendCancel = func() { canceled = true }

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd != nil {
		_ = cmd()
	}

	if model.sending {
		t.Fatalf("expected sending flag cleared after cancel shortcut")
	}
	if !canceled {
		t.Fatalf("expected cancel function to be invoked")
	}
	if text := strings.ToLower(model.statusMessage.text); !strings.Contains(text, "canceling") {
		t.Fatalf("expected cancel status message, got %q", model.statusMessage.text)
	}
}

func TestTabInViewModeCyclesFocus(t *testing.T) {
	model := newTestModelWithDoc(sampleRequestDoc)
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if cmd != nil {
		t.Fatalf("expected tab focus change to produce no command")
	}
	if model.focus != focusResponse {
		t.Fatalf("expected focus to move to response pane, got %v", model.focus)
	}
	if !model.suppressEditorKey {
		t.Fatalf("expected editor key suppression to be enabled after focus change")
	}
}

func TestHandleKeyIgnoredWhileErrorModalVisible(t *testing.T) {
	model := newTestModelWithDoc(sampleRequestDoc)
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	model.moveCursorToLine(2)
	model.showErrorModal = true

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected no command when error modal is visible")
	}
	if model.sending {
		t.Fatalf("expected sending state to remain false when dismissing error modal")
	}
}

func TestHandleKeyGhShrinksEditor(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.applyLayout()
	initialEditor := model.editor.Width()
	if initialEditor <= 0 {
		t.Fatalf("expected initial editor width to be positive, got %d", initialEditor)
	}

	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if model.editor.Width() >= initialEditor {
		t.Fatalf(
			"expected gh to shrink editor width, initial %d new %d",
			initialEditor,
			model.editor.Width(),
		)
	}
}

func TestHandleKeyGlExpandsEditor(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.applyLayout()
	initialEditor := model.editor.Width()
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if model.editor.Width() <= initialEditor {
		t.Fatalf(
			"expected gl to expand editor width, initial %d new %d",
			initialEditor,
			model.editor.Width(),
		)
	}
}

func TestHandleKeyGhCanRepeatWithoutPrefix(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.applyLayout()
	start := model.editor.Width()
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	first := model.editor.Width()
	if first >= start {
		t.Fatalf("expected gh to shrink editor width, before %d after %d", start, first)
	}
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	second := model.editor.Width()
	if second >= first {
		t.Fatalf(
			"expected repeated h to continue shrinking editor, previous %d new %d",
			first,
			second,
		)
	}
	if !model.repeatChordActive {
		t.Fatalf("expected chord repeat to remain active after repeated action")
	}
}

func TestHandleKeyGhIgnoredInInsertMode(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(true, false)
	_ = model.applyLayout()
	initialEditor := model.editor.Width()

	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if model.editor.Width() != initialEditor {
		t.Fatalf(
			"expected gh chord to be ignored in insert mode, initial %d new %d",
			initialEditor,
			model.editor.Width(),
		)
	}
	if model.hasPendingChord {
		t.Fatalf("expected pending chord state to clear when insert mode intercepts")
	}
	if model.repeatChordActive {
		t.Fatalf("expected chord repeat to remain inactive in insert mode")
	}
	if model.suppressListKey {
		t.Fatalf("expected list suppression to reset in insert mode")
	}
}

func TestHandleKeyGhShrinksSidebarWhenFocused(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 200
	model.height = 60
	model.ready = true
	_ = model.setFocus(focusFile)
	_ = model.applyLayout()
	initialSidebar := model.sidebarWidthPx
	initialEditor := model.editor.Width()

	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	if model.sidebarWidthPx >= initialSidebar {
		t.Fatalf(
			"expected gh to shrink sidebar width, initial %d new %d",
			initialSidebar,
			model.sidebarWidthPx,
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

func TestHandleKeyGlExpandsSidebarWhenFocused(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 200
	model.height = 60
	model.ready = true
	_ = model.setFocus(focusRequests)
	_ = model.applyLayout()
	initialSidebar := model.sidebarWidthPx
	initialEditor := model.editor.Width()

	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	if model.sidebarWidthPx <= initialSidebar {
		t.Fatalf(
			"expected gl to expand sidebar width, initial %d new %d",
			initialSidebar,
			model.sidebarWidthPx,
		)
	}
	if model.editor.Width() >= initialEditor {
		t.Fatalf(
			"expected editor width to shrink after sidebar expands, initial %d new %d",
			initialEditor,
			model.editor.Width(),
		)
	}
}

func TestRunEditorResizeBlockedByZoom(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 60
	model.ready = true
	_ = model.applyLayout()
	_ = model.toggleZoomForRegion(paneRegionEditor)
	initial := model.editor.Width()
	cmd := model.runEditorResize(editorSplitStep)
	if cmd != nil {
		t.Fatalf("expected resize command to be suppressed while zoomed")
	}
	if model.editor.Width() != initial {
		t.Fatalf(
			"expected editor width to remain unchanged while zoomed, initial %d new %d",
			initial,
			model.editor.Width(),
		)
	}
	if !strings.Contains(model.statusMessage.text, "Disable zoom") {
		t.Fatalf("expected zoom warning status, got %q", model.statusMessage.text)
	}
}

func TestRunEditorResizeBlockedWhenCollapsed(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 60
	model.ready = true
	_ = model.applyLayout()
	if res := model.setCollapseState(paneRegionResponse, true); res.blocked {
		t.Fatalf("expected collapse to be allowed")
	}
	initial := model.editor.Width()
	cmd := model.runEditorResize(-editorSplitStep)
	if cmd != nil {
		t.Fatalf("expected resize command to be suppressed while response collapsed")
	}
	if model.editor.Width() != initial {
		t.Fatalf(
			"expected editor width to remain unchanged while collapsed, initial %d new %d",
			initial,
			model.editor.Width(),
		)
	}
	if !strings.Contains(model.statusMessage.text, "Expand panes") {
		t.Fatalf("expected collapse warning status, got %q", model.statusMessage.text)
	}
}

func TestHandleKeyGjAdjustsSidebar(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	_ = model.setFocus(focusFile)
	_ = model.applyLayout()
	initialIndex := model.fileList.Index()
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if model.fileList.Index() != initialIndex {
		t.Fatalf(
			"expected gj chord not to move file selection, initial %d new %d",
			initialIndex,
			model.fileList.Index(),
		)
	}
}

func TestHandleKeyGjCanRepeatWithoutPrefix(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	_ = model.setFocus(focusFile)
	_ = model.applyLayout()
	initialIndex := model.fileList.Index()
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if !model.repeatChordActive {
		t.Fatalf("expected chord repeat to remain active after repeated sidebar adjustment")
	}
	if model.fileList.Index() != initialIndex {
		t.Fatalf(
			"expected repeated gj not to move file selection, initial %d new %d",
			initialIndex,
			model.fileList.Index(),
		)
	}
}

func TestHandleKeyGkAdjustsSidebar(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	_ = model.setFocus(focusRequests)
	_ = model.applyLayout()
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
}

func TestChordFallbackMaintainsEditorMotions(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 120
	model.height = 40
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.applyLayout()
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if model.hasPendingChord {
		t.Fatalf("expected pending chord to be cleared after fallback processing")
	}
	if model.editor.pendingMotion != "" {
		t.Fatalf("expected editor pending motion to be cleared, got %q", model.editor.pendingMotion)
	}
	if model.repeatChordActive {
		t.Fatalf("expected repeat chord state to be cleared after fallback")
	}
}

func TestHandleKeyDDeletesSelection(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 120
	model.height = 40
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	model.editor.SetValue("alpha")

	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 0)
	start := model.editor.caretPosition()
	editorPtr.startSelection(start, selectionManual)
	editorPtr.selection.Update(cursorPosition{Line: 0, Column: 5, Offset: 5})
	editorPtr.applySelectionHighlight()

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatalf("expected delete selection to emit command")
	}
	if got := model.editor.Value(); got != "" {
		t.Fatalf("expected selection to be removed, got %q", got)
	}
	if model.editor.hasSelection() {
		t.Fatal("expected selection to be cleared after delete")
	}
	if model.hasPendingChord {
		t.Fatal("expected chord state to remain inactive after selection delete")
	}
}

func TestHandleKeyDWithoutSelectionStartsOperator(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 120
	model.height = 40
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	model.editor.SetValue("alpha")

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("expected no immediate message for pending delete, got %T", msg)
		}
	}
	if !model.operator.active {
		t.Fatal("expected delete operator to enter pending state")
	}
	if model.operator.operator != "d" {
		t.Fatalf("expected delete operator type d, got %q", model.operator.operator)
	}
	if len(model.operator.motionKeys) != 0 {
		t.Fatalf("expected no motions captured yet, got %v", model.operator.motionKeys)
	}
	if model.hasPendingChord {
		t.Fatal("expected no chord state when delete operator pending")
	}
	if model.editor.Value() != "alpha" {
		t.Fatalf("expected content to remain unchanged, got %q", model.editor.Value())
	}
	if model.dirty {
		t.Fatal("expected dirty to remain false when operator pending")
	}
}

func TestDeleteOperatorDw(t *testing.T) {
	model := newTestModelWithDoc("word another")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "w")

	if got := model.editor.Value(); got != "another" {
		t.Fatalf("expected dw to remove word, got %q", got)
	}
	if model.operator.active {
		t.Fatal("expected operator state to clear after deletion")
	}
	if !model.dirty {
		t.Fatal("expected model marked dirty after deletion")
	}
}

func TestDeleteOperatorDe(t *testing.T) {
	model := newTestModelWithDoc("word another")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "e")

	if got := model.editor.Value(); got != " another" {
		t.Fatalf("expected de to remove to end of word, got %q", got)
	}
}

func TestDeleteOperatorDb(t *testing.T) {
	model := newTestModelWithDoc("alpha beta")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, len("alpha "))

	sendKeys(t, model, "d", "b")

	if got := model.editor.Value(); got != "beta" {
		t.Fatalf("expected db to remove previous word, got %q", got)
	}
}

func TestDeleteOperatorDW(t *testing.T) {
	model := newTestModelWithDoc("foo, bar baz")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "W")

	if got := model.editor.Value(); got != "bar baz" {
		t.Fatalf("expected dW to remove WORD, got %q", got)
	}
}

func TestDeleteOperatorDE(t *testing.T) {
	model := newTestModelWithDoc("foo,bar baz")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "E")

	if got := model.editor.Value(); got != " baz" {
		t.Fatalf("expected dE to remove to end of WORD, got %q", got)
	}
}

func TestDeleteOperatorDB(t *testing.T) {
	model := newTestModelWithDoc("foo, bar")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, len("foo, "))

	sendKeys(t, model, "d", "B")

	if got := model.editor.Value(); got != "bar" {
		t.Fatalf("expected dB to remove previous WORD, got %q", got)
	}
}

func TestChangeOperatorCw(t *testing.T) {
	model := newTestModelWithDoc("alpha beta")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "c", "w")

	if !model.editorInsertMode {
		t.Fatalf("expected change to enter insert mode")
	}
	if got := model.editor.Value(); got != " beta" {
		t.Fatalf("expected cw to change word, got %q", got)
	}
	if model.operator.active {
		t.Fatal("expected operator state to clear after change")
	}
}

func TestChangeOperatorCj(t *testing.T) {
	model := newTestModelWithDoc("first\nsecond\nthird")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "c", "j")

	if !model.editorInsertMode {
		t.Fatalf("expected change to enter insert mode")
	}
	if got := model.editor.Value(); got != "\nthird" {
		t.Fatalf("expected cj to change two lines, got %q", got)
	}
}

func TestDeleteOperatorDollar(t *testing.T) {
	model := newTestModelWithDoc("alpha beta")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, len("alpha "))

	sendKeys(t, model, "d", "$")

	if got := model.editor.Value(); got != "alpha " {
		t.Fatalf("expected d$ to trim to line end, got %q", got)
	}
}

func TestDeleteOperatorDd(t *testing.T) {
	model := newTestModelWithDoc("first\nsecond\nthird")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "d")

	if got := model.editor.Value(); got != "second\nthird" {
		t.Fatalf("expected dd to remove current line, got %q", got)
	}
}

func TestDeleteOperatorDj(t *testing.T) {
	model := newTestModelWithDoc("first\nsecond\nthird")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "j")

	if got := model.editor.Value(); got != "third" {
		t.Fatalf("expected dj to remove two lines, got %q", got)
	}
}

func TestDeleteOperatorDgg(t *testing.T) {
	model := newTestModelWithDoc("first\nsecond\nthird")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	editorPtr := &model.editor
	editorPtr.moveCursorTo(2, 0)

	sendKeys(t, model, "d", "g", "g")

	if got := model.editor.Value(); got != "" {
		t.Fatalf("expected dgg to remove from cursor to top, got %q", got)
	}
}

func TestDeleteOperatorDG(t *testing.T) {
	model := newTestModelWithDoc("first\nsecond\nthird")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "G")

	if got := model.editor.Value(); got != "" {
		t.Fatalf("expected dG to remove from cursor to end, got %q", got)
	}
}

func TestDeleteOperatorDf(t *testing.T) {
	model := newTestModelWithDoc("abcdeffg")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "f", "f")

	if got := model.editor.Value(); got != "fg" {
		t.Fatalf("expected dff to delete through target char, got %q", got)
	}
}

func TestDeleteOperatorDt(t *testing.T) {
	model := newTestModelWithDoc("abcdeffg")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "t", "f")

	if got := model.editor.Value(); got != "ffg" {
		t.Fatalf("expected dtf to delete until before target, got %q", got)
	}
}

func TestDeleteOperatorCancelEsc(t *testing.T) {
	model := newTestModelWithDoc("alpha")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	sendKeys(t, model, "d", "esc")

	if got := model.editor.Value(); got != "alpha" {
		t.Fatalf("expected cancel to preserve text, got %q", got)
	}
	if model.operator.active {
		t.Fatal("expected operator state cleared after cancel")
	}
}

func TestHandleKeyUUdoesUndo(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 120
	model.height = 40
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	model.editor.SetValue("alpha")

	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 0)
	start := model.editor.caretPosition()
	editorPtr.startSelection(start, selectionManual)
	editorPtr.selection.Update(cursorPosition{Line: 0, Column: 5, Offset: 5})
	editorPtr.applySelectionHighlight()

	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if got := model.editor.Value(); got != "" {
		t.Fatalf("expected deletion to clear content, got %q", got)
	}
	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if cmd == nil {
		t.Fatalf("expected undo command")
	}
	msg := cmd()
	evt, ok := msg.(editorEvent)
	if !ok {
		t.Fatalf("expected editorEvent, got %T", msg)
	}
	if evt.status == nil || evt.status.text != "Undid last change" {
		t.Fatalf("expected undo status, got %+v", evt.status)
	}
	if got := model.editor.Value(); got != "alpha" {
		t.Fatalf("expected undo to restore content, got %q", got)
	}
}

func TestHandleKeyCtrlRRedeos(t *testing.T) {
	model := newTestModelWithDoc("alpha")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 1)

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}); cmd != nil {
		_ = cmd()
	}
	if got := model.editor.Value(); got != "apha" {
		t.Fatalf("expected char deleted, got %q", got)
	}
	undoCmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if undoCmd == nil {
		t.Fatal("expected undo command")
	}
	if msg := undoCmd(); msg != nil {
		evt, ok := msg.(editorEvent)
		if !ok || evt.status == nil || evt.status.text != "Undid last change" {
			t.Fatalf("unexpected undo event: %+v", msg)
		}
	}
	if got := model.editor.Value(); got != "alpha" {
		t.Fatalf("expected undo to restore text, got %q", got)
	}
	redoCmd := model.handleKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if redoCmd == nil {
		t.Fatal("expected redo command")
	}
	msg := redoCmd()
	evt, ok := msg.(editorEvent)
	if !ok {
		t.Fatalf("expected editorEvent, got %T", msg)
	}
	if evt.status == nil || evt.status.text != "Redid last change" {
		t.Fatalf("expected redo status, got %+v", evt.status)
	}
	if got := model.editor.Value(); got != "apha" {
		t.Fatalf("expected redo to reapply deletion, got %q", got)
	}
}

func TestHandleKeyUWithoutHistoryShowsStatus(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 120
	model.height = 40
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	model.editor.SetValue("alpha")

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if cmd == nil {
		t.Fatalf("expected undo command")
	}
	msg := cmd()
	evt, ok := msg.(editorEvent)
	if !ok {
		t.Fatalf("expected editorEvent, got %T", msg)
	}
	if evt.status == nil || evt.status.text != "Nothing to undo" {
		t.Fatalf("expected no-history status, got %+v", evt.status)
	}
	if evt.dirty {
		t.Fatalf("expected dirty to remain false when no undo")
	}
	if got := model.editor.Value(); got != "alpha" {
		t.Fatalf("expected content unchanged, got %q", got)
	}
}

func TestHandleKeyDdDeletesLine(t *testing.T) {
	model := newTestModelWithDoc("alpha\nbeta\ncharlie")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}); cmd != nil {
		if msg := cmd(); msg != nil {
			if evt, ok := msg.(editorEvent); ok && evt.status != nil {
				t.Fatalf("unexpected status on first d: %+v", evt.status)
			}
		}
	}
	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatalf("expected command on second d")
	}
	msg := cmd()
	evt, ok := msg.(editorEvent)
	if !ok {
		t.Fatalf("expected editorEvent, got %T", msg)
	}
	expectStatusWithClipboardFallback(t, evt.status, "Deleted line")
	if got := model.editor.Value(); got != "beta\ncharlie" {
		t.Fatalf("expected first line removed, got %q", got)
	}
	if !model.dirty {
		t.Fatalf("expected model dirty after deletion")
	}
}

func TestHandleKeyDdDeletesLastLineWithoutBlank(t *testing.T) {
	model := newTestModelWithDoc("alpha\nbeta")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	model.moveCursorToLine(2)

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}); cmd != nil {
		if msg := cmd(); msg != nil {
			if evt, ok := msg.(editorEvent); ok && evt.status != nil {
				t.Fatalf("unexpected status on first d: %+v", evt.status)
			}
		}
	}
	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatalf("expected command on second d")
	}
	if msg := cmd(); msg != nil {
		evt, ok := msg.(editorEvent)
		if !ok {
			t.Fatalf("expected editorEvent, got %T", msg)
		}
		expectStatusWithClipboardFallback(t, evt.status, "Deleted line")
	}
	if got := model.editor.Value(); got != "alpha" {
		t.Fatalf("expected trailing line removed without blank, got %q", got)
	}
}

func TestHandleKeyDdDeletesTrailingBlankLine(t *testing.T) {
	model := newTestModelWithDoc("alpha\n")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	model.moveCursorToLine(2)

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}); cmd != nil {
		if msg := cmd(); msg != nil {
			if evt, ok := msg.(editorEvent); ok && evt.status != nil {
				t.Fatalf("unexpected status on first d: %+v", evt.status)
			}
		}
	}
	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatalf("expected command on second d")
	}
	if msg := cmd(); msg != nil {
		evt, ok := msg.(editorEvent)
		if !ok {
			t.Fatalf("expected editorEvent, got %T", msg)
		}
		expectStatusWithClipboardFallback(t, evt.status, "Deleted line")
	}
	if got := model.editor.Value(); got != "alpha" {
		t.Fatalf("expected trailing blank line removed, got %q", got)
	}
}

func TestHandleKeyDDeletesToLineEnd(t *testing.T) {
	model := newTestModelWithDoc("alpha beta\nsecond")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 6)

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	if cmd == nil {
		t.Fatalf("expected command for D")
	}
	_ = cmd()
	if got := model.editor.Value(); got != "alpha \nsecond" {
		t.Fatalf("expected remainder of line, got %q", got)
	}
}

func TestHandleKeyXDeletesCharacter(t *testing.T) {
	model := newTestModelWithDoc("xyz")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 1)

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatalf("expected command for x")
	}
	_ = cmd()
	if got := model.editor.Value(); got != "xz" {
		t.Fatalf("expected middle char removed, got %q", got)
	}
}

func TestHandleKeyCCChangesLineAndEntersInsert(t *testing.T) {
	model := newTestModelWithDoc("alpha\nbeta")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	model.moveCursorToLine(2)

	sendKeys(t, model, "c", "c")
	if !model.editorInsertMode {
		t.Fatalf("expected change to enter insert mode")
	}
	if got := model.editor.Value(); got != "alpha\n" {
		t.Fatalf("expected focused line cleared, got %q", got)
	}
}

func TestHandleKeyPasteAfter(t *testing.T) {
	if err := clipboard.WriteAll("ZZ"); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	model := newTestModelWithDoc("abc")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 1)

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil {
		t.Fatalf("expected paste command")
	}
	_ = cmd()
	if got := model.editor.Value(); got != "abZZc" {
		t.Fatalf("expected paste after cursor, got %q", got)
	}
}

func TestHandleKeyPasteBefore(t *testing.T) {
	if err := clipboard.WriteAll("ZZ"); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	model := newTestModelWithDoc("abc")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 1)

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
	if cmd == nil {
		t.Fatalf("expected paste command")
	}
	_ = cmd()
	if got := model.editor.Value(); got != "aZZbc" {
		t.Fatalf("expected paste before cursor, got %q", got)
	}
}

func TestHandleKeyFindMotionMovesCursor(t *testing.T) {
	model := newTestModelWithDoc("alphabet")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)
	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, 0)

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}); cmd != nil {
		if msg := cmd(); msg != nil {
			if evt, ok := msg.(editorEvent); ok && evt.status != nil {
				t.Fatalf("unexpected status for initial f: %+v", evt.status)
			}
		}
	}
	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}}); cmd != nil {
		_ = cmd()
	}
	pos := model.editor.caretPosition()
	if pos.Column != 5 {
		t.Fatalf("expected cursor at column 5 after fb, got %d", pos.Column)
	}
}

func TestHandleKeyFindPendingIgnoresDeleteOperator(t *testing.T) {
	model := newTestModelWithDoc("abcd")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}); cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("unexpected message from find operator: %T", msg)
		}
	}
	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}); cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("unexpected message when targeting find motion: %T", msg)
		}
	}
	if model.editor.awaitingFindTarget() {
		t.Fatal("expected find target to clear after applying target")
	}
	if model.pendingChord != "" {
		t.Fatalf("expected no chord pending, got %q", model.pendingChord)
	}
	if model.editorInsertMode {
		t.Fatal("expected to remain in view mode after find motion")
	}
	if got := model.editor.Value(); got != "abcd" {
		t.Fatalf("expected buffer unchanged, got %q", got)
	}
	pos := model.editor.caretPosition()
	if pos.Column != 2 {
		t.Fatalf("expected cursor before target d (column 2), got %d", pos.Column)
	}
}

func TestResponsePaneFocusChord(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyType
	}{
		{name: "CtrlF", key: tea.KeyCtrlF},
		{name: "CtrlB", key: tea.KeyCtrlB},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := New(Config{})
			model.ready = true
			model.width = 120
			model.height = 40
			if cmd := model.applyLayout(); cmd != nil {
				collectMsgs(cmd)
			}

			_ = model.setFocus(focusResponse)
			if out := model.toggleResponseSplitVertical(); out != nil {
				collectMsgs(out)
			}
			if !model.responseSplit {
				t.Fatalf("expected split to be enabled")
			}
			if model.responsePaneFocus != responsePanePrimary {
				t.Fatalf("expected initial focus on primary pane")
			}

			if cmd := model.handleKey(tea.KeyMsg{Type: tc.key}); cmd != nil {
				collectMsgs(cmd)
			}
			if !model.responsePaneChord {
				t.Fatalf("expected %s to arm pane chord", tc.name)
			}

			if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRight}); cmd != nil {
				collectMsgs(cmd)
			}
			if model.responsePaneFocus != responsePaneSecondary {
				t.Fatalf("expected chord to switch focus to secondary pane")
			}
			if model.responsePaneChord {
				t.Fatalf("expected chord to clear after navigation")
			}
		})
	}
}

func TestMainSplitOrientationChord(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 160
	model.height = 48
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	if model.mainSplitOrientation != mainSplitVertical {
		t.Fatalf("expected default orientation to be vertical")
	}
	baselineHeight := model.paneContentHeight
	if model.editorContentHeight != baselineHeight {
		t.Fatalf(
			"expected editor content height %d, got %d",
			baselineHeight,
			model.editorContentHeight,
		)
	}
	if model.responseContentHeight != baselineHeight {
		t.Fatalf(
			"expected response content height %d, got %d",
			baselineHeight,
			model.responseContentHeight,
		)
	}

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}); cmd != nil {
		collectMsgs(cmd)
	}
	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}); cmd != nil {
		collectMsgs(cmd)
	}
	if model.mainSplitOrientation != mainSplitHorizontal {
		t.Fatalf("expected g+s to switch to horizontal orientation")
	}
	if model.editorContentHeight >= baselineHeight {
		t.Fatalf(
			"expected editor content height to shrink below %d, got %d",
			baselineHeight,
			model.editorContentHeight,
		)
	}
	if model.responseContentHeight >= baselineHeight {
		t.Fatalf(
			"expected response content height to shrink below %d, got %d",
			baselineHeight,
			model.responseContentHeight,
		)
	}
	frameAllowance := model.theme.EditorBorder.GetVerticalFrameSize() + model.theme.ResponseBorder.GetVerticalFrameSize()
	expectedTotal := baselineHeight - frameAllowance
	if expectedTotal < 1 {
		expectedTotal = 1
	}
	combined := model.editorContentHeight + model.responseContentHeight
	if combined < expectedTotal-1 || combined > expectedTotal+1 {
		t.Fatalf("expected stacked heights near %d, got %d", expectedTotal, combined)
	}

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}); cmd != nil {
		collectMsgs(cmd)
	}
	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}); cmd != nil {
		collectMsgs(cmd)
	}
	if model.mainSplitOrientation != mainSplitVertical {
		t.Fatalf("expected g+v to restore vertical orientation")
	}
	if model.editorContentHeight != baselineHeight {
		t.Fatalf(
			"expected editor height reset to %d, got %d",
			baselineHeight,
			model.editorContentHeight,
		)
	}
	if model.responseContentHeight != baselineHeight {
		t.Fatalf(
			"expected response height reset to %d, got %d",
			baselineHeight,
			model.responseContentHeight,
		)
	}
}

func TestNavGateBlocksMismatchedKind(t *testing.T) {
	model := newTestModelWithDoc(sampleRequestDoc)
	m := model
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      "file:/tmp/a",
			Kind:    navigator.KindFile,
			Payload: navigator.Payload[any]{FilePath: "/tmp/a"},
		},
	})
	if m.navGate(navigator.KindRequest, "") {
		t.Fatalf("expected navGate to block non-request selection")
	}
}

func TestNavGateBlocksDifferentFile(t *testing.T) {
	model := newTestModelWithDoc(sampleRequestDoc)
	m := model
	m.currentFile = "/tmp/a.http"
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:      "req:/tmp/b:0",
			Kind:    navigator.KindRequest,
			Payload: navigator.Payload[any]{FilePath: "/tmp/b.http"},
		},
	})
	if m.navGate(navigator.KindRequest, "warn") {
		t.Fatalf("expected navGate to block request from different file")
	}
	if m.statusMessage.text != "warn" {
		t.Fatalf("expected status message to be set, got %q", m.statusMessage.text)
	}
}

func TestHandleKeyFindPendingIgnoresInsertMode(t *testing.T) {
	model := newTestModelWithDoc("pilot")
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(false, false)

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}}); cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("unexpected message from find operator: %T", msg)
		}
	}
	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}); cmd != nil {
		if msg := cmd(); msg != nil {
			t.Fatalf("unexpected message when targeting find motion: %T", msg)
		}
	}
	if model.editor.awaitingFindTarget() {
		t.Fatal("expected find target to clear after applying target")
	}
	if model.editorInsertMode {
		t.Fatal("expected find target not to enter insert mode")
	}
	pos := model.editor.caretPosition()
	if pos.Column != 1 {
		t.Fatalf("expected cursor on target character i (column 1), got %d", pos.Column)
	}
}
