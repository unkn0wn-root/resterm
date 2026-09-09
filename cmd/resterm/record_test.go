package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

type recordReadyWriter struct {
	mu sync.Mutex
	bytes.Buffer
	ready chan string
}

func (w *recordReadyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.Buffer.Write(p)
	if strings.HasPrefix(string(p), "Recorder listening on ") {
		fields := strings.Fields(string(p))
		w.ready <- fields[3]
	}
	return n, err
}

func TestRecordCLI(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer up.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	path := filepath.Join(t.TempDir(), "capture.http")
	out := &recordReadyWriter{ready: make(chan string, 1)}
	var errOut bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runRecord(ctx, []string{"--listen", "127.0.0.1:0", "--upstream", up.URL, "--out", path, "--mode", "both"}, out, &errOut)
	}()
	var addr string
	select {
	case addr = <-out.ready:
	case err := <-done:
		t.Fatalf("startup: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("startup timeout")
	}
	r, err := http.Get(addr + "/users")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("record: %v; %s", err, errOut.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown timeout")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := parser.Parse(path, data)
	if parser.Check(doc) != nil || len(doc.Requests) != 1 || len(doc.Mocks) != 1 {
		t.Fatalf("invalid output: %s", data)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected exclusions: %s", errOut.String())
	}
}

func TestRecordUsageErrorsAndHelp(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--upstream", "https://example.com"}, {"--upstream", "https://user:password@example.com", "--out", "capture.http"},
		{"--upstream", "https://example.com/api", "--out", "capture.http"},
		{"--upstream", "https://example.com", "--out", "capture.http", "--body-limit", "0"},
		{"--upstream", "https://example.com", "--out", "capture.http", "--mode", "invalid"},
		{"--upstream", "https://example.com", "--out", "capture.http", "--max-entries", "0"},
	} {
		var output bytes.Buffer
		err := runRecord(t.Context(), args, &output, &output)
		if cli.ExitCode(err) != 2 {
			t.Fatalf("args %v: %v", args, err)
		}
		if err != nil && strings.Contains(err.Error(), "password") {
			t.Fatal("usage error leaked upstream credentials")
		}
	}
	var help bytes.Buffer
	if err := runRecord(t.Context(), []string{"--help"}, &help, &help); err != nil {
		t.Fatal(err)
	}
	if help.Len() == 0 {
		t.Fatal("help is empty")
	}
}
