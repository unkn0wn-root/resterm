package ui

import (
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestHistoryListSkipsBlockedKey(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	model := New(Config{History: store})
	model.ready = true
	model.focus = focusResponse
	model.historyScope = historyScopeGlobal

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary pane")
	}
	pane.activeTab = responseTabHistory

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Hour)
	if err := store.Append(history.Entry{ID: "1", ExecutedAt: t1}); err != nil {
		t.Fatalf("append entry 1: %v", err)
	}
	if err := store.Append(history.Entry{ID: "2", ExecutedAt: t2}); err != nil {
		t.Fatalf("append entry 2: %v", err)
	}
	model.syncHistory()
	model.historyList.Select(0)
	before := model.historyList.Index()

	model.historyBlockKey = true
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(Model)

	after := model.historyList.Index()
	if after != before {
		t.Fatalf("expected history selection to remain %d, got %d", before, after)
	}
}

func TestHistoryEscClearsFilter(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	model := New(Config{History: store})
	model.ready = true
	model.focus = focusResponse
	model.historyScope = historyScopeGlobal

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary pane")
	}
	pane.activeTab = responseTabHistory

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	if err := store.Append(history.Entry{ID: "1", ExecutedAt: t1}); err != nil {
		t.Fatalf("append entry: %v", err)
	}
	model.syncHistory()
	model.historyFilterInput.SetValue("method:get")
	model.syncHistory()

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)

	if model.historyFilterInput.Value() != "" {
		t.Fatalf("expected history filter to clear, got %q", model.historyFilterInput.Value())
	}
	if model.statusMessage.text != "History filter cleared" {
		t.Fatalf("expected status message, got %q", model.statusMessage.text)
	}
}

func TestHistoryMultiSelectDelete(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	model := New(Config{History: store})
	model.ready = true
	model.focus = focusResponse
	model.historyScope = historyScopeGlobal

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary pane")
	}
	pane.activeTab = responseTabHistory

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	t3 := t2.Add(time.Hour)
	if err := store.Append(history.Entry{ID: "1", ExecutedAt: t1}); err != nil {
		t.Fatalf("append entry 1: %v", err)
	}
	if err := store.Append(history.Entry{ID: "2", ExecutedAt: t2}); err != nil {
		t.Fatalf("append entry 2: %v", err)
	}
	if err := store.Append(history.Entry{ID: "3", ExecutedAt: t3}); err != nil {
		t.Fatalf("append entry 3: %v", err)
	}
	model.syncHistory()

	model.historyList.Select(0)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)

	model.historyList.Select(1)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(Model)

	if len(model.historySelected) != 2 {
		t.Fatalf("expected 2 selected entries, got %d", len(model.historySelected))
	}
	selected := make(map[string]struct{}, len(model.historySelected))
	for id := range model.historySelected {
		selected[id] = struct{}{}
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model = updated.(Model)

	remaining := map[string]struct{}{}
	for _, entry := range store.Entries() {
		remaining[entry.ID] = struct{}{}
	}
	if len(remaining) != 3-len(selected) {
		t.Fatalf("expected %d remaining entries, got %d", 3-len(selected), len(remaining))
	}
	for id := range selected {
		if _, ok := remaining[id]; ok {
			t.Fatalf("did not expect entry %s to remain", id)
		}
	}
	if len(model.historySelected) != 0 {
		t.Fatalf("expected selection to clear, got %d", len(model.historySelected))
	}
}
