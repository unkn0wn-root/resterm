package ui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

func newTestEditor(content string) requestEditor {
	editor := newRequestEditor()
	editorPtr := &editor
	editorPtr.SetWidth(80)
	editorPtr.SetHeight(10)
	editorPtr.SetValue(content)
	editor, _ = editor.Update(nil)
	editorPtr.Focus()
	editor, _ = editor.Update(nil)
	return editor
}

func applyMotion(t *testing.T, editor requestEditor, command string) requestEditor {
	t.Helper()
	updated, _, handled := editor.HandleMotion(command)
	if !handled {
		t.Fatalf("expected motion %q to be handled", command)
	}
	return updated
}

func TestRequestEditorMotionGG(t *testing.T) {
	content := "  first\nsecond\nthird"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(2, 1)
	initial := editor.caretPosition()
	if initial.Line != 2 {
		t.Fatalf("expected starting line 2, got %d", initial.Line)
	}

	editor, _, handled := editor.HandleMotion("g")
	if !handled {
		t.Fatal("expected initial g to be handled")
	}
	if editor.pendingMotion != "g" {
		t.Fatalf("expected pending motion to be g, got %q", editor.pendingMotion)
	}
	if moved := editor.caretPosition(); moved.Line != 2 || moved.Column != initial.Column {
		t.Fatalf("cursor moved after first g: %+v", moved)
	}

	editor, _, handled = editor.HandleMotion("g")
	if !handled {
		t.Fatal("expected second g to be handled")
	}
	pos := editor.caretPosition()
	if pos.Line != 0 || pos.Column != 2 {
		t.Fatalf("expected cursor at line 0, column 2; got line %d, column %d", pos.Line, pos.Column)
	}
	if editor.pendingMotion != "" {
		t.Fatalf("expected pending motion to be cleared, got %q", editor.pendingMotion)
	}
}

func TestRequestEditorMotionG(t *testing.T) {
	content := "one\n  two\n   three"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)
	editor = applyMotion(t, editor, "G")
	pos := editor.caretPosition()
	if pos.Line != 2 || pos.Column != 3 {
		t.Fatalf("expected cursor at last line first non-blank (2,3); got (%d,%d)", pos.Line, pos.Column)
	}
}

func TestRequestEditorMotionCaret(t *testing.T) {
	content := "   alpha\n\tbravo\n    "
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 5)
	editor = applyMotion(t, editor, "^")
	pos := editor.caretPosition()
	if pos.Line != 0 || pos.Column != 3 {
		t.Fatalf("expected caret to move to first non-blank of line 0 (3); got (%d,%d)", pos.Line, pos.Column)
	}

	editorPtr.moveCursorTo(1, 0)
	editor = applyMotion(t, editor, "^")
	pos = editor.caretPosition()
	if pos.Line != 1 || pos.Column != 4 {
		t.Fatalf("expected caret to align after tab on line 1 (column 4); got (%d,%d)", pos.Line, pos.Column)
	}

	editorPtr.moveCursorTo(2, 0)
	editor = applyMotion(t, editor, "^")
	pos = editor.caretPosition()
	if pos.Line != 2 || pos.Column != 0 {
		t.Fatalf("expected blank line to stay at column 0; got (%d,%d)", pos.Line, pos.Column)
	}
}

func TestRequestEditorMotionE(t *testing.T) {
	content := "word another\nlast"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)
	editor = applyMotion(t, editor, "e")
	pos := editor.caretPosition()
	if pos.Line != 0 || pos.Column != 3 {
		t.Fatalf("expected end of first word at column 3; got (%d,%d)", pos.Line, pos.Column)
	}

	editorPtr.moveCursorTo(0, 4)
	editor = applyMotion(t, editor, "e")
	pos = editor.caretPosition()
	if pos.Line != 0 {
		t.Fatalf("expected to remain on first line; got line %d", pos.Line)
	}
	if want := utf8.RuneCountInString("word another") - 1; pos.Column != want {
		t.Fatalf("expected end of second word at column %d; got %d", want, pos.Column)
	}

	final := utf8.RuneCountInString("word another")
	editorPtr.moveCursorTo(0, final)
	editor = applyMotion(t, editor, "e")
	pos = editor.caretPosition()
	if pos.Line != 1 || pos.Column != 3 {
		t.Fatalf("expected e to advance to end of next line; got (%d,%d)", pos.Line, pos.Column)
	}
}

func TestRequestEditorMotionPaging(t *testing.T) {
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	editor := newTestEditor(strings.Join(lines, "\n"))
	editorPtr := &editor
	editorPtr.SetHeight(4)
	editor, _ = editor.Update(nil)
	editorPtr.moveCursorTo(0, 0)

	editor = applyMotion(t, editor, "ctrl+f")
	if line := editor.Line(); line != 3 {
		t.Fatalf("expected ctrl+f to advance to line 3; got %d", line)
	}

	editor = applyMotion(t, editor, "ctrl+d")
	if line := editor.Line(); line != 5 {
		t.Fatalf("expected ctrl+d to advance half-page to line 5; got %d", line)
	}

	editor = applyMotion(t, editor, "ctrl+u")
	if line := editor.Line(); line != 3 {
		t.Fatalf("expected ctrl+u to move back to line 3; got %d", line)
	}

	editor = applyMotion(t, editor, "ctrl+b")
	if line := editor.Line(); line != 0 {
		t.Fatalf("expected ctrl+b to return to line 0; got %d", line)
	}
}
