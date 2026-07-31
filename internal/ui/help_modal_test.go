package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/helpdoc"
)

func TestFilteredHelpSectionsMatchesEntryTokens(t *testing.T) {
	model := New(Config{})
	model.helpFilter.SetValue("reload disk")

	sections := model.filteredHelpSections()
	if len(sections) != 1 {
		t.Fatalf("expected one matching section, got %+v", sections)
	}
	if sections[0].title != "Requests & Files" {
		t.Fatalf("expected Requests & Files section, got %q", sections[0].title)
	}
	if len(sections[0].entries) != 1 {
		t.Fatalf("expected one matching entry, got %+v", sections[0].entries)
	}
	if got := sections[0].entries[0].description; got != "Reload file from disk" {
		t.Fatalf("unexpected help entry %q", got)
	}
}

func TestFilteredHelpSectionsMatchesSectionTitle(t *testing.T) {
	model := New(Config{})
	model.helpFilter.SetValue("history")

	sections := model.filteredHelpSections()
	if len(sections) == 0 {
		t.Fatalf("expected history section to match")
	}

	for _, section := range sections {
		if section.title != "History" {
			continue
		}
		if len(section.entries) < 2 {
			t.Fatalf("expected full history section entries, got %+v", section.entries)
		}
		return
	}
	t.Fatalf("expected History section in %+v", sections)
}

func TestHelpIndexOmitsStaticTopicsSection(t *testing.T) {
	model := New(Config{})
	for _, section := range model.filteredHelpSections() {
		if section.title == helpTopicsSectionTitle {
			t.Fatal("expected topics only when filtering, not in the unfiltered index")
		}
	}
}

func TestFilteredHelpSectionsMatchesWebSocketCommands(t *testing.T) {
	model := New(Config{})
	model.helpFilter.SetValue("websocket ping")

	sections := model.filteredHelpSections()
	if len(sections) != 2 {
		t.Fatalf("expected topic and shortcut sections, got %+v", sections)
	}
	if sections[0].title != helpTopicsSectionTitle {
		t.Fatalf("expected topic section first, got %q", sections[0].title)
	}
	if sections[1].title != "Streaming & WebSocket" {
		t.Fatalf("expected Streaming & WebSocket section, got %q", sections[1].title)
	}
	if len(sections[1].entries) != 1 {
		t.Fatalf("expected one matching shortcut, got %+v", sections[1].entries)
	}
	if got := sections[1].entries[0].description; got != "WebSocket commands: console / ping / close / clear" {
		t.Fatalf("unexpected help entry %q", got)
	}
}

func TestHelpKeySearchFocusOwnsChordKeys(t *testing.T) {
	model := New(Config{})
	model.showHelp = true
	model.helpJustOpened = false

	if cmd := model.handleKey(keyMsgFor("/")); cmd != nil {
		_ = cmd()
	}
	if !model.helpFilter.Focused() {
		t.Fatalf("expected help filter to be focused")
	}

	if cmd := model.handleKey(keyMsgFor("g")); cmd != nil {
		_ = cmd()
	}
	if got := model.helpFilter.Value(); got != "g" {
		t.Fatalf("expected help filter to receive typed key, got %q", got)
	}
	if model.hasPendingChord {
		t.Fatalf("did not expect help search to start a chord")
	}
}

func TestHelpKeyEscClearsThenCloses(t *testing.T) {
	model := New(Config{})
	model.showHelp = true
	model.helpJustOpened = false

	if cmd := model.handleKey(keyMsgFor("/")); cmd != nil {
		_ = cmd()
	}
	if cmd := model.handleKey(keyMsgFor("r")); cmd != nil {
		_ = cmd()
	}
	if strings.TrimSpace(model.helpFilter.Value()) == "" {
		t.Fatalf("expected help filter query to be set")
	}

	if cmd := model.handleKey(keyMsgFor("esc")); cmd != nil {
		_ = cmd()
	}
	if !model.showHelp {
		t.Fatalf("expected help to remain open after clearing search")
	}
	if model.helpFilter.Value() != "" {
		t.Fatalf("expected help filter to be cleared, got %q", model.helpFilter.Value())
	}

	if cmd := model.handleKey(keyMsgFor("esc")); cmd != nil {
		_ = cmd()
	}
	if model.showHelp {
		t.Fatalf("expected help to close after second escape")
	}
}

func TestRenderHelpOverlayShowsNoMatchesMessage(t *testing.T) {
	model := New(Config{})
	model.width = 120
	model.height = 40
	model.helpFilter.SetValue("no-such-help-entry")

	out := ansi.Strip(model.renderHelpOverlay())
	if !strings.Contains(out, "No help entries match the current filter.") {
		t.Fatalf("expected no-match help message, got %q", out)
	}
}

func TestRenderHelpOverlayDoesNotRepeatInactiveSearchHint(t *testing.T) {
	model := New(Config{})
	model.width = 120
	model.height = 40

	out := ansi.Strip(model.renderHelpOverlay())
	if !strings.Contains(out, "/ search") {
		t.Fatalf("expected top search guidance, got %q", out)
	}
	if strings.Contains(out, "Shift+F to search") {
		t.Fatalf("did not expect inactive search hint to repeat, got %q", out)
	}
}

func TestHelpQueryOpensExactTopicOrFilteredIndex(t *testing.T) {
	model := New(Config{})

	model.openHelpQuery([]string{"auth"})
	if !model.showHelp || model.helpTopic == nil || model.helpTopic.ID != "authentication" {
		t.Fatalf("expected exact alias to open authentication topic, got %+v", model.helpTopic)
	}

	model.openHelpQuery([]string{"request", "syntax"})
	if model.helpTopic != nil {
		t.Fatalf("expected an inexact query to open the index, got topic %q", model.helpTopic.ID)
	}
	if got := model.helpFilter.Value(); got != "request syntax" {
		t.Fatalf("expected query to seed the help filter, got %q", got)
	}
	if !model.helpFilter.Focused() {
		t.Fatal("expected filtered help index to focus its search input")
	}
}

func TestRenderHelpTopicUsesEmbeddedMarkdown(t *testing.T) {
	model := New(Config{})
	model.width = 100
	model.height = 36
	topic, ok := helpdoc.Lookup("quick-start")
	if !ok {
		t.Fatal("quick-start topic missing")
	}
	model.openHelpTopic(topic)

	out := ansi.Strip(model.renderHelpOverlay())
	for _, want := range []string{
		"Quick Start",
		"Create a .http or .rest file",
		"GET https://example.com/health",
		"o web docs",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected topic overlay to contain %q, got %q", want, out)
		}
	}
}

func TestHelpTopicOpenWebDocsAndClose(t *testing.T) {
	opener := &recordingURLopener{}
	model := New(Config{Version: "v2.0.0"})
	model.docsOpener = opener
	topic, _ := helpdoc.Lookup("requests")
	model.openHelpTopic(topic)

	cmd := model.handleHelpKey(keyMsgFor("o"))
	msg, ok := cmd().(docsOpenedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("expected docs result, got %+v", msg)
	}
	if len(opener.links) != 1 || !strings.Contains(opener.links[0], "/blob/v2.0.0/") {
		t.Fatalf("unexpected opened links: %+v", opener.links)
	}

	model.handleHelpKey(keyMsgFor("esc"))
	if model.showHelp || model.helpTopic != nil {
		t.Fatalf("expected Esc to close topic help, state=%t topic=%+v", model.showHelp, model.helpTopic)
	}
}

func TestHelpTopicBrowserFailureIsReturnedAsMessage(t *testing.T) {
	opener := &recordingURLopener{err: errors.New("no browser")}
	model := New(Config{})
	model.docsOpener = opener
	topic, _ := helpdoc.Lookup("requests")
	model.openHelpTopic(topic)

	msg := model.handleHelpKey(keyMsgFor("o"))().(docsOpenedMsg)
	if !errors.Is(msg.err, opener.err) {
		t.Fatalf("expected browser error, got %v", msg.err)
	}
}
