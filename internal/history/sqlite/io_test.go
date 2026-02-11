package sqlite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestExportImportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	srcDB := filepath.Join(dir, "src.db")
	dstDB := filepath.Join(dir, "dst.db")
	out := filepath.Join(dir, "hist.json")

	src := New(srcDB)
	if err := src.Load(); err != nil {
		t.Fatalf("load src: %v", err)
	}

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	_ = src.Append(history.Entry{ID: "1", ExecutedAt: t1, Method: "GET", URL: "https://one.test"})
	_ = src.Append(history.Entry{ID: "2", ExecutedAt: t2, Method: "POST", URL: "https://two.test"})

	n, err := src.ExportJSON(out)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 exported rows, got %d", n)
	}

	dst := New(dstDB)
	if err := dst.Load(); err != nil {
		t.Fatalf("load dst: %v", err)
	}
	n, err = dst.ImportJSON(out)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 imported rows, got %d", n)
	}

	got := dst.Entries()
	if len(got) != 2 {
		t.Fatalf("expected 2 rows in dst, got %d", len(got))
	}
	if got[0].ID != "2" || got[1].ID != "1" {
		t.Fatalf("expected IDs 2,1 got %q,%q", got[0].ID, got[1].ID)
	}
}

func TestBackup(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "hist.db")
	out := filepath.Join(dir, "hist.bak.db")

	s := New(db)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := s.Backup(out); err != nil {
		t.Fatalf("backup: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Size() <= 0 {
		t.Fatalf("expected non-empty backup file")
	}

	cpy := New(out)
	if err := cpy.Load(); err != nil {
		t.Fatalf("load backup db: %v", err)
	}
	got := cpy.Entries()
	if len(got) != 1 {
		t.Fatalf("expected 1 row in backup db, got %d", len(got))
	}
	if got[0].ID != "1" {
		t.Fatalf("expected backup row ID 1, got %q", got[0].ID)
	}
}

func TestBackupSamePathRejected(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "hist.db")

	s := New(db)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Backup(db); err == nil {
		t.Fatalf("expected same-path backup error")
	}
}

func TestExportJSONKeepsTargetOnWriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod directory write bit semantics are not stable on windows")
	}

	dir := t.TempDir()
	db := filepath.Join(dir, "hist.db")
	out := filepath.Join(dir, "out.json")

	s := New(db)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now(), Method: "GET"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	const keep = "keep-this-content"
	if err := os.WriteFile(out, []byte(keep), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod readonly dir: %v", err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	if _, err := s.ExportJSON(out); err == nil {
		t.Fatalf("expected export error for readonly dir")
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output after failed export: %v", err)
	}
	if string(got) != keep {
		t.Fatalf("expected original output content to stay intact")
	}
}

func TestExportJSONValidPayload(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "hist.db")
	out := filepath.Join(dir, "out.json")

	s := New(db)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	_ = s.Append(
		history.Entry{ID: "1", ExecutedAt: time.Now(), Method: "GET", URL: "https://one.test"},
	)

	if _, err := s.ExportJSON(out); err != nil {
		t.Fatalf("export: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read exported file: %v", err)
	}
	var es []history.Entry
	if err := json.Unmarshal(data, &es); err != nil {
		t.Fatalf("decode exported json: %v", err)
	}
	if len(es) != 1 || es[0].ID != "1" {
		t.Fatalf("unexpected exported rows: %+v", es)
	}
}
