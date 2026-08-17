package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/prompt"
)

func TestExCatalogSuggestions(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		label  string
		insert string
	}{
		{name: "command alias", input: "ma", label: "help [topic] (:h, :man)", insert: "help "},
		{name: "help topic", input: "help web", label: "streaming", insert: "help streaming"},
		{name: "man topic", input: "man grpc", label: "grpc", insert: "help grpc"},
		{name: "docs topic", input: "docs auth", label: "authentication", insert: "docs authentication"},
		{name: "mock command", input: "mock rest", label: "restart [host:port] [flags]", insert: "mock restart "},
		{
			name: "mock arguments", input: "mock start --rec",
			label: "start [host:port] [--source files] [--recursive] [--all]", insert: "mock start --rec",
		},
		{
			name: "mock arguments after completion", input: "mock reset ",
			label: "reset [sequence]", insert: "mock reset ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := exCommands.Suggestions(tt.input)
			if len(items) != 1 {
				t.Fatalf("expected one suggestion, got %+v", items)
			}
			insert, _, err := items[0].Edit.Apply(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if items[0].Label != tt.label || insert != tt.insert {
				t.Fatalf("unexpected suggestion: %+v", items[0])
			}
		})
	}
}

func TestExCommandLabelsShowAliases(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "write", want: "write (:w)"},
		{name: "quit", want: "quit[!] (:q, :qa, :qall)"},
		{name: "wq", want: "wq"},
		{name: "exit", want: "exit (:x, :xit)"},
		{name: "edit", want: "edit [path] (:e)"},
		{name: "help", want: "help [topic] (:h, :man)"},
		{name: "nohlsearch", want: "nohlsearch (:noh)"},
		{name: "mock", want: "mock [command]"},
		{name: "docs", want: "docs [topic]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := exCommands.Lookup(tt.name)
			if !ok {
				t.Fatalf("command %q not found", tt.name)
			}
			if got := def.label(); got != tt.want {
				t.Fatalf("label = %q, want %q", got, tt.want)
			}
		})
	}
}

// A long label used to take the whole row and leave the summary one character.
func TestCommandSuggestionPopupKeepsBothColumnsReadable(t *testing.T) {
	for _, width := range []int{80, 120} {
		model := New(Config{})
		model.width = width
		model.commandLine.menu.Reset(exCommands.Suggestions("mock "))
		content := strings.Repeat(strings.Repeat(" ", width)+"\n", 13) + strings.Repeat(" ", width)

		plain := ansi.Strip(model.renderCommandSuggestionPopup(content, 1))
		for _, want := range []string{
			"status",
			"Show server address and counters",
			"start",
			"Start the mock server",
			"capture",
			"Capture the focused response as a mock",
		} {
			if !strings.Contains(plain, want) {
				t.Fatalf("width %d popup is missing %q:\n%s", width, want, plain)
			}
		}
	}
}

// A subcommand that takes no arguments has no grammar to hint at, and neither
// does a name that does not exist.
func TestMockArgumentHintOnlyWhereArgumentsExist(t *testing.T) {
	for _, input := range []string{"mock status ", "mock logs --all", "mock unsupported --all", "mock  start"} {
		if items := exCommands.Suggestions(input); len(items) != 0 {
			t.Fatalf("%q suggested %+v, want nothing", input, items)
		}
	}
}

// The hint is the whole command, not a value to insert, so completing it must
// leave what the user typed alone.
func TestMockArgumentHintCompletesToItself(t *testing.T) {
	model := New(Config{})
	model.openCommandLine()
	model.commandLineJustOpened = false
	model.commandLine.input.SetValue("mock start 127.0.0.1:9000 --rec")
	model.commandLine.refresh(model.commandSource())

	model.handleCommandLineKey(tea.KeyMsg{Type: tea.KeyTab})

	if got := model.commandLine.value(); got != "mock start 127.0.0.1:9000 --rec" {
		t.Fatalf("argument hint rewrote the command line to %q", got)
	}
}

// Neither column may starve the other, and a row too narrow for both drops the
// summary instead of showing a stub of one.
func TestCompletionPopupColumnsShareTheRow(t *testing.T) {
	tests := []struct {
		name                   string
		label, summary, textW  int
		wantLabel, wantSummary int
	}{
		{name: "both fit", label: 27, summary: 37, textW: 74, wantLabel: 27, wantSummary: 37},
		{name: "summary trimmed", label: 27, summary: 49, textW: 60, wantLabel: 27, wantSummary: 32},
		{name: "label yields", label: 58, summary: 49, textW: 40, wantLabel: 23, wantSummary: 16},
		{name: "summary dropped", label: 58, summary: 49, textW: 24, wantLabel: 24, wantSummary: 0},
		{name: "no summary", label: 20, summary: 0, textW: 40, wantLabel: 20, wantSummary: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labelW, summaryW := completionPopupColumns(tt.label, tt.summary, tt.textW)
			if labelW != tt.wantLabel || summaryW != tt.wantSummary {
				t.Fatalf("columns = %d, %d, want %d, %d", labelW, summaryW, tt.wantLabel, tt.wantSummary)
			}
			if labelW+summaryW > tt.textW {
				t.Fatalf("columns %d + %d overflow %d", labelW, summaryW, tt.textW)
			}
		})
	}
}

func TestCommandLineTabCompletesSelectedSuggestion(t *testing.T) {
	model := New(Config{})
	model.openCommandLine()
	model.commandLineJustOpened = false
	model.commandLine.input.SetValue("help unary")
	model.commandLine.refresh(model.commandSource())

	model.handleCommandLineKey(tea.KeyMsg{Type: tea.KeyTab})

	if got := model.commandLine.value(); got != "help grpc" {
		t.Fatalf("expected selected topic completion, got %q", got)
	}
}

func TestCommandLineEnterExecutesExplicitSelection(t *testing.T) {
	model := New(Config{})
	model.openCommandLine()
	model.commandLineJustOpened = false
	model.commandLine.input.SetValue("help req")
	model.commandLine.refresh(model.commandSource())
	for range model.commandLine.menu.Items() {
		model.commandLine.menu.Move(1)
		item, ok := model.commandLine.menu.Selected()
		if ok && exSuggestionValue(t, "help req", item) == "help requests" {
			break
		}
	}
	item, ok := model.commandLine.menu.Selected()
	if !ok || exSuggestionValue(t, "help req", item) != "help requests" {
		t.Fatalf("requests suggestion missing from %+v", model.commandLine.menu.Items())
	}

	model.handleCommandLineKey(tea.KeyMsg{Type: tea.KeyEnter})

	if !model.showHelp || model.helpTopic == nil || model.helpTopic.ID != "requests" {
		t.Fatalf("expected selected requests topic, state=%t topic=%+v", model.showHelp, model.helpTopic)
	}
}

func exSuggestionValue(t *testing.T, input string, item prompt.Item) string {
	t.Helper()
	value, _, err := item.Edit.Apply(input)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestCommandLineEnterExecutesTypedPromptWithoutSelection(t *testing.T) {
	model := New(Config{})
	model.openCommandLine()
	model.commandLineJustOpened = false
	model.commandLine.input.SetValue("help req")
	model.commandLine.refresh(model.commandSource())

	model.handleCommandLineKey(tea.KeyMsg{Type: tea.KeyEnter})

	if !model.showHelp || model.helpTopic != nil {
		t.Fatalf("expected typed partial query to open help index, state=%t topic=%+v", model.showHelp, model.helpTopic)
	}
	if got := model.helpFilter.Value(); got != "req" {
		t.Fatalf("expected typed query to seed help filter, got %q", got)
	}
}

func TestCommandLineEmptyEnterDoesNotRunFirstSuggestion(t *testing.T) {
	model := New(Config{})
	model.openCommandLine()
	model.commandLineJustOpened = false

	cmd := model.handleCommandLineKey(tea.KeyMsg{Type: tea.KeyEnter})
	status, ok := statusMsgFromCmd(cmd)
	if !ok || status.level != statusWarn || status.text != "Enter a command" {
		t.Fatalf("expected empty-command warning, got %+v (ok=%v)", status, ok)
	}
}

func TestRenderCommandSuggestionPopupOverlaysWithoutReflow(t *testing.T) {
	model := New(Config{})
	model.width = 80
	model.commandLine.menu.Reset(exCommands.Suggestions("he"))
	content := strings.Repeat(strings.Repeat(" ", 80)+"\n", 11) + strings.Repeat(" ", 80)

	out := model.renderCommandSuggestionPopup(content, 1)

	plain := ansi.Strip(out)
	if !strings.Contains(plain, "help [topic]") {
		t.Fatalf("expected help suggestion in popup, got %q", plain)
	}
	if got := lipgloss.Height(out); got != lipgloss.Height(content) {
		t.Fatalf("popup changed layout height from %d to %d", lipgloss.Height(content), got)
	}
	if got := lipgloss.Width(out); got != 80 {
		t.Fatalf("popup changed layout width to %d", got)
	}
}
