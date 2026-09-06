package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/intellisense"
	"github.com/unkn0wn-root/resterm/internal/prompt"
)

func selectEditorCompletion(t *testing.T, editor *requestEditor, label string) {
	t.Helper()
	for i, item := range editor.completion.filtered {
		if item.Label == label {
			editor.completion.selection = i
			return
		}
	}
	t.Fatalf("completion %q not found in %v", label, editor.completion.filtered)
}

func editorCompletionLabels(editor requestEditor) []string {
	labels := make([]string, len(editor.completion.filtered))
	for i, item := range editor.completion.filtered {
		labels[i] = item.Label
	}
	return labels
}

func deliverEditorPathRead(t *testing.T, editor *requestEditor, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a pending path read")
	}
	msg, ok := cmd().(pathReadMsg)
	if !ok {
		t.Fatalf("path read command returned %T", msg)
	}
	if msg.id != promptEditor {
		t.Fatalf("path read prompt = %v, want editor", msg.id)
	}
	editor.deliverPathCompletion(msg.read)
}

func TestRequestEditorChainsTypedDirectiveStages(t *testing.T) {
	editor := newTestEditor("# @auth ")
	editor.moveCursorTo(0, len([]rune(editor.Value())))
	editor.SetCompletionEnabled(true)
	editor.openCompletions()

	selectEditorCompletion(t, &editor, "oauth2")
	editor.applyCompletion()
	if got := editor.Value(); got != "# @auth oauth2 " {
		t.Fatalf("oauth2 stage = %q", got)
	}

	selectEditorCompletion(t, &editor, "grant=")
	editor.applyCompletion()
	if got := editor.Value(); got != "# @auth oauth2 grant=" {
		t.Fatalf("grant stage = %q", got)
	}

	selectEditorCompletion(t, &editor, "client_credentials")
	editor.applyCompletion()
	if got := editor.Value(); got != "# @auth oauth2 grant=client_credentials " {
		t.Fatalf("grant value = %q", got)
	}
	if !editor.hasActiveCompletion() {
		t.Fatal("expected remaining OAuth options after accepting the grant")
	}
	if slices.Contains(editorCompletionLabels(editor), "grant=") {
		t.Fatalf("completed grant was offered again: %v", editor.completion.filtered)
	}
}

func TestRequestEditorBrowsesAndChainsUsePaths(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "modules")
	if err := os.Mkdir(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		filepath.Join(root, "ignored.txt"):      "ignored",
		filepath.Join(moduleDir, "helpers.rts"): "export let helper = 1",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	editor := newTestEditor("# @use ")
	editor.moveCursorTo(0, len([]rune(editor.Value())))
	editor.SetCompletionRoot(root)
	editor.SetCompletionEnabled(true)
	deliverEditorPathRead(t, &editor, editor.openCompletions())

	labels := editorCompletionLabels(editor)
	if slices.Contains(labels, "ignored.txt") {
		t.Fatalf("RTS path suggestions included ignored.txt: %v", labels)
	}

	selectEditorCompletion(t, &editor, "modules/")
	deliverEditorPathRead(t, &editor, editor.applyCompletion())
	if got := editor.Value(); got != "# @use modules/" {
		t.Fatalf("directory completion = %q", got)
	}

	selectEditorCompletion(t, &editor, "helpers.rts")
	editor.applyCompletion()
	if got := editor.Value(); got != "# @use modules/helpers.rts " {
		t.Fatalf("file completion = %q", got)
	}
	if !editor.hasActiveCompletion() || !slices.Contains(editorCompletionLabels(editor), "as") {
		t.Fatalf("expected @use alias stage, got %v", editor.completion.filtered)
	}
}

func TestRequestEditorKeepsBrowsingAcrossSpacesInPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module with spaces.rts"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}

	editor := newTestEditor(`# @use "module`)
	editor.moveCursorTo(0, len([]rune(editor.Value())))
	editor.SetCompletionRoot(root)
	editor.SetCompletionEnabled(true)
	deliverEditorPathRead(t, &editor, editor.openCompletions())

	editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if got := editor.Value(); got != `# @use "module ` {
		t.Fatalf("editor value = %q", got)
	}
	if !editor.hasActiveCompletion() {
		t.Fatal("the space closed the path popup")
	}
	selectEditorCompletion(t, &editor, "module with spaces.rts")
	editor.applyCompletion()
	if got := editor.Value(); got != "# @use \"module with spaces.rts\" " {
		t.Fatalf("quoted path completion = %q", got)
	}
}

func TestContextualCtrlNInEditorInsertMode(t *testing.T) {
	m := New(Config{WorkspaceRoot: t.TempDir()})
	m.focus = focusEditor
	m.editor.SetValue("# @auth ")
	m.editor.moveCursorTo(0, len([]rune(m.editor.Value())))
	_ = m.setInsertMode(true, false)

	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	if !m.editor.hasActiveCompletion() {
		t.Fatal("inactive Ctrl+N did not open contextual choices")
	}
	if m.showNewFileModal {
		t.Fatal("Ctrl+N opened New Request in editor insert mode")
	}
	selection := m.editor.completion.selection
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.editor.completion.selection == selection {
		t.Fatal("active Ctrl+N did not select the next completion")
	}

	m.editor.SetValue("GET https://example.test")
	m.editor.moveCursorTo(0, len([]rune(m.editor.Value())))
	m.editor.completion.deactivate()
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.editor.completion.active || m.showNewFileModal {
		t.Fatal("Ctrl+N with no contextual choices should be a no-op")
	}

	_ = m.setInsertMode(false, false)
	m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlN})
	if !m.showNewFileModal {
		t.Fatal("Ctrl+N outside editor insert mode did not keep New Request behavior")
	}
}

func TestRequestEditorRejectsStalePathReads(t *testing.T) {
	for _, change := range []string{"document", "caret", "root", "dismiss", "disable", "reopen"} {
		t.Run(change, func(t *testing.T) {
			editor := newTestEditor("# @use ")
			editor.moveCursorTo(0, len([]rune(editor.Value())))
			editor.SetCompletionRoot(t.TempDir())
			editor.SetCompletionEnabled(true)
			// Control delivery order so the test does not depend on read timing.
			cmd := editor.openCompletions()
			if cmd == nil {
				t.Fatal("expected directory read")
			}
			msg := cmd().(pathReadMsg)
			msg.read.Entries = []prompt.DirEntry{{Name: "module.rts"}}
			switch change {
			case "document":
				editor.SetValue("# @auth ")
			case "caret":
				editor.moveCursorTo(0, 0)
			case "root":
				editor.SetCompletionRoot(t.TempDir())
			case "dismiss":
				editor.closeCompletions()
			case "disable":
				editor.SetCompletionEnabled(false)
			case "reopen":
				editor.closeCompletions()
				cmd := editor.openCompletions()
				if cmd == nil {
					t.Fatal("reopened completion did not read the directory")
				}
				fresh := cmd().(pathReadMsg)
				fresh.read.Entries = []prompt.DirEntry{{Name: "fresh.rts"}}
				editor.deliverPathCompletion(fresh.read)
			}
			editor.deliverPathCompletion(msg.read)
			if change == "reopen" {
				if got := editorCompletionLabels(editor); !slices.Equal(got, []string{"fresh.rts"}) {
					t.Fatalf("late read replaced the new session's suggestions: %v", got)
				}
				return
			}
			if editor.hasActiveCompletion() {
				t.Fatalf("late read survived %s: %+v", change, editor.completion)
			}
		})
	}
}

func TestRequestEditorPendingPathReadUsesLatestQuery(t *testing.T) {
	editor := newTestEditor("# @use ")
	editor.moveCursorTo(0, len([]rune(editor.Value())))
	editor.SetCompletionRoot(t.TempDir())
	editor.SetCompletionEnabled(true)
	cmd := editor.openCompletions()
	if cmd == nil {
		t.Fatal("expected directory read")
	}
	msg := cmd().(pathReadMsg)
	msg.read.Entries = []prompt.DirEntry{{Name: "module.rts"}, {Name: "another.rts"}}
	editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("mod")})
	if next := editor.openCompletions(); next != nil {
		t.Fatal("same directory started a duplicate read")
	}
	editor.deliverPathCompletion(msg.read)
	if got := editorCompletionLabels(editor); !slices.Equal(got, []string{"module.rts"}) {
		t.Fatalf("pending read did not use latest query: %v", got)
	}
	editor.applyCompletion()
	if got := editor.Value(); got != "# @use module.rts " {
		t.Fatalf("pending read used stale replacement offsets: %q", got)
	}
}

func TestRequestEditorCompletesOneProfileListSegment(t *testing.T) {
	for _, input := range []string{
		"# @apply use=|ø,use=two,use=three",
		"# @apply use=ø|,use=two,use=three",
		"# @apply use=one,use=|ø,use=three",
		"# @apply use=one,use=ø|,use=three",
	} {
		t.Run(input, func(t *testing.T) {
			at := strings.IndexByte(input, '|')
			line := strings.Replace(input, "|", "", 1)
			editor := newTestEditor(line)
			editor.moveCursorTo(0, len([]rune(input[:at])))
			editor.SetCompletionScope(intellisense.Scope{Profiles: intellisense.ProfileSet{
				Patch: []string{"øne", "one", "two", "three"},
			}})
			editor.SetCompletionEnabled(true)
			editor.openCompletions()
			labels := editorCompletionLabels(editor)
			if slices.Contains(labels, "three") {
				t.Fatal("offered a profile already selected after the caret")
			}
			if strings.Contains(line, "use=one") && slices.Contains(labels, "one") {
				t.Fatal("offered a profile already selected before the caret")
			}
			selectEditorCompletion(t, &editor, "øne")
			editor.applyCompletion()
			if want := strings.Replace(line, "ø", "øne", 1); editor.Value() != want {
				t.Fatalf("list completion = %q, want %q", editor.Value(), want)
			}
		})
	}
}

func TestRequestEditorChainsPastExistingSeparator(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"# @auth oauth2 grant=pass|word ", "# @auth oauth2 grant=password "},
		{"# @auth oauth2 grant=pass|word\nGET https://example.test", "# @auth oauth2 grant=password \nGET https://example.test"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			before, after, _ := strings.Cut(tc.input, "|")
			editor := newTestEditor(before + after)
			editor.moveCursorTo(0, len([]rune(before)))
			editor.SetCompletionEnabled(true)
			editor.openCompletions()
			selectEditorCompletion(t, &editor, "password")
			editor.applyCompletion()
			if got := editor.Value(); got != tc.want {
				t.Fatalf("value = %q, want %q", got, tc.want)
			}
			if got, want := editor.caretPosition().Offset, len([]rune("# @auth oauth2 grant=password ")); got != want {
				t.Fatalf("caret = %d, want %d after the separator", got, want)
			}
			labels := editorCompletionLabels(editor)
			if slices.Contains(labels, "password") || slices.Contains(labels, "grant=") {
				t.Fatalf("grant stage was offered again: %v", labels)
			}
			if !slices.Contains(labels, "client_id=") {
				t.Fatalf("expected the remaining OAuth options, got %v", labels)
			}
		})
	}
}
