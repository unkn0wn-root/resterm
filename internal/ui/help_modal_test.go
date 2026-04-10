package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

func TestFilteredHelpSectionsMatchesWebSocketCommands(t *testing.T) {
	model := New(Config{})
	model.helpFilter.SetValue("websocket ping")

	sections := model.filteredHelpSections()
	if len(sections) != 1 {
		t.Fatalf("expected one matching section, got %+v", sections)
	}
	if sections[0].title != "Streaming & WebSocket" {
		t.Fatalf("expected Streaming & WebSocket section, got %q", sections[0].title)
	}
	if len(sections[0].entries) != 1 {
		t.Fatalf("expected one matching entry, got %+v", sections[0].entries)
	}
	if got := sections[0].entries[0].description; got != "WebSocket commands: console / ping / close / clear" {
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
