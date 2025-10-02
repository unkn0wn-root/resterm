package ui

import (
	"testing"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/parser"
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
	model.moveCursorToLine(2)

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected enter key to trigger command in view mode")
	}
}

func TestHandleKeyEnterInInsertModeDoesNotSend(t *testing.T) {
	model := newTestModelWithDoc(sampleRequestDoc)
	model.setFocus(focusEditor)
	model.setInsertMode(true, false)
	model.moveCursorToLine(2)

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected enter key to be ignored in insert mode")
	}
}

func TestHandleKeyIgnoredWhileErrorModalVisible(t *testing.T) {
	model := newTestModelWithDoc(sampleRequestDoc)
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	_ = model.applyLayout()
	initialEditor := model.editor.Width()
	if initialEditor <= 0 {
		t.Fatalf("expected initial editor width to be positive, got %d", initialEditor)
	}

	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if model.editor.Width() >= initialEditor {
		t.Fatalf("expected gh to shrink editor width, initial %d new %d", initialEditor, model.editor.Width())
	}
}

func TestHandleKeyGlExpandsEditor(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	model.setFocus(focusEditor)
	_ = model.applyLayout()
	initialEditor := model.editor.Width()
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if model.editor.Width() <= initialEditor {
		t.Fatalf("expected gl to expand editor width, initial %d new %d", initialEditor, model.editor.Width())
	}
}

func TestHandleKeyGhCanRepeatWithoutPrefix(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	model.setFocus(focusEditor)
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
		t.Fatalf("expected repeated h to continue shrinking editor, previous %d new %d", first, second)
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
	model.setFocus(focusEditor)
	model.setInsertMode(true, false)
	_ = model.applyLayout()
	initialEditor := model.editor.Width()

	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if model.editor.Width() != initialEditor {
		t.Fatalf("expected gh chord to be ignored in insert mode, initial %d new %d", initialEditor, model.editor.Width())
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

func TestHandleKeyGjAdjustsSidebar(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	model.setFocus(focusFile)
	_ = model.applyLayout()
	initialFiles := model.sidebarFilesHeight
	initialIndex := model.fileList.Index()
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if model.sidebarFilesHeight >= initialFiles {
		t.Fatalf("expected gj to reduce files pane height, initial %d new %d", initialFiles, model.sidebarFilesHeight)
	}
	if model.fileList.Index() != initialIndex {
		t.Fatalf("expected gj chord not to move file selection, initial %d new %d", initialIndex, model.fileList.Index())
	}
}

func TestHandleKeyGjCanRepeatWithoutPrefix(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	model.setFocus(focusFile)
	_ = model.applyLayout()
	initialIndex := model.fileList.Index()
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	first := model.sidebarFilesHeight
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	second := model.sidebarFilesHeight
	if second >= first {
		t.Fatalf("expected repeated j to continue shrinking files pane, first %d second %d", first, second)
	}
	if !model.repeatChordActive {
		t.Fatalf("expected chord repeat to remain active after repeated sidebar adjustment")
	}
	if model.fileList.Index() != initialIndex {
		t.Fatalf("expected repeated gj not to move file selection, initial %d new %d", initialIndex, model.fileList.Index())
	}
}

func TestHandleKeyGkAdjustsSidebar(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 160
	model.height = 50
	model.ready = true
	model.setFocus(focusRequests)
	_ = model.applyLayout()
	initialFiles := model.sidebarFilesHeight
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if model.sidebarFilesHeight <= initialFiles {
		t.Fatalf("expected gk to increase files pane height, initial %d new %d", initialFiles, model.sidebarFilesHeight)
	}
}

func TestChordFallbackMaintainsEditorMotions(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 120
	model.height = 40
	model.ready = true
	model.setFocus(focusEditor)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

	sendKeys(t, model, "d", "e")

	if got := model.editor.Value(); got != " another" {
		t.Fatalf("expected de to remove to end of word, got %q", got)
	}
}

func TestDeleteOperatorDb(t *testing.T) {
	model := newTestModelWithDoc("alpha beta")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

	editorPtr := &model.editor
	editorPtr.moveCursorTo(0, len("alpha "))

	sendKeys(t, model, "d", "b")

	if got := model.editor.Value(); got != "beta" {
		t.Fatalf("expected db to remove previous word, got %q", got)
	}
}

func TestDeleteOperatorDollar(t *testing.T) {
	model := newTestModelWithDoc("alpha beta")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

	sendKeys(t, model, "d", "d")

	if got := model.editor.Value(); got != "second\nthird" {
		t.Fatalf("expected dd to remove current line, got %q", got)
	}
}

func TestDeleteOperatorDj(t *testing.T) {
	model := newTestModelWithDoc("first\nsecond\nthird")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

	sendKeys(t, model, "d", "j")

	if got := model.editor.Value(); got != "third" {
		t.Fatalf("expected dj to remove two lines, got %q", got)
	}
}

func TestDeleteOperatorDgg(t *testing.T) {
	model := newTestModelWithDoc("first\nsecond\nthird")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

	sendKeys(t, model, "d", "G")

	if got := model.editor.Value(); got != "" {
		t.Fatalf("expected dG to remove from cursor to end, got %q", got)
	}
}

func TestDeleteOperatorDf(t *testing.T) {
	model := newTestModelWithDoc("abcdeffg")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

	sendKeys(t, model, "d", "f", "f")

	if got := model.editor.Value(); got != "fg" {
		t.Fatalf("expected dff to delete through target char, got %q", got)
	}
}

func TestDeleteOperatorDt(t *testing.T) {
	model := newTestModelWithDoc("abcdeffg")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

	sendKeys(t, model, "d", "t", "f")

	if got := model.editor.Value(); got != "ffg" {
		t.Fatalf("expected dtf to delete until before target, got %q", got)
	}
}

func TestDeleteOperatorCancelEsc(t *testing.T) {
	model := newTestModelWithDoc("alpha")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

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
	if evt.status == nil || evt.status.text != "Deleted line" {
		t.Fatalf("expected delete line status, got %+v", evt.status)
	}
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
		if evt.status == nil || evt.status.text != "Deleted line" {
			t.Fatalf("expected delete line status, got %+v", evt.status)
		}
	}
	if got := model.editor.Value(); got != "alpha" {
		t.Fatalf("expected trailing line removed without blank, got %q", got)
	}
}

func TestHandleKeyDdDeletesTrailingBlankLine(t *testing.T) {
	model := newTestModelWithDoc("alpha\n")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
		if evt.status == nil || evt.status.text != "Deleted line" {
			t.Fatalf("expected delete line status, got %+v", evt.status)
		}
	}
	if got := model.editor.Value(); got != "alpha" {
		t.Fatalf("expected trailing blank line removed, got %q", got)
	}
}

func TestHandleKeyDDeletesToLineEnd(t *testing.T) {
	model := newTestModelWithDoc("alpha beta\nsecond")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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

func TestHandleKeyCChangesLineAndEntersInsert(t *testing.T) {
	model := newTestModelWithDoc("alpha\nbeta")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
	model.moveCursorToLine(2)

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if cmd == nil {
		t.Fatalf("expected command for c")
	}
	_ = cmd()
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
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
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

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

func TestHandleKeyFindPendingIgnoresInsertMode(t *testing.T) {
	model := newTestModelWithDoc("pilot")
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)

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
