package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestParseExCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kind  exCommandKind
		bang  bool
	}{
		{name: "empty", input: "  :", kind: exCommandEmpty},
		{name: "write", input: "w", kind: exCommandWrite},
		{name: "write alias", input: ":write", kind: exCommandWrite},
		{name: "quit", input: "q", kind: exCommandQuit},
		{name: "quit bang", input: "q!", kind: exCommandQuit, bang: true},
		{name: "quit all", input: "qa", kind: exCommandQuit},
		{name: "quit all bang", input: "qall!", kind: exCommandQuit, bang: true},
		{name: "write quit", input: "wq", kind: exCommandWriteQuit},
		{name: "write quit bang", input: "wq!", kind: exCommandWriteQuit, bang: true},
		{name: "exit", input: "x", kind: exCommandExit},
		{name: "edit", input: "edit", kind: exCommandEdit},
		{name: "help", input: "h", kind: exCommandHelp},
		{name: "no highlight", input: "nohlsearch", kind: exCommandNoHighlight},
		{name: "mock", input: "mock", kind: exCommandMock},
		{name: "mock args", input: "mock start 127.0.0.1:9090", kind: exCommandMock},
		{name: "mock bang", input: "mock!", kind: exCommandUnknown},
		{name: "trailing args", input: "w other.http", kind: exCommandTrailing},
		{name: "edit with path", input: "e file.http", kind: exCommandTrailing},
		{name: "unknown", input: "set number", kind: exCommandUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseExCommand(tt.input)
			if got.kind != tt.kind {
				t.Fatalf("kind: want %v, got %v", tt.kind, got.kind)
			}
			if got.bang != tt.bang {
				t.Fatalf("bang: want %v, got %v", tt.bang, got.bang)
			}
		})
	}
}

func TestColonOpensCommandLineFromNormalModePanes(t *testing.T) {
	tests := []struct {
		name  string
		focus paneFocus
	}{
		{name: "editor", focus: focusEditor},
		{name: "response", focus: focusResponse},
		{name: "requests", focus: focusRequests},
		{name: "file", focus: focusFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := New(Config{})
			model.ready = true
			model.focus = tt.focus
			_ = model.setInsertMode(false, false)

			updated, _ := model.Update(colonKeyMsg())
			model = updated.(Model)

			if !model.showCommandLine {
				t.Fatalf("expected command line to open for focus %v", tt.focus)
			}
			if got := model.commandLineInput.Value(); got != "" {
				t.Fatalf("expected trigger colon to be consumed, got %q", got)
			}
		})
	}
}

func TestColonDoesNotOpenCommandLineInEditorInsertMode(t *testing.T) {
	model := New(Config{})
	model.ready = true
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(true, false)

	updated, _ := model.Update(colonKeyMsg())
	model = updated.(Model)

	if model.showCommandLine {
		t.Fatal("did not expect command line in insert mode")
	}
	if !strings.Contains(model.editor.Value(), ":") {
		t.Fatalf("expected insert mode colon to enter editor text, got %q", model.editor.Value())
	}
}

func TestColonDoesNotOpenCommandLineInsideSearchPrompt(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.showSearchPrompt = true
	model.searchInput.Focus()

	updated, _ := model.Update(colonKeyMsg())
	model = updated.(Model)

	if model.showCommandLine {
		t.Fatal("did not expect command line while search prompt is active")
	}
	if got := model.searchInput.Value(); got != ":" {
		t.Fatalf("expected colon to remain search input, got %q", got)
	}
}

func TestRenderCommandLinePrompt(t *testing.T) {
	model := New(Config{})
	model.width = 80
	model.showCommandLine = true
	model.commandLineInput.Focus()

	out := ansi.Strip(model.renderCommandBar())
	if !strings.HasPrefix(out, " :") {
		t.Fatalf("expected command line to align with command bar gutter, got %q", out)
	}
	if !strings.Contains(out, "w q wq q! qa noh e help") {
		t.Fatalf("expected command hints, got %q", out)
	}
}

func TestRenderCommandLineKeepsCursorTailVisible(t *testing.T) {
	model := New(Config{})
	model.width = 24
	model.showCommandLine = true
	model.commandLineInput.SetValue(strings.Repeat("a", 30) + "TAIL")
	model.commandLineInput.CursorEnd()
	model.commandLineInput.Focus()

	out := ansi.Strip(model.renderCommandBar())
	if !strings.Contains(out, "TAIL") {
		t.Fatalf("expected long command to render cursor tail, got %q", out)
	}
	if lipgloss.Width(out) > 24 {
		t.Fatalf("expected command line width <= 24, got %d in %q", lipgloss.Width(out), out)
	}
}

func TestExQuitGuardsDirtyBuffer(t *testing.T) {
	model := New(Config{})
	model.dirty = true

	status, ok := statusMsgFromCmd(model.executeExCommand("q"))
	if !ok {
		t.Fatal("expected dirty quit warning")
	}
	if status.level != statusWarn || !strings.Contains(status.text, "No write since last change") {
		t.Fatalf("unexpected dirty quit status: %+v", status)
	}

	if !commandHasQuit(model.executeExCommand("q!")) {
		t.Fatal("expected q! to quit")
	}
}

func TestExWriteSavesCurrentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.http")
	if err := os.WriteFile(path, []byte("GET https://old.example\n"), 0o644); err != nil {
		t.Fatalf("write setup file: %v", err)
	}

	model := New(Config{
		WorkspaceRoot:  dir,
		FilePath:       path,
		InitialContent: "GET https://old.example\n",
	})
	model.editor.SetValue("GET https://new.example\n")
	model.markDirty()

	collectMsgs(model.executeExCommand("w"))

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "GET https://new.example\n" {
		t.Fatalf("unexpected saved content: %q", string(data))
	}
	if model.dirty {
		t.Fatal("expected write command to clear dirty state")
	}
}

func TestExWriteQuitQuitsOnlyAfterSuccessfulSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request.http")
	if err := os.WriteFile(path, []byte("GET https://old.example\n"), 0o644); err != nil {
		t.Fatalf("write setup file: %v", err)
	}

	model := New(Config{WorkspaceRoot: dir, FilePath: path, InitialContent: "old\n"})
	model.editor.SetValue("new\n")
	model.markDirty()

	if !commandHasQuit(model.executeExCommand("wq")) {
		t.Fatal("expected wq to save and quit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "new\n" {
		t.Fatalf("unexpected saved content: %q", string(data))
	}
}

func TestExWriteQuitDoesNotQuitAfterFailedSave(t *testing.T) {
	dir := t.TempDir()
	model := New(Config{})
	model.currentFile = dir
	model.editor.SetValue("new\n")
	model.markDirty()

	if commandHasQuit(model.executeExCommand("wq")) {
		t.Fatal("did not expect wq to quit after failed save")
	}
}

func TestExWriteQuitUnnamedBufferQuitsAfterSaveAsSubmit(t *testing.T) {
	dir := t.TempDir()
	model := New(Config{WorkspaceRoot: dir, InitialContent: "GET https://example.com\n"})

	cmd := model.executeExCommand("wq")
	if cmd != nil {
		collectMsgs(cmd)
	}
	if !model.showNewFileModal {
		t.Fatal("expected wq on unnamed buffer to open save-as modal")
	}
	if model.saveAsFollowUp == nil {
		t.Fatal("expected save-as submit to be marked for quit")
	}

	model.newFileInput.SetValue("saved")
	if !commandHasQuit(model.submitNewFile()) {
		t.Fatal("expected successful save-as submit to quit")
	}
	data, err := os.ReadFile(filepath.Join(dir, "saved.http"))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != "GET https://example.com\n" {
		t.Fatalf("unexpected saved content: %q", string(data))
	}
}

func TestExNoHighlightClearsEditorSearch(t *testing.T) {
	model := New(Config{})
	model.editor.SetValue("foo\nbar\nfoo\n")
	updated, cmd := model.editor.ApplySearch("foo", false)
	model.editor = updated
	collectMsgs(cmd)
	if !model.editor.SearchActive() {
		t.Fatal("expected search to be active before noh")
	}

	collectMsgs(model.executeExCommand("noh"))

	if model.editor.SearchActive() {
		t.Fatal("expected noh to clear editor search")
	}
}

func TestExEditAndHelpOpenExistingUI(t *testing.T) {
	model := New(Config{})

	model.executeExCommand("e")
	if !model.showOpenModal {
		t.Fatal("expected :e to open path modal")
	}

	model.closeOpenModal()
	model.executeExCommand("help")
	if !model.showHelp {
		t.Fatal("expected :help to open help")
	}
}

func colonKeyMsg() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}}
}

func commandHasQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	return messageHasQuit(msg)
}

func messageHasQuit(msg tea.Msg) bool {
	switch typed := msg.(type) {
	case nil:
		return false
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, cmd := range typed {
			if commandHasQuit(cmd) {
				return true
			}
		}
	}
	return false
}
