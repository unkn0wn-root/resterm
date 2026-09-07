package recorder

import (
	"cmp"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func startTest(t *testing.T, upstream string, configure func(*Config)) *Session {
	t.Helper()
	c := DefaultConfig()
	c.Listen = "127.0.0.1:0"
	c.Upstream = upstream
	if configure != nil {
		configure(&c)
	}
	s, err := Start(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		_ = s.Close(ctx)
	})
	return s
}

func entriesAfter(t *testing.T, s *Session, n int) []Entry {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		entries := s.Snapshot(0)
		if len(entries) >= n {
			return entries
		}
		select {
		case <-s.Updates():
		case <-ctx.Done():
			t.Fatalf("recordings: got %d, want %d; stats %+v", len(entries), n, s.Stats())
		}
	}
}

func textEntry(id uint64, path, body string) Entry {
	return Entry{
		ID: id, Method: "POST", URL: "http://example.invalid" + path, Status: 200,
		Request:  Message{Headers: http.Header{"Content-Type": {"text/plain"}}, Body: []byte(body)},
		Response: Message{Headers: http.Header{"Content-Type": {"text/plain"}}, Body: []byte(body)},
	}
}

func buildExport(t *testing.T, entries []Entry, opts ExportOptions) *Plan {
	t.Helper()
	if opts.Path == "" {
		opts.Path = filepath.Join(t.TempDir(), "rec.http")
	}
	opts.Upstream = cmp.Or(opts.Upstream, "http://example.invalid")
	p, err := Build(entries, opts)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func newTestOutput(t *testing.T) *Output {
	t.Helper()
	out, err := CreateOutput(filepath.Join(t.TempDir(), "rec.http"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := out.Close(); err != nil {
			t.Error(err)
		}
	})
	return out
}

func readTestOutput(t *testing.T, out *Output) *restfile.Document {
	t.Helper()
	source, err := os.ReadFile(out.path)
	if err != nil {
		t.Fatal(err)
	}
	doc := parser.Parse(out.path, source)
	if err := parser.Check(doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
