package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestContextHelpResolvesDirectiveCommentStyles(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		topic string
	}{
		{name: "hash", line: "# @auth bearer token", topic: "authentication"},
		{name: "slash", line: "// @grpc inventory.Service/Get", topic: "grpc"},
		{name: "dash", line: "-- @trace dns<=10ms", topic: "tracing"},
		{name: "inline block", line: "/* @mock status=200 */", topic: "mocks"},
		{name: "block continuation", line: " * @workflow checkout", topic: "workflows"},
		{name: "directive disambiguation", line: "# @variables", topic: "graphql"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := New(Config{})
			model.editor.SetValue(tt.line)
			model.editor.moveCursorTo(0, 0)
			topic, ok := model.contextHelpTopic()
			if !ok || topic.ID != tt.topic {
				t.Fatalf("expected topic %q, got %+v (ok=%v)", tt.topic, topic, ok)
			}
		})
	}
}

func TestContextHelpResolvesTemplateAndKeyword(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		col   int
		topic string
	}{
		{name: "template", line: "GET https://{{host}}/health", col: 15, topic: "variables"},
		{name: "HTTP method", line: "POST https://example.com", col: 2, topic: "requests"},
		{name: "protocol", line: "websocket", col: 4, topic: "streaming"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := New(Config{})
			model.editor.SetValue(tt.line)
			model.editor.moveCursorTo(0, tt.col)
			topic, ok := model.contextHelpTopic()
			if !ok || topic.ID != tt.topic {
				t.Fatalf("expected topic %q, got %+v (ok=%v)", tt.topic, topic, ok)
			}
		})
	}
}

func TestContextHelpNoMatchReturnsGuidance(t *testing.T) {
	model := New(Config{})
	model.editor.SetValue("unrecognized")
	model.editor.moveCursorTo(0, 3)

	status, ok := statusMsgFromCmd(model.showContextHelp())
	if !ok || status.level != statusWarn {
		t.Fatalf("expected warning status, got %+v (ok=%v)", status, ok)
	}
	if status.text != "No help topic for this context; press ? or run :help" {
		t.Fatalf("unexpected guidance %q", status.text)
	}
}

func TestContextHelpBindingOpensTopicOnlyInEditorNormalMode(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusEditor
	model.editor.SetValue("# @auth bearer token")
	model.editor.moveCursorTo(0, 3)
	_ = model.setInsertMode(false, false)

	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})

	if !model.showHelp || model.helpTopic == nil || model.helpTopic.ID != "authentication" {
		t.Fatalf("expected K to open authentication help, state=%t topic=%+v", model.showHelp, model.helpTopic)
	}
}

func TestContextHelpBindingTypesInEditorInsertMode(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.editor.SetValue("")
	_ = model.setFocus(focusEditor)
	_ = model.setInsertMode(true, false)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})
	model = updated.(Model)

	if model.showHelp {
		t.Fatal("did not expect contextual help in insert mode")
	}
	if !strings.Contains(model.editor.Value(), "K") {
		t.Fatalf("expected K to be inserted, got %q", model.editor.Value())
	}
}

func TestContextHelpBindingKeepsResponseScrollBehavior(t *testing.T) {
	model := New(Config{})
	model.focus = focusResponse
	pane := model.focusedPane()
	pane.viewport.Width = 40
	pane.viewport.Height = 2
	pane.viewport.SetContent("one\ntwo\nthree\nfour\nfive")
	pane.viewport.SetYOffset(2)

	model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'K'}})

	if model.showHelp {
		t.Fatal("did not expect contextual help in the response pane")
	}
	if got := pane.viewport.YOffset; got != 1 {
		t.Fatalf("expected K to retain response scroll-up behavior, got offset %d", got)
	}
}
