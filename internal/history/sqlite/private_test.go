package sqlite

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix file modes")
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", filepath.Base(path), err)
	}
	if got := st.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %v, want %v", filepath.Base(path), got, want)
	}
}

func TestStoreKeepsFilesPrivate(t *testing.T) {
	skipOnWindows(t)
	dir := filepath.Join(t.TempDir(), "state")
	p := filepath.Join(dir, "history.db")
	s := New(p)
	defer func() { _ = s.Close() }()
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}
	assertMode(t, dir, 0o700)
	assertMode(t, p, 0o600)
	assertMode(t, p+"-wal", 0o600)
	assertMode(t, p+"-shm", 0o600)
}

func TestStoreTightensExistingFile(t *testing.T) {
	skipOnWindows(t)
	p := filepath.Join(t.TempDir(), "history.db")
	s := New(p)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	s = New(p)
	defer func() { _ = s.Close() }()
	if err := s.Load(); err != nil {
		t.Fatalf("load again: %v", err)
	}
	assertMode(t, p, 0o600)
}

func TestRecoveredStoreIsPrivate(t *testing.T) {
	skipOnWindows(t)
	p := filepath.Join(t.TempDir(), "history.db")
	if err := os.WriteFile(p, []byte("not-a-sqlite-db"), 0o644); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}
	s := New(p)
	defer func() { _ = s.Close() }()
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	rec := s.RecoveryInfo()
	if rec == nil {
		t.Fatalf("expected recovery")
	}
	assertMode(t, p, 0o600)
	assertMode(t, rec.Backup, 0o600)
}

func TestExportJSONIsPrivate(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.db"))
	defer func() { _ = s.Close() }()
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}
	out := filepath.Join(dir, "history.json")
	if _, err := s.ExportJSON(out); err != nil {
		t.Fatalf("export: %v", err)
	}
	assertMode(t, out, 0o600)
}

func TestBackupIsPrivateAndReplacesTarget(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	s := New(filepath.Join(dir, "history.db"))
	defer func() { _ = s.Close() }()
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}
	out := filepath.Join(dir, "history.bak.db")
	if err := os.WriteFile(out, []byte("old backup"), 0o644); err != nil {
		t.Fatalf("write old backup: %v", err)
	}
	if err := s.Backup(out); err != nil {
		t.Fatalf("backup: %v", err)
	}
	assertMode(t, out, 0o600)
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range es {
		if e.IsDir() {
			t.Fatalf("backup left %s behind", e.Name())
		}
	}
}

func TestStoreKeepsReadOnlyMode(t *testing.T) {
	skipOnWindows(t)
	p := filepath.Join(t.TempDir(), "history.db")
	s := New(p)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := os.Chmod(p, 0o444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	s = New(p)
	defer func() { _ = s.Close() }()
	_ = s.Load()
	assertMode(t, p, 0o400)
}

func TestStoreLeavesDirectoryAlone(t *testing.T) {
	skipOnWindows(t)
	p := filepath.Join(t.TempDir(), "history.db")
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	s := New(p)
	defer func() { _ = s.Close() }()
	if err := s.Load(); err == nil {
		t.Fatal("load opened a directory")
	}
	assertMode(t, p, 0o755)
}
