package sqlite

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestByFileFiltersAndSorts(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	s := New(p)

	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	fa := filepath.Join(dir, "a.http")
	fb := filepath.Join(dir, "b.http")
	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(2 * time.Minute)

	if err := s.Append(history.Entry{ID: "1", ExecutedAt: t1, FilePath: fa}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := s.Append(history.Entry{ID: "2", ExecutedAt: t2, FilePath: fa}); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	if err := s.Append(history.Entry{ID: "3", ExecutedAt: t1, FilePath: fb}); err != nil {
		t.Fatalf("append 3: %v", err)
	}

	got, err := s.ByFile(filepath.Join(dir, ".", "a.http"))
	if err != nil {
		t.Fatalf("by file: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0].ID != "2" || got[1].ID != "1" {
		t.Fatalf("expected 2 then 1, got %q then %q", got[0].ID, got[1].ID)
	}
	empty, err := s.ByFile("")
	if err != nil {
		t.Fatalf("by file blank: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty result for blank file path")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	s := New(p)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now()}); err != nil {
		t.Fatalf("append: %v", err)
	}

	ok, err := s.Delete("1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !ok {
		t.Fatalf("expected delete to return true")
	}
	got, err := s.Entries()
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty rows after delete")
	}
}

func TestByRequestSkipsWorkflowRows(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	s := New(p)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	t3 := t2.Add(1 * time.Minute)

	_ = s.Append(history.Entry{ID: "1", ExecutedAt: t1, Method: "GET", RequestName: "alpha"})
	_ = s.Append(history.Entry{ID: "2", ExecutedAt: t2, Method: "POST", URL: "https://alpha.test"})
	_ = s.Append(history.Entry{
		ID:          "3",
		ExecutedAt:  t3,
		Method:      restfile.HistoryMethodWorkflow,
		RequestName: "alpha",
	})

	got, err := s.ByRequest("alpha")
	if err != nil {
		t.Fatalf("by request alpha: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].ID != "1" {
		t.Fatalf("expected ID 1, got %q", got[0].ID)
	}

	got, err = s.ByRequest("https://alpha.test")
	if err != nil {
		t.Fatalf("by request url: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].ID != "2" {
		t.Fatalf("expected ID 2, got %q", got[0].ID)
	}
}

func TestByWorkflowCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	s := New(p)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)

	_ = s.Append(history.Entry{
		ID:          "1",
		ExecutedAt:  t1,
		Method:      restfile.HistoryMethodWorkflow,
		RequestName: " Deploy ",
	})
	_ = s.Append(history.Entry{
		ID:          "2",
		ExecutedAt:  t2,
		Method:      "GET",
		RequestName: "deploy",
	})

	got, err := s.ByWorkflow("deploy")
	if err != nil {
		t.Fatalf("by workflow: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].ID != "1" {
		t.Fatalf("expected workflow ID 1, got %q", got[0].ID)
	}
}

func TestAppendConcurrent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	s := New(p)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}

	const workers = 12
	const perWorker = 60

	var n atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(w int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				id := n.Add(1)
				e := history.Entry{
					ID:         fmt.Sprintf("%d", id),
					ExecutedAt: time.Now().Add(time.Duration(id) * time.Nanosecond),
					Method:     "GET",
					URL:        fmt.Sprintf("https://svc/%d/%d", w, j),
				}
				if err := s.Append(e); err != nil {
					t.Errorf("append failed: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	got, err := s.Entries()
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	want := workers * perWorker
	if len(got) != want {
		t.Fatalf("expected %d rows, got %d", want, len(got))
	}
}

func TestByRequestBlankReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	s := New(p)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := s.Append(history.Entry{ID: "1", ExecutedAt: time.Now(), Method: "GET"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := s.ByRequest("")
	if err != nil {
		t.Fatalf("by request blank: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty rows for blank request id")
	}
}

func TestEntriesReturnsErrorOnQueryFailure(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "history.db")
	s := New(p)
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := s.db.Exec(`DROP TABLE hist`); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	if _, err := s.Entries(); err == nil {
		t.Fatalf("expected query error")
	}
}
