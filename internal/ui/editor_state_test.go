package ui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
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

func statusFromCmd(t *testing.T, cmd tea.Cmd) *statusMsg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	evt, ok := msg.(editorEvent)
	if !ok {
		t.Fatalf("expected editorEvent, got %T", msg)
	}
	return evt.status
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

func TestRequestEditorApplySearchLiteral(t *testing.T) {
	content := "one\ntwo\nthree two"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor, cmd := editor.ApplySearch("two", false)
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status message from search")
	}
	if status.level != statusInfo {
		t.Fatalf("expected info status, got %v", status.level)
	}
	if want := "Match 1/2 for \"two\""; status.text != want {
		t.Fatalf("expected status %q, got %q", want, status.text)
	}

	if editor.search.query != "two" {
		t.Fatalf("search query not stored: %q", editor.search.query)
	}
	if editor.search.index != 0 {
		t.Fatalf("expected search index 0, got %d", editor.search.index)
	}
	pos := editor.caretPosition()
	if pos.Line != 1 || pos.Column != 0 {
		t.Fatalf("expected caret at line 1 column 0, got line %d column %d", pos.Line, pos.Column)
	}
}

func TestRequestEditorApplySearchRegexInvalid(t *testing.T) {
	editor := newTestEditor("alpha")
	editor, cmd := editor.ApplySearch("[", true)
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status message for invalid regex")
	}
	if status.level != statusError {
		t.Fatalf("expected error status, got %v", status.level)
	}
	if !strings.Contains(status.text, "Invalid regex") {
		t.Fatalf("unexpected status text %q", status.text)
	}
	if editor.search.active {
		t.Fatal("search should be inactive after invalid regex")
	}
	if len(editor.search.matches) != 0 {
		t.Fatalf("expected no matches, got %d", len(editor.search.matches))
	}
}

func TestRequestEditorNextSearchMatchWrap(t *testing.T) {
	content := "foo bar\nfoo baz\nfoo"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor, cmd := editor.ApplySearch("foo", false)
	if status := statusFromCmd(t, cmd); status == nil {
		t.Fatal("expected initial search status")
	}

	editor, cmd = editor.NextSearchMatch()
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after next search")
	}
	if status.level != statusInfo {
		t.Fatalf("expected info level, got %v", status.level)
	}
	if !strings.Contains(status.text, "Match 2/3") {
		t.Fatalf("expected status to show second match, got %q", status.text)
	}
	if editor.search.index != 1 {
		t.Fatalf("expected search index 1, got %d", editor.search.index)
	}

	editor, cmd = editor.NextSearchMatch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after wrap")
	}
	if !strings.Contains(status.text, "Match 3/3") {
		t.Fatalf("expected status to show third match, got %q", status.text)
	}
	if strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("did not expect wrap notice on third match, got %q", status.text)
	}
	if editor.search.index != 2 {
		t.Fatalf("expected search index 2, got %d", editor.search.index)
	}

	editor, cmd = editor.NextSearchMatch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after cycling to first match")
	}
	if !strings.Contains(status.text, "Match 1/3") {
		t.Fatalf("expected status to reset to first match, got %q", status.text)
	}
	if !strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("expected wrap notice when cycling back, got %q", status.text)
	}
	if editor.search.index != 0 {
		t.Fatalf("expected search index reset to 0, got %d", editor.search.index)
	}
}
