package ui

import (
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestDeleteHistoryEntryRemovesFromStore(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	if err := store.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	entry := history.Entry{
		ID:         "1",
		ExecutedAt: time.Now(),
		Method:     "GET",
		URL:        "https://example.com",
	}
	if err := store.Append(entry); err != nil {
		t.Fatalf("append: %v", err)
	}

	model := New(Config{History: store})
	model.ready = true
	model.width = 120
	model.height = 40
	model.focus = focusResponse
	model.responsePanes[0].activeTab = responseTabHistory
	model.syncHistory()
	if len(model.historyEntries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(model.historyEntries))
	}
	model.historyList.Select(0)

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}
	updated, _ := model.Update(msg)
	model = updated.(Model)

	entries := store.Entries()
	if len(entries) != 0 {
		t.Fatalf("expected store to be empty, got %d entries", len(entries))
	}
	if len(model.historyEntries) != 0 {
		t.Fatalf("expected model history to be empty, got %d", len(model.historyEntries))
	}
	if model.statusMessage.text != "History entry deleted" {
		t.Fatalf("expected delete status message, got %q", model.statusMessage.text)
	}
}
