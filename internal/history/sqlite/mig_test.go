package sqlite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestMigrateJSONImportsOnce(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "history.db")
	jsonPath := filepath.Join(dir, "history.json")

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	src := []history.Entry{
		{ID: "1", ExecutedAt: t1, Method: "GET", URL: "https://one.test"},
		{ID: "2", ExecutedAt: t2, Method: "POST", URL: "https://two.test"},
	}
	writeLegacyJSON(t, jsonPath, src)

	s := New(dbPath, 10)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	n, err := s.MigrateJSON(jsonPath)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 imported rows, got %d", n)
	}
	if got := s.Entries(); len(got) != 2 {
		t.Fatalf("expected 2 rows after import, got %d", len(got))
	}

	n, err = s.MigrateJSON(jsonPath)
	if err != nil {
		t.Fatalf("migrate second: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 imported rows on second run, got %d", n)
	}
	if got := s.Entries(); len(got) != 2 {
		t.Fatalf("expected 2 rows after second import, got %d", len(got))
	}
}

func TestMigrateJSONSkipsWhenDBHasRows(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "history.db")
	jsonPath := filepath.Join(dir, "history.json")
	writeLegacyJSON(t, jsonPath, []history.Entry{
		{ID: "2", ExecutedAt: time.Now(), Method: "POST", URL: "https://two.test"},
	})

	s := New(dbPath, 10)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now(), Method: "GET"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	n, err := s.MigrateJSON(jsonPath)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 imported rows when DB not empty, got %d", n)
	}
	if got := s.Entries(); len(got) != 1 {
		t.Fatalf("expected original rows only, got %d", len(got))
	}
}

func TestMigrateJSONTrimmedByMax(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "history.db")
	jsonPath := filepath.Join(dir, "history.json")

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	t3 := t2.Add(1 * time.Minute)
	writeLegacyJSON(t, jsonPath, []history.Entry{
		{ID: "1", ExecutedAt: t1},
		{ID: "2", ExecutedAt: t2},
		{ID: "3", ExecutedAt: t3},
	})

	s := New(dbPath, 2)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	n, err := s.MigrateJSON(jsonPath)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 inserted before trim, got %d", n)
	}
	got := s.Entries()
	if len(got) != 2 {
		t.Fatalf("expected 2 rows after trim, got %d", len(got))
	}
	if got[0].ID != "3" || got[1].ID != "2" {
		t.Fatalf("expected retained rows 3,2 got %q,%q", got[0].ID, got[1].ID)
	}
}

func TestMigrateJSONMissingFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "history.db")
	jsonPath := filepath.Join(dir, "missing.json")

	s := New(dbPath, 10)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	n, err := s.MigrateJSON(jsonPath)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 imported rows for missing file, got %d", n)
	}
}

func TestMigrateJSONSkipsLegacyReadAfterDone(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "history.db")
	jsonPath := filepath.Join(dir, "history.json")
	writeLegacyJSON(t, jsonPath, []history.Entry{
		{ID: "1", ExecutedAt: time.Now(), Method: "GET", URL: "https://one.test"},
	})

	s := New(dbPath, 10)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	n, err := s.MigrateJSON(jsonPath)
	if err != nil {
		t.Fatalf("migrate first: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 imported row, got %d", n)
	}

	if err := os.Remove(jsonPath); err != nil {
		t.Fatalf("remove legacy file: %v", err)
	}
	if err := os.Mkdir(jsonPath, 0o755); err != nil {
		t.Fatalf("replace legacy file with dir: %v", err)
	}

	n, err = s.MigrateJSON(jsonPath)
	if err != nil {
		t.Fatalf("migrate second: expected marker short-circuit, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 imported rows on second run, got %d", n)
	}
}

func TestMigrateJSONInvalidData(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "history.db")
	jsonPath := filepath.Join(dir, "history.json")
	if err := os.WriteFile(jsonPath, []byte("{bad-json"), 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	s := New(dbPath, 10)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	if _, err := s.MigrateJSON(jsonPath); err == nil {
		t.Fatalf("expected migrate parse error")
	}
	if got := s.Entries(); len(got) != 0 {
		t.Fatalf("expected no rows after failed migration, got %d", len(got))
	}
}

func writeLegacyJSON(t *testing.T, path string, es []history.Entry) {
	t.Helper()
	data, err := json.Marshal(es)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
}
