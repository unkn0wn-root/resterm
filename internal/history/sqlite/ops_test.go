package sqlite

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestStatsCheckCompact(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")

	s := New(p)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)
	_ = s.Append(history.Entry{ID: "1", ExecutedAt: t1, Method: "GET"})
	_ = s.Append(history.Entry{ID: "2", ExecutedAt: t2, Method: "POST"})

	st, err := s.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if st.Schema != schemaVer {
		t.Fatalf("expected schema v%d, got %d", schemaVer, st.Schema)
	}
	if st.Rows != 2 {
		t.Fatalf("expected 2 rows, got %d", st.Rows)
	}
	if st.Oldest.IsZero() || st.Newest.IsZero() {
		t.Fatalf("expected oldest/newest timestamps")
	}
	if st.DBBytes <= 0 {
		t.Fatalf("expected non-zero db size")
	}

	if err := s.Check(false); err != nil {
		t.Fatalf("quick check: %v", err)
	}
	if err := s.Check(true); err != nil {
		t.Fatalf("full check: %v", err)
	}
	if err := s.Compact(); err != nil {
		t.Fatalf("compact: %v", err)
	}
}

func TestCompactShrinksFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "history.db")
	s := New(p)
	body := strings.Repeat("x", 4096)
	for i := range 200 {
		if err := s.Append(history.Entry{ID: strconv.Itoa(i), ExecutedAt: time.Now(), BodySnippet: body}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	for i := range 190 {
		if _, err := s.Delete(strconv.Itoa(i)); err != nil {
			t.Fatalf("delete: %v", err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	s = New(p)
	defer func() { _ = s.Close() }()
	before, err := s.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if err := s.Compact(); err != nil {
		t.Fatalf("compact: %v", err)
	}
	after, err := s.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if after.DBBytes >= before.DBBytes/2 || after.WALBytes != 0 {
		t.Fatalf("after compact db=%d wal=%d, want db well below %d and no wal",
			after.DBBytes, after.WALBytes, before.DBBytes)
	}
}

func TestCompactFailsWhenCheckpointIsBlocked(t *testing.T) {
	p := filepath.Join(t.TempDir(), "history.db")
	s := New(p)
	defer func() { _ = s.Close() }()
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}
	db, err := s.handle()
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	// Applies to the store's only connection. Without it the checkpoint waits 5s before giving up.
	if _, err := db.Exec(`PRAGMA busy_timeout=0;`); err != nil {
		t.Fatalf("busy timeout: %v", err)
	}
	r, err := sql.Open(drv, p)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	defer func() { _ = r.Close() }()
	tx, err := r.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM hist`).Scan(&n); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := s.Compact(); err == nil {
		t.Fatal("compact succeeded while a reader blocked the checkpoint")
	}
}
