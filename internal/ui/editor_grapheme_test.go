package ui

import (
	"fmt"
	"slices"
	"testing"
	"unicode"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/rivo/uniseg"

	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

var graphemeDocs = []string{
	"ae\u0301z qq",                    // decomposed e-acute inside a word
	"0️⃣x qq",                         // keycap digit
	"a⚙️z qq",                         // variation selector
	"a\U0001F469\u200d\U0001F4BBz qq", // zero width joiner sequence
	"\U0001F1F3\U0001F1F4 ab",         // regional indicator pair
	"a\U0001F44D\U0001F3FDz qq",       // skin tone modifier
	"GET /a\u0301/b HTTP/1.1",         // a mark in the middle of a path
}

func caretStops(t *testing.T, content string, move func(*requestEditor)) []int {
	t.Helper()
	runes := []rune(content)
	stops := make([]int, 0, len(runes)+1)
	for start := range len(runes) + 1 {
		editor := newTestEditor(content)
		line, col := editor.positionForOffset(start)
		editor.moveCursorTo(line, col)
		move(&editor)
		stops = append(stops, editor.caretPosition().Offset)
	}
	return stops
}

func TestWordMotionsLandOnCharacterBoundaries(t *testing.T) {
	for _, content := range graphemeDocs {
		runes := []rune(content)
		for _, tc := range []struct {
			name string
			move func(*requestEditor)
		}{
			{"w", func(e *requestEditor) { e.moveToWordNext(false) }},
			{"W", func(e *requestEditor) { e.moveToWordNext(true) }},
			{"e", func(e *requestEditor) { e.moveToWordEnd(false) }},
			{"E", func(e *requestEditor) { e.moveToWordEnd(true) }},
			{"b", func(e *requestEditor) { e.moveToWordStart(false) }},
			{"B", func(e *requestEditor) { e.moveToWordStart(true) }},
		} {
			for start, stop := range caretStops(t, content, tc.move) {
				if begin, _ := textarea.GraphemeRange(runes, stop); begin != stop && stop < len(runes) {
					t.Errorf("%q %s from %d landed at %d, inside the character at %d",
						content, tc.name, start, stop, begin)
				}
			}
		}
	}
}

func TestWordMotionsTreatMarksAsPartOfTheirCharacter(t *testing.T) {
	const content = "ae\u0301z qq"
	for _, tc := range []struct {
		name string
		move func(*requestEditor)
		want []int
	}{
		{"w", func(e *requestEditor) { e.moveToWordNext(false) }, []int{5, 5, 5, 5, 5, 7, 7, 7}},
		{"e", func(e *requestEditor) { e.moveToWordEnd(false) }, []int{3, 3, 3, 6, 6, 6, 6, 7}},
		{"b", func(e *requestEditor) { e.moveToWordStart(false) }, []int{0, 0, 0, 0, 0, 0, 5, 5}},
	} {
		got := caretStops(t, content, tc.move)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %d stops, want %d", tc.name, len(got), len(tc.want))
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s from %d: caret at %d, want %d (all stops %v)", tc.name, i, got[i], tc.want[i], got)
				break
			}
		}
	}
}

func TestMouseSelectionCoversWholeCharacter(t *testing.T) {
	for _, tc := range []struct {
		content string
		column  int
		want    string
	}{
		{"a⚙️z", 1, "⚙️"},
		{"a\U0001F469\u200d\U0001F4BBz", 1, "\U0001F469\u200d\U0001F4BB"},
		{"ae\u0301z", 1, "e\u0301"},
		{"a\U0001F1F3\U0001F1F4z", 1, "\U0001F1F3\U0001F1F4"},
		{"abc", 1, "b"},
	} {
		t.Run(tc.content, func(t *testing.T) {
			for _, reversed := range []bool{false, true} {
				m := newTestModelWithDoc(tc.content + "\n")
				at := cursorPosition{
					Line:   0,
					Column: tc.column,
					Offset: m.editor.offsetForPosition(0, tc.column),
				}
				anchor, caret := m.editorMouseSelectionPositions(at, at)
				if reversed {
					anchor, caret = caret, anchor
				}
				m.editor.SetManualSelection(anchor, caret)
				if got := m.editor.selectedText(); got != tc.want {
					t.Fatalf("reversed=%t: selected %q, want %q", reversed, got, tc.want)
				}
			}
		})
	}
}

func TestSingleCharacterOperationsTakeWholeCharacter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{"variation selector", "a⚙️z", "az"},
		{"zwj sequence", "a\U0001F469\u200d\U0001F4BBz", "az"},
		{"combining accent", "ae\u0301z", "az"},
		{"regional indicators", "a\U0001F1F3\U0001F1F4z", "az"},
		{"plain", "abz", "az"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deleted := editorAfter(t, tc.content, func(e requestEditor) requestEditor {
				e, _ = e.DeleteCharAtCursor()
				return e
			})
			if deleted != tc.want {
				t.Errorf("x left %q, want %q", deleted, tc.want)
			}

			changed := editorAfter(t, tc.content, func(e requestEditor) requestEditor {
				e, _ = e.ApplyInsertAction(editorInsertSubstituteChar)
				return e
			})
			if changed != tc.want {
				t.Errorf("s left %q, want %q", changed, tc.want)
			}

			selected := editorAfter(t, tc.content, func(e requestEditor) requestEditor {
				e, _ = e.ToggleVisual()
				e, _ = e.DeleteSelection()
				return e
			})
			if selected != tc.want {
				t.Errorf("visual delete left %q, want %q", selected, tc.want)
			}
		})
	}
}

func editorAfter(t *testing.T, content string, fn func(requestEditor) requestEditor) string {
	t.Helper()
	editor := newTestEditor(content)
	editor.moveCursorTo(0, 1)
	return fn(editor).Value()
}

func TestFindCharLandsOnCharacterBoundaries(t *testing.T) {
	const content = "ae\u0301z-q"
	runes := []rune(content)
	for _, command := range []string{"f-", "t-", "F-", "T-"} {
		editor := newTestEditor(content)
		if command[0] == 'f' || command[0] == 't' {
			editor.moveCursorTo(0, 0)
		} else {
			editor.moveCursorTo(0, len(runes))
		}
		editor, _ = editor.executeFindMotion(string(command[0]), rune(command[1]))
		col := editor.caretPosition().Column
		if start, _ := textarea.GraphemeRange(runes, col); start != col && col < len(runes) {
			t.Errorf("%s left the caret at %d, inside the character at %d", command, col, start)
		}
	}
}

func TestInclusiveMotionTakesWholeFinalCharacter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    string
	}{
		{"combining accent", "ae\u0301 qq", " qq"},
		{"variation selector", "-\u2699\ufe0f z", " z"},
		{"zwj sequence", "-\U0001F469\u200d\U0001F4BB z", " z"},
		{"regional indicators", "-\U0001F1F3\U0001F1F4 z", " z"},
		{"plain", "ab qq", " qq"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := newTestEditor(tc.content)
			editor.moveCursorTo(0, 0)
			anchor := editor.caretPosition()
			editor.moveToWordEnd(false)
			editor, _ = editor.DeleteMotion(anchor, deleteMotionSpec{
				command:             "e",
				includeFinalForward: true,
			})
			if got := editor.Value(); got != tc.want {
				t.Errorf("de left %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSearchCaretLandsOnCharacterBoundary(t *testing.T) {
	const content = "xe\u0301x"
	editor := newTestEditor(content)
	editor, _ = editor.ApplySearch("\u0301", false)
	if len(editor.search.matches) == 0 {
		t.Fatalf("expected the pattern to match the combining mark")
	}
	col := editor.caretPosition().Column
	runes := []rune(content)
	if start, _ := textarea.GraphemeRange(runes, col); start != col {
		t.Fatalf("search left the caret at %d, inside the character at %d", col, start)
	}
}

// A cluster starting with one of these runes indicates a split grapheme in
// these test inputs.
func orphanedMark(r rune) bool {
	switch {
	case r == 0x200D: // zero width joiner
		return true
	case r >= 0x1F3FB && r <= 0x1F3FF: // skin tone modifiers
		return true
	default:
		return unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc)
	}
}

func assertNoOrphanedMarks(t *testing.T, what, value string) {
	t.Helper()
	g := uniseg.NewGraphemes(value)
	for g.Next() {
		if r := []rune(g.Str())[0]; orphanedMark(r) {
			t.Errorf("%s left %q, which begins a character with the orphaned mark %U", what, value, r)
			return
		}
	}
}

type editCommand struct {
	name string
	run  func(requestEditor) requestEditor
}

func editCommands() []editCommand {
	return []editCommand{
		{"x", func(e requestEditor) requestEditor { e, _ = e.DeleteCharAtCursor(); return e }},
		{"s", func(e requestEditor) requestEditor {
			e, _ = e.ApplyInsertAction(editorInsertSubstituteChar)
			return e
		}},
		{"v then d", func(e requestEditor) requestEditor {
			e, _ = e.ToggleVisual()
			e, _ = e.DeleteSelection()
			return e
		}},
		{"dw", motionCommand("w", false, deleteOperator)},
		{"de", motionCommand("e", true, deleteOperator)},
		{"db", motionCommand("b", false, deleteOperator)},
		{"cw", motionCommand("w", false, changeOperator)},
		{"ce", motionCommand("e", true, changeOperator)},
		{"cb", motionCommand("b", false, changeOperator)},
		{"backspace", editorKey(tea.KeyMsg{Type: tea.KeyBackspace})},
		{"delete", editorKey(tea.KeyMsg{Type: tea.KeyDelete})},
		{"transpose", editorKey(tea.KeyMsg{Type: tea.KeyCtrlT})},
		{"newline", editorKey(tea.KeyMsg{Type: tea.KeyEnter})},
		{"delete to line start", editorKey(tea.KeyMsg{Type: tea.KeyCtrlU})},
		{"delete to line end", editorKey(tea.KeyMsg{Type: tea.KeyCtrlK})},
		{"insert", editorKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Q")})},
		{"delete word back", editorKey(tea.KeyMsg{Type: tea.KeyCtrlW})},
	}
}

func runEdit(content string, col int, run func(requestEditor) requestEditor) string {
	editor := newTestEditor(content)
	editor.moveCursorTo(0, col)
	return run(editor).Value()
}

func TestEditingNeverOrphansMarks(t *testing.T) {
	for _, content := range graphemeDocs {
		runes := []rune(content)
		for _, command := range editCommands() {
			for col := range len(runes) + 1 {
				assertNoOrphanedMarks(t,
					fmt.Sprintf("%s at column %d of %q", command.name, col, content),
					runEdit(content, col, command.run))
			}
		}
	}
}

// A split flag can leave a valid regional indicator, so checking for orphaned
// marks alone does not catch every partial deletion.
func TestEditsAgreeAcrossColumnsInsideACharacter(t *testing.T) {
	for _, content := range graphemeDocs {
		runes := []rune(content)
		for _, command := range editCommands() {
			for c := range textarea.Clusters(runes) {
				want := runEdit(content, c.Start, command.run)
				for col := c.Start + 1; col < c.End; col++ {
					if got := runEdit(content, col, command.run); got != want {
						t.Errorf("%s in %q: column %d gives %q but column %d gives %q",
							command.name, content, col, got, c.Start, want)
					}
				}
			}
		}
	}
}

type operator func(requestEditor, cursorPosition, deleteMotionSpec) (requestEditor, tea.Cmd)

func deleteOperator(e requestEditor, a cursorPosition, s deleteMotionSpec) (requestEditor, tea.Cmd) {
	return e.DeleteMotion(a, s)
}

func changeOperator(e requestEditor, a cursorPosition, s deleteMotionSpec) (requestEditor, tea.Cmd) {
	return e.ChangeMotion(a, s)
}

func motionCommand(command string, inclusive bool, apply operator) func(requestEditor) requestEditor {
	return func(e requestEditor) requestEditor {
		anchor := e.caretPosition()
		switch command {
		case "w":
			e.moveToWordNext(false)
		case "e":
			e.moveToWordEnd(false)
		case "b":
			e.moveToWordStart(false)
		}
		e, _ = apply(e, anchor, deleteMotionSpec{
			command:             command,
			includeFinalForward: inclusive,
		})
		return e
	}
}

func editorKey(msg tea.KeyMsg) func(requestEditor) requestEditor {
	return func(e requestEditor) requestEditor {
		e.Model, _ = e.Model.Update(msg)
		return e
	}
}

func TestAfterCursorEditsFollowWholeCharacter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"combining accent", "ae\u0301z"},
		{"keycap digit", "a0\ufe0f\u20e3z"},
		{"variation selector", "a⚙\ufe0fz"},
		{"zwj sequence", "a\U0001F469\u200d\U0001F4BBz"},
		{"regional indicators", "a\U0001F1F3\U0001F1F4z"},
		{"skin tone modifier", "a\U0001F44D\U0001F3FDz"},
		{"plain", "abz"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runes := []rune(tc.content)
			_, end := textarea.GraphemeRange(runes, 1)
			want := string(runes[:end]) + "Q" + string(runes[end:])

			appended := editorAfter(t, tc.content, func(e requestEditor) requestEditor {
				e, _ = e.ApplyInsertAction(editorInsertAfterCursor)
				e.Model, _ = e.Model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Q")})
				return e
			})
			if appended != want {
				t.Errorf("a then Q left %q, want %q", appended, want)
			}

			paste := func(e requestEditor) requestEditor {
				e.registerText = "Q"
				e, _ = e.PasteClipboard(true)
				return e
			}
			if err := clipboard.WriteAll("Q"); err == nil {
				if pasted := editorAfter(t, tc.content, paste); pasted != want {
					t.Errorf("p from the clipboard left %q, want %q", pasted, want)
				}
			}

			t.Setenv("PATH", t.TempDir())
			if pasted := editorAfter(t, tc.content, paste); pasted != want {
				t.Errorf("p from the register left %q, want %q", pasted, want)
			}
		})
	}
}

// Paste finds its insert point from the line start. A grapheme never spans a
// line break, so that has to agree with a scan of the whole document at every
// offset, including the line breaks themselves.
func TestCharEndFromLineStartMatchesTheDocument(t *testing.T) {
	for _, content := range []string{
		"ae\u0301z qq\na\U0001F469\u200d\U0001F4BBz\n\nGET /a\u0301/b HTTP/1.1\n",
		"\n\n\na\U0001F1F3\U0001F1F4z\n",
		"{\n\t\"名前\": \"0\ufe0f\u20e3\"\n}",
	} {
		runes := []rune(content)
		editor := newTestEditor(content)
		for at := range len(runes) + 1 {
			_, col := editor.positionForOffset(at)
			start := at - col
			if got := start + charEnd(runes[start:], col); got != charEnd(runes, at) {
				t.Errorf("%q at %d: line start gives %d, document gives %d",
					content, at, got, charEnd(runes, at))
			}
		}
	}
}

func TestEditorPositionsIgnoreEscapedRunes(t *testing.T) {
	const doc = "GET https://example.com/pro\u00adducts\n"
	editor := newTestEditor(doc)

	line := editor.LineRunes(0)
	hyphen := slices.Index(line, '\u00ad')
	if hyphen < 0 {
		t.Fatal("fixture lost its soft hyphen")
	}

	const escapeWidth = 6
	if got := editor.ColumnForVisibleCell(0, hyphen); got != hyphen {
		t.Errorf("cell %d maps to rune %d, want %d", hyphen, got, hyphen)
	}
	if got := editor.ColumnForVisibleCell(0, hyphen+escapeWidth); got != hyphen+1 {
		t.Errorf("cell after the escape maps to rune %d, want %d", got, hyphen+1)
	}

	editorPtr := &editor
	editorPtr.moveCursorTo(0, hyphen+1)
	if got := editor.caretPosition().Column; got != hyphen+1 {
		t.Errorf("caret column %d, want %d", got, hyphen+1)
	}
	if editor.Value() != doc {
		t.Error("rendering the escape changed the buffer")
	}
}
