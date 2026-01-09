package history

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreByFileFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	store := NewStore(path, 10)

	fileA := filepath.Join(dir, "a.http")
	fileB := filepath.Join(dir, "b.http")

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(2 * time.Minute)

	if err := store.Append(Entry{ID: "1", ExecutedAt: t1, FilePath: fileA}); err != nil {
		t.Fatalf("append entry 1: %v", err)
	}
	if err := store.Append(Entry{ID: "2", ExecutedAt: t2, FilePath: fileA}); err != nil {
		t.Fatalf("append entry 2: %v", err)
	}
	if err := store.Append(Entry{ID: "3", ExecutedAt: t1, FilePath: fileB}); err != nil {
		t.Fatalf("append entry 3: %v", err)
	}

	got := store.ByFile(filepath.Join(dir, ".", "a.http"))
	if len(got) != 2 {
		t.Fatalf("expected 2 entries for file A, got %d", len(got))
	}
	if got[0].ID != "2" || got[1].ID != "1" {
		t.Fatalf("expected newest-first order, got %q then %q", got[0].ID, got[1].ID)
	}

	if len(store.ByFile("")) != 0 {
		t.Fatalf("expected empty result for blank path")
	}
}
