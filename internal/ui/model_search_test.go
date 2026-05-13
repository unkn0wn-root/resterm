package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRetreatResponseSearchWrap(t *testing.T) {
	model := New(Config{})
	model.responsePaneFocus = responsePanePrimary
	model.searchResponsePane = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.viewport.Width = 80
	pane.snapshot = &responseSnapshot{
		id:      "snap-1",
		pretty:  withTrailingNewline("foo bar\nfoo baz\nfoo"),
		raw:     withTrailingNewline("foo bar\nfoo baz\nfoo"),
		headers: withTrailingNewline("Status: 200 OK"),
		ready:   true,
	}

	if status := statusFromCmd(t, model.applyResponseSearch("foo", false)); status == nil {
		t.Fatal("expected initial response search status")
	}

	cmd := model.retreatResponseSearch()
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after retreating search")
	}
	if status.level != statusInfo {
		t.Fatalf("expected info level, got %v", status.level)
	}
	if !strings.Contains(status.text, "Match 3/3") {
		t.Fatalf("expected wrap to last match, got %q", status.text)
	}
	if !strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("expected wrapped indicator, got %q", status.text)
	}
	if pane.search.index != 2 {
		t.Fatalf("expected search index 2, got %d", pane.search.index)
	}

	cmd = model.retreatResponseSearch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after moving to previous match")
	}
	if !strings.Contains(status.text, "Match 2/3") {
		t.Fatalf("expected to move to second match, got %q", status.text)
	}
	if strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("did not expect wrapped indicator on second match, got %q", status.text)
	}
	if pane.search.index != 1 {
		t.Fatalf("expected search index 1, got %d", pane.search.index)
	}
}

func TestClearResponseSearchOnEsc(t *testing.T) {
	model := New(Config{})
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.viewport.Width = 80
	pane.snapshot = &responseSnapshot{
		id:      "snap-1",
		pretty:  withTrailingNewline("foo bar\nfoo baz\nfoo"),
		raw:     withTrailingNewline("foo bar\nfoo baz\nfoo"),
		headers: withTrailingNewline("Status: 200 OK"),
		ready:   true,
	}

	if status := statusFromCmd(t, model.applyResponseSearch("foo", false)); status == nil {
		t.Fatal("expected initial response search status")
	}
	if !pane.search.active || len(pane.search.matches) == 0 {
		t.Fatalf("expected search to be active with matches")
	}

	cmd := model.handleKeyWithChord(tea.KeyMsg{Type: tea.KeyEscape}, false)
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after clearing search")
	}
	if status.text != "Search cleared" {
		t.Fatalf("expected \"Search cleared\" status, got %q", status.text)
	}
	if pane.search.hasQuery() || pane.search.active || len(pane.search.matches) != 0 {
		t.Fatalf(
			"expected search state cleared, got query=%q active=%v matches=%d",
			pane.search.query,
			pane.search.active,
			len(pane.search.matches),
		)
	}
}

func TestSlashOpensEditorSearchPromptWithoutTypingSlash(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusEditor
	model.editorInsertMode = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)

	if !model.showSearchPrompt {
		t.Fatal("expected search prompt to open")
	}
	if model.searchTarget != searchTargetEditor {
		t.Fatalf("expected editor search target, got %v", model.searchTarget)
	}
	if got := model.searchInput.Value(); got != "" {
		t.Fatalf("expected trigger slash to be consumed, got search input %q", got)
	}
}

func TestSlashOpensResponseSearchPromptWithoutTypingSlash(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.activeTab = responseTabPretty

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)

	if !model.showSearchPrompt {
		t.Fatal("expected search prompt to open")
	}
	if model.searchTarget != searchTargetResponse {
		t.Fatalf("expected response search target, got %v", model.searchTarget)
	}
	if got := model.searchInput.Value(); got != "" {
		t.Fatalf("expected trigger slash to be consumed, got search input %q", got)
	}
}

func TestSlashStillOpensHistoryFilter(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.activeTab = responseTabHistory

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(Model)

	if model.showSearchPrompt {
		t.Fatal("did not expect response search prompt on history slash")
	}
	if !model.historyFilterActive {
		t.Fatal("expected history filter to open")
	}
}

func TestShiftFStillOpensResponseSearchOnHistoryTab(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.activeTab = responseTabHistory

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	model = updated.(Model)

	if !model.showSearchPrompt {
		t.Fatal("expected response search prompt to open")
	}
	if model.searchTarget != searchTargetResponse {
		t.Fatalf("expected response search target, got %v", model.searchTarget)
	}
	if model.historyFilterActive {
		t.Fatal("did not expect history filter to open for Shift+F")
	}
	if got := model.searchInput.Value(); got != "" {
		t.Fatalf("expected trigger key to be consumed, got search input %q", got)
	}
}

func TestQuestionMarkStillOpensHelp(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = updated.(Model)

	if !model.showHelp {
		t.Fatal("expected question mark to open help")
	}
	if model.showSearchPrompt {
		t.Fatal("did not expect question mark to open search while help binding is active")
	}
}
