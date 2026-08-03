package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestExCatalogSuggestions(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		label  string
		insert string
	}{
		{name: "command alias", input: "ma", label: "help [topic]", insert: "help "},
		{name: "help topic", input: "help web", label: "streaming", insert: "help streaming"},
		{name: "man topic", input: "man grpc", label: "grpc", insert: "help grpc"},
		{name: "docs topic", input: "docs auth", label: "authentication", insert: "docs authentication"},
		{
			name: "mock command", input: "mock rest",
			label: "restart [host:port] [--source files] [--recursive] [--all]", insert: "mock restart ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := exCommands.Suggestions(tt.input)
			if len(items) != 1 {
				t.Fatalf("expected one suggestion, got %+v", items)
			}
			if items[0].label != tt.label || items[0].insert != tt.insert {
				t.Fatalf("unexpected suggestion: %+v", items[0])
			}
		})
	}
}

func TestExSuggestionSelectionWraps(t *testing.T) {
	state := exSuggestionState{}
	state.reset([]exSuggestion{{label: "one"}, {label: "two"}})
	if _, ok := state.selected(); ok {
		t.Fatal("expected refreshed suggestions to start without an explicit selection")
	}

	state.move(-1)
	item, ok := state.selected()
	if !ok || item.label != "two" {
		t.Fatalf("expected selection to wrap to last item, got %+v (ok=%v)", item, ok)
	}
	state.move(1)
	item, ok = state.selected()
	if !ok || item.label != "one" {
		t.Fatalf("expected selection to wrap to first item, got %+v (ok=%v)", item, ok)
	}
}

func TestCommandLineTabCompletesSelectedSuggestion(t *testing.T) {
	model := New(Config{})
	model.openCommandLine()
	model.commandLineJustOpened = false
	model.commandLineInput.SetValue("help unary")
	model.refreshCommandSuggestions()

	model.handleCommandLineKey(tea.KeyMsg{Type: tea.KeyTab})

	if got := model.commandLineInput.Value(); got != "help grpc" {
		t.Fatalf("expected selected topic completion, got %q", got)
	}
}

func TestCommandLineEnterExecutesExplicitSelection(t *testing.T) {
	model := New(Config{})
	model.openCommandLine()
	model.commandLineJustOpened = false
	model.commandLineInput.SetValue("help req")
	model.refreshCommandSuggestions()
	for range model.commandSuggestions.items {
		model.commandSuggestions.move(1)
		item, ok := model.commandSuggestions.selected()
		if ok && item.insert == "help requests" {
			break
		}
	}
	item, ok := model.commandSuggestions.selected()
	if !ok || item.insert != "help requests" {
		t.Fatalf("requests suggestion missing from %+v", model.commandSuggestions.items)
	}

	model.handleCommandLineKey(tea.KeyMsg{Type: tea.KeyEnter})

	if !model.showHelp || model.helpTopic == nil || model.helpTopic.ID != "requests" {
		t.Fatalf("expected selected requests topic, state=%t topic=%+v", model.showHelp, model.helpTopic)
	}
}

func TestCommandLineEnterExecutesTypedPromptWithoutSelection(t *testing.T) {
	model := New(Config{})
	model.openCommandLine()
	model.commandLineJustOpened = false
	model.commandLineInput.SetValue("help req")
	model.refreshCommandSuggestions()

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
	model.commandSuggestions.reset(exCommands.Suggestions("he"))
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
