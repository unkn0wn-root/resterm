package ui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/atotto/clipboard"
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
	if evt.status == nil || evt.status.text != "Selection deleted" {
		t.Fatalf("expected delete status message, got %+v", evt.status)
	}
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
	editorPtr.selection.Update(cursorPosition{Line: 46, Column: 0, Offset: editor.offsetForPosition(46, 0)})
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
	if evt.status == nil || evt.status.text != "Deleted line" {
		t.Fatalf("expected line deletion status, got %+v", evt.status)
	}
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
	if evt.status == nil || evt.status.text != "Deleted to end of line" {
		t.Fatalf("expected delete tail status, got %+v", evt.status)
	}
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
	if evt.status == nil || evt.status.text != "Deleted character" {
		t.Fatalf("expected char deletion status, got %+v", evt.status)
	}
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
	if evt.status.text != "Deleted selection" && !strings.Contains(evt.status.text, "Clipboard unavailable") {
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
	if evt.status == nil || evt.status.text != "Changed line" {
		t.Fatalf("expected change line status, got %+v", evt.status)
	}
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
		t.Fatalf("expected cursor on inserted line after first paste, got line %d col %d", pos.Line, pos.Column)
	}

	editor, _ = editor.PasteClipboard(true)
	if got := editor.Value(); got != "second\nfirst\nfirst\nthird" {
		t.Fatalf("unexpected value after second paste: %q", got)
	}
	pos = editor.caretPosition()
	if pos.Line != 2 || pos.Column != 0 {
		t.Fatalf("expected cursor on latest inserted line, got line %d col %d", pos.Line, pos.Column)
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
