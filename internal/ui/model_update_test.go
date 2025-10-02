package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/parser"
)

const sampleRequestDoc = "### example\n# @name getExample\nGET https://example.com\n"

func newTestModelWithDoc(content string) *Model {
	model := New(Config{})
	model.editor.SetValue(content)
	model.doc = parser.Parse(model.currentFile, []byte(content))
	return &model
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
}

func TestHandleKeyDWithoutSelectionShowsStatus(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 120
	model.height = 40
	model.ready = true
	model.setFocus(focusEditor)
	model.setInsertMode(false, false)
	model.editor.SetValue("alpha")

	cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatalf("expected command when deleting without selection")
	}
	msg := cmd()
	evt, ok := msg.(editorEvent)
	if !ok {
		t.Fatalf("expected editorEvent, got %T", msg)
	}
	if evt.dirty {
		t.Fatalf("expected dirty to remain false when nothing deleted")
	}
	if evt.status == nil || evt.status.text != "No selection to delete" {
		t.Fatalf("expected warning status, got %+v", evt.status)
	}
	if got := model.editor.Value(); got != "alpha" {
		t.Fatalf("expected content to remain unchanged, got %q", got)
	}
}
