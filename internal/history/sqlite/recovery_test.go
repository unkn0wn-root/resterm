package sqlite

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestLoadRecoversCorruptDB(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	raw := []byte("not-a-sqlite-db")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}

	s := New(p, 10)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	rec := s.RecoveryInfo()
	if rec == nil {
		t.Fatalf("expected recovery info")
	}
	if rec.Path != p {
		t.Fatalf("expected recovery path %s, got %s", p, rec.Path)
	}
	if rec.Backup == "" {
		t.Fatalf("expected recovery backup path")
	}
	got, err := os.ReadFile(rec.Backup)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("backup content mismatch")
	}

	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now(), Method: "GET"}); err != nil {
		t.Fatalf("append after recover: %v", err)
	}
	if n := len(s.Entries()); n != 1 {
		t.Fatalf("expected 1 row after append, got %d", n)
	}
}
