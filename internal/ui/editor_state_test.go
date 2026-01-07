package ui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/ui/hint"
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

func editorEventFromCmd(t *testing.T, cmd tea.Cmd) editorEvent {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected command to emit editorEvent")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("expected editorEvent message, got nil")
	}
	evt, ok := msg.(editorEvent)
	if !ok {
		t.Fatalf("expected editorEvent, got %T", msg)
	}
	return evt
}

func collectHintLabels(options []hint.Hint) map[string]bool {
	labels := make(map[string]bool, len(options))
	for _, option := range options {
		labels[option.Label] = true
	}
	return labels
}

const clipboardFallbackStatus = "Clipboard unavailable; saved in editor register"

func expectStatusWithClipboardFallback(t *testing.T, status *statusMsg, want string) {
	t.Helper()
	if status == nil {
		t.Fatalf("expected status %q, got nil", want)
	}
	if status.text == want || status.text == clipboardFallbackStatus {
		return
	}
	t.Fatalf("expected status %q (or clipboard fallback), got %q", want, status.text)
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
		t.Fatalf(
			"expected cursor at line 0, column 2; got line %d, column %d",
			pos.Line,
			pos.Column,
		)
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
		t.Fatalf(
			"expected cursor at last line first non-blank (2,3); got (%d,%d)",
			pos.Line,
			pos.Column,
		)
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
		t.Fatalf(
			"expected caret to move to first non-blank of line 0 (3); got (%d,%d)",
			pos.Line,
			pos.Column,
		)
	}

	editorPtr.moveCursorTo(1, 0)
	editor = applyMotion(t, editor, "^")
	pos = editor.caretPosition()
	if pos.Line != 1 || pos.Column != 4 {
		t.Fatalf(
			"expected caret to align after tab on line 1 (column 4); got (%d,%d)",
			pos.Line,
			pos.Column,
		)
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

	editor = applyMotion(t, editor, "e")
	pos = editor.caretPosition()
	if pos.Line != 0 {
		t.Fatalf("expected to remain on first line; got line %d", pos.Line)
	}
	if want := utf8.RuneCountInString("word another") - 1; pos.Column != want {
		t.Fatalf("expected end of second word at column %d; got %d", want, pos.Column)
	}

	editor = applyMotion(t, editor, "e")
	pos = editor.caretPosition()
	if pos.Line != 1 || pos.Column != 3 {
		t.Fatalf("expected e to advance to end of next line; got (%d,%d)", pos.Line, pos.Column)
	}
}

func TestRequestEditorMotionWordEnds(t *testing.T) {
	content := "foo,bar baz"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor = applyMotion(t, editor, "e")
	pos := editor.caretPosition()
	if pos.Line != 0 || pos.Column != 2 {
		t.Fatalf("expected end of foo at column 2; got (%d,%d)", pos.Line, pos.Column)
	}

	editor = applyMotion(t, editor, "e")
	pos = editor.caretPosition()
	if pos.Line != 0 || pos.Column != 3 {
		t.Fatalf("expected end of comma word at column 3; got (%d,%d)", pos.Line, pos.Column)
	}

	editor = applyMotion(t, editor, "e")
	pos = editor.caretPosition()
	if pos.Line != 0 || pos.Column != 6 {
		t.Fatalf("expected end of bar at column 6; got (%d,%d)", pos.Line, pos.Column)
	}

	editorPtr.moveCursorTo(0, 0)
	editor = applyMotion(t, editor, "E")
	pos = editor.caretPosition()
	if pos.Line != 0 || pos.Column != 6 {
		t.Fatalf("expected end of foo,bar at column 6; got (%d,%d)", pos.Line, pos.Column)
	}
}

func TestRequestEditorMotionWordStarts(t *testing.T) {
	content := "foo, bar baz"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor = applyMotion(t, editor, "w")
	pos := editor.caretPosition()
	if pos.Line != 0 || pos.Column != 3 {
		t.Fatalf("expected comma start at column 3; got (%d,%d)", pos.Line, pos.Column)
	}

	editor = applyMotion(t, editor, "w")
	pos = editor.caretPosition()
	if pos.Line != 0 || pos.Column != 5 {
		t.Fatalf("expected bar start at column 5; got (%d,%d)", pos.Line, pos.Column)
	}

	editor = applyMotion(t, editor, "b")
	pos = editor.caretPosition()
	if pos.Line != 0 || pos.Column != 3 {
		t.Fatalf("expected comma start at column 3; got (%d,%d)", pos.Line, pos.Column)
	}

	editorPtr.moveCursorTo(0, 0)
	editor = applyMotion(t, editor, "W")
	pos = editor.caretPosition()
	if pos.Line != 0 || pos.Column != 5 {
		t.Fatalf("expected bar start at column 5; got (%d,%d)", pos.Line, pos.Column)
	}

	editor = applyMotion(t, editor, "B")
	pos = editor.caretPosition()
	if pos.Line != 0 || pos.Column != 0 {
		t.Fatalf("expected foo start at column 0; got (%d,%d)", pos.Line, pos.Column)
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

func TestRequestEditorMotionsDisabled(t *testing.T) {
	editor := newTestEditor("")
	editorPtr := &editor
	editorPtr.SetMotionsEnabled(false)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}
	editor, _ = editor.Update(msg)
	if got := editor.Value(); got != "e" {
		t.Fatalf("expected rune to insert when motions disabled; got %q", got)
	}

	_, _, handled := editor.HandleMotion("G")
	if handled {
		t.Fatal("expected motion handler to ignore commands when disabled")
	}
}

func TestRequestEditorDeleteSelectionRemovesText(t *testing.T) {
	editor := newTestEditor("alpha")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)
	start := editor.caretPosition()
	editorPtr.startSelection(start, selectionManual)
	editorPtr.selection.Update(cursorPosition{Line: 0, Column: 5, Offset: 5})
	editorPtr.applySelectionHighlight()

	updated, cmd := editor.DeleteSelection()
	evt := editorEventFromCmd(t, cmd)
	if !evt.dirty {
		t.Fatalf("expected delete selection to mark editor dirty")
	}
	expectStatusWithClipboardFallback(t, evt.status, "Selection deleted")
	if got := updated.Value(); got != "" {
		t.Fatalf("expected selection to be removed, got %q", got)
	}
	if updated.hasSelection() {
		t.Fatal("expected selection to be cleared")
	}
}

func TestRequestEditorDeleteSelectionRequiresSelection(t *testing.T) {
	editor := newTestEditor("alpha")
	updated, cmd := editor.DeleteSelection()
	evt := editorEventFromCmd(t, cmd)
	if evt.dirty {
		t.Fatalf("expected no dirty flag when nothing deleted")
	}
	if evt.status == nil || evt.status.text != "No selection to delete" {
		t.Fatalf("expected warning about missing selection, got %+v", evt.status)
	}
	if got := updated.Value(); got != "alpha" {
		t.Fatalf("expected content to remain unchanged, got %q", got)
	}
}

func TestRequestEditorVisualYankIncludesCaretRune(t *testing.T) {
	editor := newTestEditor("Align")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor, _ = editor.ToggleVisual()
	editor, cmd := editor.YankSelection()
	_ = editorEventFromCmd(t, cmd)

	if got := editor.registerText; got != "A" {
		t.Fatalf("expected visual yank to capture caret rune, got %q", got)
	}
	if editor.hasSelection() {
		t.Fatalf("expected selection to clear after yank")
	}
}

func TestRequestEditorUndoRestoresDeletion(t *testing.T) {
	editor := newTestEditor("alpha")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)
	start := editor.caretPosition()
	editorPtr.startSelection(start, selectionManual)
	editorPtr.selection.Update(cursorPosition{Line: 0, Column: 5, Offset: 5})
	editorPtr.applySelectionHighlight()

	editor, _ = editor.DeleteSelection()
	editor, cmd := editor.UndoLastChange()
	evt := editorEventFromCmd(t, cmd)
	if !evt.dirty {
		t.Fatalf("expected undo to mark editor dirty")
	}
	if evt.status == nil || evt.status.text != "Undid last change" {
		t.Fatalf("expected undo status message, got %+v", evt.status)
	}
	if got := editor.Value(); got != "alpha" {
		t.Fatalf("expected undo to restore content, got %q", got)
	}
	if !editor.selection.IsActive() {
		t.Fatalf("expected selection to be restored")
	}
}

func TestRequestEditorUndoWhenEmpty(t *testing.T) {
	editor := newTestEditor("alpha")
	editor, cmd := editor.UndoLastChange()
	evt := editorEventFromCmd(t, cmd)
	if evt.dirty {
		t.Fatalf("expected no dirty flag when nothing to undo")
	}
	if evt.status == nil || evt.status.text != "Nothing to undo" {
		t.Fatalf("expected no-undo status message, got %+v", evt.status)
	}
	if got := editor.Value(); got != "alpha" {
		t.Fatalf("expected content unchanged, got %q", got)
	}
}

func TestDeleteSelectionPreservesViewStart(t *testing.T) {
	var lines []string
	for i := 0; i < 120; i++ {
		lines = append(lines, fmt.Sprintf("line %03d", i))
	}
	content := strings.Join(lines, "\n")
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.SetHeight(8)
	editorPtr.SetViewStart(40)
	if got := editor.ViewStart(); got != 40 {
		t.Fatalf("expected view start 40, got %d", got)
	}

	editorPtr.moveCursorTo(60, 0)
	start := editor.caretPosition()
	editorPtr.startSelection(start, selectionManual)
	offset := editor.offsetForPosition(61, 0)
	editorPtr.selection.Update(cursorPosition{Line: 61, Column: 0, Offset: offset})
	editorPtr.applySelectionHighlight()

	_, _ = editor.DeleteSelection()
	if got := editor.ViewStart(); got != 40 {
		t.Fatalf("expected view start to remain 40, got %d", got)
	}
}

func TestUndoRestoresViewStart(t *testing.T) {
	var lines []string
	for i := 0; i < 120; i++ {
		lines = append(lines, fmt.Sprintf("line %03d", i))
	}
	content := strings.Join(lines, "\n")
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.SetHeight(8)
	editorPtr.SetViewStart(30)

	editorPtr.moveCursorTo(45, 0)
	start := editor.caretPosition()
	editorPtr.startSelection(start, selectionManual)
	editorPtr.selection.Update(
		cursorPosition{Line: 46, Column: 0, Offset: editor.offsetForPosition(46, 0)},
	)
	editorPtr.applySelectionHighlight()

	editor, _ = editor.DeleteSelection()
	if got := editor.ViewStart(); got != 30 {
		t.Fatalf("expected delete to preserve view start, got %d", got)
	}
	editor, _ = editor.UndoLastChange()
	if got := editor.ViewStart(); got != 30 {
		t.Fatalf("expected undo to restore view start 30, got %d", got)
	}
}

func TestRedoRestoresUndoneChange(t *testing.T) {
	editor := newTestEditor("abc")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 1)

	editor, _ = editor.DeleteCharAtCursor()
	if got := editor.Value(); got != "ac" {
		t.Fatalf("expected middle char removed, got %q", got)
	}
	editor, _ = editor.UndoLastChange()
	if got := editor.Value(); got != "abc" {
		t.Fatalf("expected undo to restore text, got %q", got)
	}
	editor, cmd := editor.RedoLastChange()
	evt := editorEventFromCmd(t, cmd)
	if evt.status == nil || evt.status.text != "Redid last change" {
		t.Fatalf("expected redo status, got %+v", evt.status)
	}
	if got := editor.Value(); got != "ac" {
		t.Fatalf("expected redo to reapply deletion, got %q", got)
	}
}

func TestDeleteCurrentLineRemovesLine(t *testing.T) {
	editor := newTestEditor("alpha\nbeta\ncharlie")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	before := editor.Value()
	editor, cmd := editor.DeleteCurrentLine()
	evt := editorEventFromCmd(t, cmd)
	expectStatusWithClipboardFallback(t, evt.status, "Deleted line")
	if got := editor.Value(); got == before || got != "beta\ncharlie" {
		t.Fatalf("expected first line removed, got %q", got)
	}
}

func TestDeleteToLineEndRemovesTail(t *testing.T) {
	editor := newTestEditor("alpha beta\nsecond")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 6)

	editor, cmd := editor.DeleteToLineEnd()
	evt := editorEventFromCmd(t, cmd)
	expectStatusWithClipboardFallback(t, evt.status, "Deleted to end of line")
	if got := editor.Value(); got != "alpha \nsecond" {
		t.Fatalf("expected tail removed, got %q", got)
	}
}

func TestDeleteCharAtCursorRemovesRune(t *testing.T) {
	editor := newTestEditor("xyz")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 1)

	editor, cmd := editor.DeleteCharAtCursor()
	evt := editorEventFromCmd(t, cmd)
	expectStatusWithClipboardFallback(t, evt.status, "Deleted character")
	if got := editor.Value(); got != "xz" {
		t.Fatalf("expected middle character removed, got %q", got)
	}
}

func TestDeleteCharAtCursorRemovesSelection(t *testing.T) {
	editor := newTestEditor("alpha beta")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)
	start := editor.caretPosition()
	editorPtr.startSelection(start, selectionManual)
	editorPtr.selection.Update(cursorPosition{Line: 0, Column: 5, Offset: 5})
	editorPtr.applySelectionHighlight()

	editor, cmd := editor.DeleteCharAtCursor()
	evt := editorEventFromCmd(t, cmd)
	if evt.status == nil {
		t.Fatalf("expected selection deletion status, got %+v", evt.status)
	}
	if evt.status.text != "Deleted selection" &&
		!strings.Contains(evt.status.text, "Clipboard unavailable") {
		t.Fatalf("unexpected selection deletion status, got %+v", evt.status)
	}
	if editor.hasSelection() {
		t.Fatalf("expected selection to clear after deletion")
	}
	if got := editor.Value(); got != " beta" {
		t.Fatalf("expected selected text removed, got %q", got)
	}
}

func TestChangeCurrentLineClearsContent(t *testing.T) {
	editor := newTestEditor("alpha\nbeta")
	editorPtr := &editor
	editorPtr.moveCursorTo(1, 0)

	editor, cmd := editor.ChangeCurrentLine()
	evt := editorEventFromCmd(t, cmd)
	expectStatusWithClipboardFallback(t, evt.status, "Changed line")
	if got := editor.Value(); got != "alpha\n" {
		t.Fatalf("expected second line cleared, got %q", got)
	}
}

func TestPasteClipboardInsertsAfterCursor(t *testing.T) {
	if err := clipboard.WriteAll("ZZ"); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	editor := newTestEditor("abc")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 1)

	editor, cmd := editor.PasteClipboard(true)
	evt := editorEventFromCmd(t, cmd)
	if evt.status == nil {
		t.Fatal("expected paste status, got nil")
	}
	if evt.status.text != "Pasted" {
		t.Fatalf("expected paste status, got %+v", evt.status)
	}
	if got := editor.Value(); got != "abZZc" {
		t.Fatalf("expected clipboard pasted after character, got %q", got)
	}
}

func TestPasteClipboardInsertsBeforeCursorMovesCaretToEnd(t *testing.T) {
	if err := clipboard.WriteAll("ZZ"); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	editor := newTestEditor("abc")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 1)

	editor, cmd := editor.PasteClipboard(false)
	evt := editorEventFromCmd(t, cmd)
	if evt.status == nil {
		t.Fatal("expected paste status, got nil")
	}
	if got := editor.Value(); got != "aZZbc" {
		t.Fatalf("expected clipboard pasted before character, got %q", got)
	}
	pos := editor.caretPosition()
	if pos.Line != 0 || pos.Column != 2 {
		t.Fatalf("expected cursor at end of paste, got (%d,%d)", pos.Line, pos.Column)
	}
}

func TestPasteClipboardLinewisePreservesFollowingLine(t *testing.T) {
	if err := clipboard.WriteAll(""); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	editor := newTestEditor("first\nsecond\nthird")
	editor.registerText = "alpha\n"
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor, cmd := editor.PasteClipboard(true)
	evt := editorEventFromCmd(t, cmd)
	if evt.status == nil {
		t.Fatal("expected paste status, got nil")
	}
	if evt.status.text != "Pasted from editor register" {
		t.Fatalf("expected register paste status, got %+v", evt.status)
	}
	if got := editor.Value(); got != "first\nalpha\nsecond\nthird" {
		t.Fatalf("expected linewise paste to preserve following line, got %q", got)
	}
	pos := editor.caretPosition()
	if pos.Line != 1 {
		t.Fatalf("expected cursor to land on inserted line, got line %d", pos.Line)
	}
	if pos.Column != 0 {
		t.Fatalf("expected cursor at column 0 of inserted line, got column %d", pos.Column)
	}
}

func TestPasteClipboardLinewiseBeforeInsertsAboveLine(t *testing.T) {
	if err := clipboard.WriteAll(""); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	editor := newTestEditor("first\nsecond\nthird")
	editor.registerText = "alpha\n"
	editorPtr := &editor
	editorPtr.moveCursorTo(1, 3)

	editor, cmd := editor.PasteClipboard(false)
	evt := editorEventFromCmd(t, cmd)
	if evt.status == nil {
		t.Fatal("expected paste status, got nil")
	}
	if got := editor.Value(); got != "first\nalpha\nsecond\nthird" {
		t.Fatalf("expected linewise paste above cursor line, got %q", got)
	}
	pos := editor.caretPosition()
	if pos.Line != 1 || pos.Column != 0 {
		t.Fatalf("expected cursor on inserted line, got line %d col %d", pos.Line, pos.Column)
	}
}

func TestPasteClipboardLinewiseRepeatedKeepsOrder(t *testing.T) {
	editor := newTestEditor("second\nthird")
	editorPtr := &editor

	// Prime the register with a linewise yank (simulating `dd`).
	editor.registerText = "first\n"

	editorPtr.moveCursorTo(0, 0)
	editor, _ = editor.PasteClipboard(true)
	if got := editor.Value(); got != "second\nfirst\nthird" {
		t.Fatalf("unexpected value after first paste: %q", got)
	}
	pos := editor.caretPosition()
	if pos.Line != 1 || pos.Column != 0 {
		t.Fatalf(
			"expected cursor on inserted line after first paste, got line %d col %d",
			pos.Line,
			pos.Column,
		)
	}

	editor, _ = editor.PasteClipboard(true)
	if got := editor.Value(); got != "second\nfirst\nfirst\nthird" {
		t.Fatalf("unexpected value after second paste: %q", got)
	}
	pos = editor.caretPosition()
	if pos.Line != 2 || pos.Column != 0 {
		t.Fatalf(
			"expected cursor on latest inserted line, got line %d col %d",
			pos.Line,
			pos.Column,
		)
	}
}

func TestPasteClipboardLinewiseCRLF(t *testing.T) {
	if err := clipboard.WriteAll(""); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	editor := newTestEditor("first\nsecond")
	editor.registerText = "alpha\r\n"
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor, _ = editor.PasteClipboard(true)
	if got := editor.Value(); got != "first\nalpha\nsecond" {
		t.Fatalf("unexpected value after CRLF paste: %q", got)
	}
}

func TestPasteClipboardCROnly(t *testing.T) {
	if err := clipboard.WriteAll(""); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	editor := newTestEditor("first\nsecond")
	editor.registerText = "alpha\r"
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor, _ = editor.PasteClipboard(true)
	if got := editor.Value(); got != "first\nalpha\nsecond" {
		t.Fatalf("unexpected value after CR paste: %q", got)
	}
}

func TestHandleMotionFindForward(t *testing.T) {
	editor := newTestEditor("alphabet")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	updated, _, handled := editor.HandleMotion("f")
	if !handled {
		t.Fatalf("expected f to be handled")
	}
	updated, cmd, handled := updated.HandleMotion("b")
	if !handled {
		t.Fatalf("expected second motion key to be handled")
	}
	if cmd != nil {
		if evt := cmd(); evt != nil {
			if e, ok := evt.(editorEvent); ok && e.status != nil {
				t.Fatalf("did not expect status on successful find, got %+v", e.status)
			}
		}
	}
	pos := updated.caretPosition()
	if pos.Column != 5 {
		t.Fatalf("expected cursor at column 5 (b), got %d", pos.Column)
	}
}

func TestHandleMotionFindBackwardTill(t *testing.T) {
	editor := newTestEditor("alphabet")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 6)

	updated, _, handled := editor.HandleMotion("T")
	if !handled {
		t.Fatalf("expected T to be handled")
	}
	updated, cmd, handled := updated.HandleMotion("l")
	if !handled {
		t.Fatalf("expected target key to be handled")
	}
	if cmd != nil {
		if evt := cmd(); evt != nil {
			if e, ok := evt.(editorEvent); ok && e.status != nil {
				t.Fatalf("did not expect warning on successful find, got %+v", e.status)
			}
		}
	}
	pos := updated.caretPosition()
	if pos.Column != 2 {
		t.Fatalf("expected cursor just after found char (column 2), got %d", pos.Column)
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

func TestRequestEditorRegexSearchMatches(t *testing.T) {
	content := "foo\nfao\nbar"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor, cmd := editor.ApplySearch("f.o", true)
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status from regex search")
	}
	if status.level != statusInfo {
		t.Fatalf("expected info status, got %v", status.level)
	}
	if !strings.Contains(status.text, "Match 1/2") {
		t.Fatalf("expected first regex match status, got %q", status.text)
	}
	if !editor.search.isRegex {
		t.Fatal("expected regex flag to remain set")
	}

	editor, cmd = editor.NextSearchMatch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after advancing regex search")
	}
	if !strings.Contains(status.text, "Match 2/2") {
		t.Fatalf("expected second regex match status, got %q", status.text)
	}
	if editor.search.index != 1 {
		t.Fatalf("expected search index 1 after advancing, got %d", editor.search.index)
	}

	editor, cmd = editor.PrevSearchMatch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after retreating regex search")
	}
	if !strings.Contains(status.text, "Match 1/2") {
		t.Fatalf("expected to return to first regex match, got %q", status.text)
	}
	if editor.search.index != 0 {
		t.Fatalf("expected search index reset to 0, got %d", editor.search.index)
	}
}

func TestRequestEditorPrevSearchMatchWrap(t *testing.T) {
	content := "foo bar\nfoo baz\nfoo"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor, cmd := editor.ApplySearch("foo", false)
	if status := statusFromCmd(t, cmd); status == nil {
		t.Fatal("expected initial search status")
	}

	editor, cmd = editor.PrevSearchMatch()
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after previous search")
	}
	if status.level != statusInfo {
		t.Fatalf("expected info level, got %v", status.level)
	}
	if !strings.Contains(status.text, "Match 3/3") {
		t.Fatalf("expected status to show third match, got %q", status.text)
	}
	if !strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("expected wrap notice when cycling back, got %q", status.text)
	}
	if editor.search.index != 2 {
		t.Fatalf("expected search index 2, got %d", editor.search.index)
	}

	editor, cmd = editor.PrevSearchMatch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after moving to previous match")
	}
	if !strings.Contains(status.text, "Match 2/3") {
		t.Fatalf("expected status to show second match, got %q", status.text)
	}
	if strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("did not expect wrap notice on second match, got %q", status.text)
	}
	if editor.search.index != 1 {
		t.Fatalf("expected search index 1, got %d", editor.search.index)
	}

	editor, cmd = editor.PrevSearchMatch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after cycling to first match")
	}
	if !strings.Contains(status.text, "Match 1/3") {
		t.Fatalf("expected status to show first match, got %q", status.text)
	}
	if strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("did not expect wrap notice on first match, got %q", status.text)
	}
	if editor.search.index != 0 {
		t.Fatalf("expected search index reset to 0, got %d", editor.search.index)
	}
}

func TestRequestEditorMetadataHintsSuggestAndAccept(t *testing.T) {
	editor := newTestEditor("# ")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 2)
	editorPtr.SetMetadataHintsEnabled(true)

	editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	if !editor.metadataHints.active {
		t.Fatal("expected metadata hints to activate after typing @")
	}
	if len(editor.metadataHints.filtered) == 0 {
		t.Fatal("expected metadata hint options")
	}

	editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	editor, cmd := editor.Update(tea.KeyMsg{Type: tea.KeyEnter})
	evt := editorEventFromCmd(t, cmd)
	if !evt.dirty {
		t.Fatal("expected autocomplete acceptance to mark editor dirty")
	}
	if got := editor.Value(); !strings.HasPrefix(got, "# @name ") {
		t.Fatalf("expected @name completion, got %q", got)
	}
	if editor.metadataHints.active {
		t.Fatal("expected metadata hints to close after acceptance")
	}
}

func TestRequestEditorMetadataHintsSuggestWsSubcommands(t *testing.T) {
	editor := newTestEditor("# ")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 2)
	editorPtr.SetMetadataHintsEnabled(true)

	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'@'}},
		{Type: tea.KeyRunes, Runes: []rune{'w'}},
		{Type: tea.KeyRunes, Runes: []rune{'s'}},
		{Type: tea.KeyRunes, Runes: []rune{' '}},
	}
	for _, key := range keys {
		var cmd tea.Cmd
		editor, cmd = editor.Update(key)
		if cmd != nil {
			cmd() // drain potential status updates
		}
	}

	if !editor.metadataHints.active {
		t.Fatal("expected metadata hints to activate for @ws directive")
	}
	if editor.metadataHints.ctx.Mode != hint.ModeSubcommand {
		t.Fatalf("expected subcommand hint mode, got %v", editor.metadataHints.ctx.Mode)
	}

	labels := collectHintLabels(editor.metadataHints.filtered)
	for _, label := range []string{"send", "send-json", "send-base64", "send-file", "ping", "pong", "wait", "close"} {
		if !labels[label] {
			t.Fatalf("expected ws subcommand %q in suggestions", label)
		}
	}

	editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if len(editor.metadataHints.filtered) == 0 {
		t.Fatal("expected filtered subcommand suggestions after typing prefix")
	}
	if got := editor.metadataHints.filtered[editor.metadataHints.selection].Label; got != "send" {
		t.Fatalf("expected first suggestion to be send, got %q", got)
	}

	editor, cmd := editor.Update(tea.KeyMsg{Type: tea.KeyEnter})
	evt := editorEventFromCmd(t, cmd)
	if !evt.dirty {
		t.Fatal("expected accepting subcommand hint to mark editor dirty")
	}
	if got := editor.Value(); !strings.HasPrefix(got, "# @ws send ") {
		t.Fatalf("expected editor content to include ws subcommand, got %q", got)
	}
	if editor.metadataHints.active {
		t.Fatal("expected metadata hints to close after subcommand acceptance")
	}
}

func TestRequestEditorMetadataHintsTracePlaceholder(t *testing.T) {
	editor := newTestEditor("# ")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 2)
	editorPtr.SetMetadataHintsEnabled(true)

	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'@'}},
		{Type: tea.KeyRunes, Runes: []rune{'t'}},
		{Type: tea.KeyRunes, Runes: []rune{'r'}},
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
		{Type: tea.KeyRunes, Runes: []rune{'c'}},
		{Type: tea.KeyRunes, Runes: []rune{'e'}},
		{Type: tea.KeyRunes, Runes: []rune{' '}},
		{Type: tea.KeyRunes, Runes: []rune{'d'}},
	}
	for _, key := range keys {
		var cmd tea.Cmd
		editor, cmd = editor.Update(key)
		if cmd != nil {
			cmd()
		}
	}

	if !editor.metadataHints.active {
		t.Fatal("expected metadata hints to remain active for trace subcommand")
	}
	if editor.metadataHints.ctx.Mode != hint.ModeSubcommand {
		t.Fatalf("expected subcommand mode, got %v", editor.metadataHints.ctx.Mode)
	}

	editor, cmd := editor.Update(tea.KeyMsg{Type: tea.KeyEnter})
	evt := editorEventFromCmd(t, cmd)
	if !evt.dirty {
		t.Fatal("expected accepting trace hint to mark editor dirty")
	}
	if editor.metadataHints.active {
		t.Fatal("expected metadata hints to close after acceptance")
	}

	wantContent := "# @trace dns<=50ms "
	if got := editor.Value(); got != wantContent {
		t.Fatalf(
			"unexpected editor content after trace insertion:\nwant %q\n got %q",
			wantContent,
			got,
		)
	}

	pos := editor.caretPosition()
	wantedOffset := utf8.RuneCountInString("# @trace dns<=")
	if pos.Offset != wantedOffset {
		t.Fatalf("expected caret offset %d, got %d", wantedOffset, pos.Offset)
	}
	if pos.Column != wantedOffset {
		t.Fatalf("expected caret column %d, got %d", wantedOffset, pos.Column)
	}
}

func TestRequestEditorMetadataHintsProfileMultipleParams(t *testing.T) {
	editor := newTestEditor("# ")
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 2)
	editorPtr.SetMetadataHintsEnabled(true)

	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'@'}},
		{Type: tea.KeyRunes, Runes: []rune{'p'}},
		{Type: tea.KeyRunes, Runes: []rune{'r'}},
		{Type: tea.KeyRunes, Runes: []rune{'o'}},
		{Type: tea.KeyRunes, Runes: []rune{'f'}},
		{Type: tea.KeyRunes, Runes: []rune{'i'}},
		{Type: tea.KeyRunes, Runes: []rune{'l'}},
		{Type: tea.KeyRunes, Runes: []rune{'e'}},
		{Type: tea.KeyRunes, Runes: []rune{' '}},
	}
	for _, key := range keys {
		var cmd tea.Cmd
		editor, cmd = editor.Update(key)
		if cmd != nil {
			cmd()
		}
	}

	if !editor.metadataHints.active {
		t.Fatal("expected metadata hints to activate for profile subcommands")
	}
	if editor.metadataHints.ctx.Mode != hint.ModeSubcommand {
		t.Fatalf("expected subcommand mode, got %v", editor.metadataHints.ctx.Mode)
	}

	editor, cmd := editor.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		cmd()
	}
	if editor.metadataHints.active {
		t.Fatal("expected metadata hints to close after first profile acceptance")
	}

	editorPtr.moveCursorTo(0, utf8.RuneCountInString(editor.Value()))
	editorPtr.clearSelection()

	editor, cmd = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if cmd != nil {
		cmd()
	}
	if !editor.metadataHints.active {
		t.Fatal("expected metadata hints to stay active for additional profile params")
	}
	labels := collectHintLabels(editor.metadataHints.filtered)
	if !labels["warmup="] {
		t.Fatalf("expected warmup suggestion, got %v", editor.metadataHints.filtered)
	}
}

func TestRequestEditorMetadataHintsIgnoreNonCommentContext(t *testing.T) {
	editor := newTestEditor("")
	editorPtr := &editor
	editorPtr.SetMetadataHintsEnabled(true)

	editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	if editor.metadataHints.active {
		t.Fatal("expected metadata hints to remain inactive outside comment context")
	}
}

func TestAnalyzeMetadataHintContextSupportsChainedProfileParams(t *testing.T) {
	query := []rune("profile count=1 warm")
	ctx, ok := hint.AnalyzeContext(query)
	if !ok {
		t.Fatal("expected analyzeMetadataHintContext to accept chained params")
	}
	if ctx.Mode != hint.ModeSubcommand {
		t.Fatalf("expected subcommand mode, got %v", ctx.Mode)
	}
	if ctx.BaseKey != "profile" {
		t.Fatalf("expected base key profile, got %q", ctx.BaseKey)
	}
	if ctx.Query != "warm" {
		t.Fatalf("expected query warm, got %q", ctx.Query)
	}
	wantStart := len([]rune("profile count=1 "))
	if ctx.TokenStart != wantStart {
		t.Fatalf("expected token start %d, got %d", wantStart, ctx.TokenStart)
	}

	trailing := []rune("profile count=1 ")
	ctx, ok = hint.AnalyzeContext(trailing)
	if !ok {
		t.Fatal("expected analyzeMetadataHintContext to accept trailing space after params")
	}
	if ctx.Query != "" {
		t.Fatalf("expected empty query for trailing space, got %q", ctx.Query)
	}
	if ctx.TokenStart != len(trailing) {
		t.Fatalf("expected token start at end of query, got %d", ctx.TokenStart)
	}
}

func TestRequestEditorExitSearchMode(t *testing.T) {
	content := "foo\nbar\nfoo"
	editor := newTestEditor(content)
	editorPtr := &editor
	editorPtr.moveCursorTo(0, 0)

	editor, cmd := editor.ApplySearch("foo", false)
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after applying search")
	}
	if !editor.SearchActive() {
		t.Fatal("expected search to be active after applying search")
	}

	exitCmd := editorPtr.ExitSearchMode()
	exitStatus := statusFromCmd(t, exitCmd)
	if exitStatus == nil {
		t.Fatal("expected status when exiting search mode")
	}
	if exitStatus.level != statusInfo {
		t.Fatalf("expected info status when exiting search, got %v", exitStatus.level)
	}
	if exitStatus.text != "Search cleared" {
		t.Fatalf("unexpected exit status %q", exitStatus.text)
	}
	if editor.SearchActive() {
		t.Fatal("expected search to be inactive after exit")
	}
	if editor.search.active {
		t.Fatal("internal search flag should be false after exit")
	}
	if editor.search.index != -1 {
		t.Fatalf("expected search index reset to -1, got %d", editor.search.index)
	}
	if len(editor.search.matches) != 0 {
		t.Fatalf("expected search matches cleared, got %d", len(editor.search.matches))
	}
	if _, ok := editor.currentSearchMatch(); ok {
		t.Fatal("did not expect current search match after exit")
	}

	editor, cmd = editor.NextSearchMatch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after resuming search")
	}
	if !editor.SearchActive() {
		t.Fatal("expected search to reactivate on next match")
	}
	if editor.search.index != 0 {
		t.Fatalf("expected search index 0 after resuming, got %d", editor.search.index)
	}
}
