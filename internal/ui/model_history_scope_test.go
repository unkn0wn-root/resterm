package ui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestSyncHistoryScopeRequest(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	model := New(Config{History: store})

	reqA := &restfile.Request{
		Metadata: restfile.RequestMetadata{Name: "alpha"},
		URL:      "https://alpha.test",
	}
	model.currentRequest = reqA
	model.historyScope = historyScopeRequest

	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	if err := store.Append(
		history.Entry{ID: "1", ExecutedAt: t1, RequestName: "alpha"},
	); err != nil {
		t.Fatalf("append alpha: %v", err)
	}
	if err := store.Append(
		history.Entry{ID: "2", ExecutedAt: t1, RequestName: "beta"},
	); err != nil {
		t.Fatalf("append beta: %v", err)
	}

	model.syncHistory()
	if len(model.historyEntries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(model.historyEntries))
	}
	if model.historyEntries[0].RequestName != "alpha" {
		t.Fatalf("expected alpha entry, got %q", model.historyEntries[0].RequestName)
	}
}

func TestSyncHistoryScopeFile(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	model := New(Config{History: store})

	fileA := filepath.Join(dir, "a.http")
	fileB := filepath.Join(dir, "b.http")
	model.currentFile = fileA
	model.historyScope = historyScopeFile

	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	if err := store.Append(history.Entry{ID: "1", ExecutedAt: t1, FilePath: fileA}); err != nil {
		t.Fatalf("append file A: %v", err)
	}
	if err := store.Append(history.Entry{ID: "2", ExecutedAt: t1, FilePath: fileB}); err != nil {
		t.Fatalf("append file B: %v", err)
	}

	model.syncHistory()
	if len(model.historyEntries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(model.historyEntries))
	}
	if model.historyEntries[0].FilePath != fileA {
		t.Fatalf("expected file A entry, got %q", model.historyEntries[0].FilePath)
	}
}

func TestSyncHistoryScopeFileMatchesLegacyEntries(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	model := New(Config{History: store})

	fileA := filepath.Join(dir, "api", "a.http")
	model.currentFile = fileA
	model.workspaceRoot = dir
	model.doc = &restfile.Document{
		Requests: []*restfile.Request{
			{Metadata: restfile.RequestMetadata{Name: "alpha"}, URL: "https://alpha.test"},
		},
	}
	model.historyScope = historyScopeFile

	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	if err := store.Append(history.Entry{ID: "1", ExecutedAt: t1, FilePath: fileA}); err != nil {
		t.Fatalf("append file A: %v", err)
	}
	if err := store.Append(
		history.Entry{ID: "2", ExecutedAt: t1, FilePath: filepath.Join("api", "a.http")},
	); err != nil {
		t.Fatalf("append relative file A: %v", err)
	}
	if err := store.Append(
		history.Entry{ID: "3", ExecutedAt: t1, RequestName: "alpha"},
	); err != nil {
		t.Fatalf("append legacy alpha: %v", err)
	}
	if err := store.Append(
		history.Entry{ID: "4", ExecutedAt: t1, RequestName: "beta"},
	); err != nil {
		t.Fatalf("append legacy beta: %v", err)
	}

	model.syncHistory()
	if len(model.historyEntries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(model.historyEntries))
	}
	got := map[string]struct{}{}
	for _, entry := range model.historyEntries {
		got[entry.ID] = struct{}{}
	}
	for _, id := range []string{"1", "2", "3"} {
		if _, ok := got[id]; !ok {
			t.Fatalf("expected entry %q in file scope", id)
		}
	}
	if _, ok := got["4"]; ok {
		t.Fatalf("did not expect legacy beta entry to match file scope")
	}
}

func TestSyncHistorySortOrder(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	model := New(Config{History: store})

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Hour)
	if err := store.Append(history.Entry{ID: "1", ExecutedAt: t1}); err != nil {
		t.Fatalf("append older: %v", err)
	}
	if err := store.Append(history.Entry{ID: "2", ExecutedAt: t2}); err != nil {
		t.Fatalf("append newer: %v", err)
	}

	model.historyScope = historyScopeGlobal
	model.historySort = historySortNewest
	model.syncHistory()
	if len(model.historyEntries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(model.historyEntries))
	}
	if model.historyEntries[0].ID != "2" {
		t.Fatalf("expected newest first, got %q", model.historyEntries[0].ID)
	}

	model.historySort = historySortOldest
	model.syncHistory()
	if model.historyEntries[0].ID != "1" {
		t.Fatalf("expected oldest first, got %q", model.historyEntries[0].ID)
	}
}
