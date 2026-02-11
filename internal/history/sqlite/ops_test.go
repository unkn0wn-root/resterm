package sqlite

import (
	"path/filepath"
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
