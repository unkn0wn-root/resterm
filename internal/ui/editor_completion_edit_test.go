package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/intellisense"
)

func TestRequestEditorCompletionPreservesExistingText(t *testing.T) {
	cases := []struct {
		name, input, label, want string
	}{
		{"header delimiter", "GET https://example.test\nCont|ent-Type: application/json", "Content-Type", "GET https://example.test\nContent-Type: application/json"},
		{"header without delimiter", "GET https://example.test\nCont|ent-Type", "Content-Type", "GET https://example.test\nContent-Type: "},
		{"header spacing", "GET https://example.test\nCont|ent-Type:    application/json", "Content-Type", "GET https://example.test\nContent-Type:    application/json"},
		{"URL suffix", "GET htt|ps://example.test/v1", "https://", "GET https://example.test/v1"},
		{"partial scheme delimiter", "GET ht|tp:/example.test/v1", "https://", "GET https://example.test/v1"},
		{"different scheme", "GET h|ttps://example.test/v1?q=ø#part", "http://", "GET http://example.test/v1?q=ø#part"},
		{"spaced template", "GET https://{{ ho| }}", "host", "GET https://{{ host }}"},
		{"partial template delimiter", "GET https://{{ ho| }/v1", "host", "GET https://{{ host }}/v1"},
		{"missing template delimiter", "GET https://{{ ho| ", "host", "GET https://{{ host }}"},
		{"existing helper arguments", "GET https://{{ $randomI|nt(10, 20) }}", "$randomInt", "GET https://{{ $randomInt(10, 20) }}"},
		{"spaced helper arguments", "GET https://{{ $randomI|nt (10, 20) }}", "$randomInt", "GET https://{{ $randomInt (10, 20) }}"},
		{"unfinished helper arguments", "GET https://{{ $randomI|nt(10, ", "$randomInt", "GET https://{{ $randomInt(10, "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, after, _ := strings.Cut(tc.input, "|")
			editor := newTestEditor(before + after)
			line := strings.Count(before, "\n")
			col := len([]rune(before[strings.LastIndexByte(before, '\n')+1:]))
			editor.moveCursorTo(line, col)
			editor.SetCompletionScope(intellisense.Scope{Variables: []intellisense.VarRef{{Name: "host"}}})
			editor.SetCompletionEnabled(true)
			editor, _ = editor.NextCompletion()
			selectEditorCompletion(t, &editor, tc.label)
			editor.applyCompletion()
			if got := editor.Value(); got != tc.want {
				t.Fatalf("got %q; want %q", got, tc.want)
			}
			if editor.hasSelection() {
				t.Fatal("completion selected existing text as a placeholder")
			}
		})
	}
}

func TestRequestEditorRefreshesPathsAfterReopen(t *testing.T) {
	for _, change := range []string{"disable", "dismiss", "document"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			editor := newTestEditor("# @use ")
			editor.moveCursorTo(0, len([]rune(editor.Value())))
			editor.SetCompletionRoot(root)
			editor.SetCompletionEnabled(true)
			deliverEditorPathRead(t, &editor, editor.openCompletions())
			if cmd := editor.openCompletions(); cmd != nil {
				t.Fatal("repeated query reread the directory in the same session")
			}
			switch change {
			case "disable":
				editor.SetCompletionEnabled(false)
			case "dismiss":
				editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyEsc})
			case "document":
				editor.SetValue("# @use n")
				editor.moveCursorTo(0, len([]rune(editor.Value())))
			}
			if err := os.WriteFile(filepath.Join(root, "new.rts"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			editor.SetCompletionEnabled(true)
			deliverEditorPathRead(t, &editor, editor.openCompletions())
			if got := editorCompletionLabels(editor); !slices.Contains(got, "new.rts") {
				t.Fatalf("new file missing after reopening completion: %v", got)
			}
		})
	}
}

func TestRequestEditorCompletesPathListAtCaret(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "twø.pem"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		"# @settings http-root-cas=tw|o.pem,three.pem",
		"# @settings http-root-cas=one.pem,tw|o.pem,three.pem",
		"# @settings grpc-root-cas=one.pem;tw|o.pem;three.pem",
		`# @settings http-root-cas="one.pem; tw|o.pem, three.pem"`,
		`# @setting http-root-cas "one.pem; tw|o.pem, three.pem"`,
	} {
		t.Run(input, func(t *testing.T) {
			before, after, _ := strings.Cut(input, "|")
			editor := newTestEditor(before + after)
			editor.moveCursorTo(0, len([]rune(before)))
			editor.SetCompletionRoot(root)
			editor.SetCompletionEnabled(true)
			deliverEditorPathRead(t, &editor, editor.openCompletions())
			selectEditorCompletion(t, &editor, "twø.pem")
			editor.applyCompletion()
			want := strings.Replace(before+after, "two.pem", "twø.pem", 1)
			if got := editor.Value(); got != want {
				t.Fatalf("path list = %q, want %q", got, want)
			}
			end := strings.Index(want, "twø.pem") + len("twø.pem")
			if got := editor.caretPosition().Offset; got != len([]rune(want[:end])) {
				t.Fatalf("caret = %d, want it after the completed path", got)
			}
			if editor.hasActiveCompletion() {
				t.Fatal("accepting a file reopened completion for the same path")
			}
		})
	}
}

func TestRequestEditorBrowsesDirectoryInsidePathList(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "certs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "certs", "two.pem"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	const input = `# @settings http-root-cas="one.pem; ce|, three.pem"`
	before, after, _ := strings.Cut(input, "|")
	editor := newTestEditor(before + after)
	editor.moveCursorTo(0, len([]rune(before)))
	editor.SetCompletionRoot(root)
	editor.SetCompletionEnabled(true)
	deliverEditorPathRead(t, &editor, editor.openCompletions())
	selectEditorCompletion(t, &editor, "certs/")
	deliverEditorPathRead(t, &editor, editor.applyCompletion())
	selectEditorCompletion(t, &editor, "two.pem")
	editor.applyCompletion()
	if got, want := editor.Value(), `# @settings http-root-cas="one.pem; certs/two.pem, three.pem"`; got != want {
		t.Fatalf("path list = %q, want %q", got, want)
	}

	editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if got, want := editor.Value(), `# @settings http-root-cas="one.pem; certs/two.pemX, three.pem"`; got != want {
		t.Fatalf("text after completion = %q, want %q", got, want)
	}
}
