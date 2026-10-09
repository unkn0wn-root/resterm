package sqlite

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
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

	s := New(p)
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
	es, err := s.Entries()
	if err != nil {
		t.Fatalf("entries after recover: %v", err)
	}
	if n := len(es); n != 1 {
		t.Fatalf("expected 1 row after append, got %d", n)
	}
}

func TestLoadRecoversFromFailedQuickCheck(t *testing.T) {
	p := filepath.Join(t.TempDir(), "history.db")
	s := New(p)
	for i := range 300 {
		e := history.Entry{ID: strconv.Itoa(i), ExecutedAt: time.Now(), URL: "https://example.com/" + strconv.Itoa(i)}
		if err := s.Append(e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	const pageSize = 4096
	for page := 2; page < len(raw)/pageSize; page++ {
		bad := append([]byte(nil), raw...)
		for i := page*pageSize + 100; i < page*pageSize+300; i++ {
			bad[i] ^= 0x5a
		}
		q := filepath.Join(t.TempDir(), "history.db")
		if err := os.WriteFile(q, bad, 0o600); err != nil {
			t.Fatalf("write corrupt db: %v", err)
		}
		db, err := openReadyDB(q)
		if err == nil {
			_ = db.Close()
		}
		var ie *integrityCheckError
		if !errors.As(err, &ie) {
			continue
		}
		if ie.Result == "" {
			t.Fatalf("quick_check failure has no detail")
		}
		got := New(q)
		if err := got.Load(); err != nil {
			t.Fatalf("load: %v", err)
		}
		defer func() { _ = got.Close() }()
		if got.RecoveryInfo() == nil {
			t.Fatalf("expected recovery after quick_check failure")
		}
		return
	}
	t.Fatal("no damaged page made quick_check fail")
}
