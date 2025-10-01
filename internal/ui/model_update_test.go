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
