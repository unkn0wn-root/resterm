package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/mock"
)

func TestServeMocksStartsAndStopsWithContext(t *testing.T) {
	file := filepath.Join(t.TempDir(), "api.http")
	writeMockFile(t, file, `# @mock method=GET path=/value default=true
HTTP/1.1 200 OK

ok`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := defaultMockConfig()
	cfg.path = file
	cfg.addr = "127.0.0.1:0"
	cfg.cors = "off"
	cfg.watch = false
	var out, errOut bytes.Buffer
	err := serveMocks(ctx, cfg, &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Mock server listening") {
		t.Fatalf("stdout:\n%s", out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr:\n%s", errOut.String())
	}
}

func TestServeMocksRequiresTLSPair(t *testing.T) {
	var out, errOut bytes.Buffer
	err := serveMocks(context.Background(), mockConfig{
		path:    ".",
		addr:    "127.0.0.1:0",
		cors:    "off",
		tlsCert: "cert.pem",
	}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("err = %v, want the TLS pair error", err)
	}
}

func TestServeMocksValidatesJournalLimitsAsUsageErrors(t *testing.T) {
	cfg := defaultMockConfig()
	cfg.path = "."
	cfg.addr = "127.0.0.1:0"
	cfg.cors = "off"
	cfg.journalBytes = "1KiB"
	cfg.journalBodyLimit = "2KiB"
	var out, errOut bytes.Buffer
	err := serveMocks(context.Background(), cfg, &out, &errOut)
	if cli.ExitCode(err) != 2 || !strings.Contains(err.Error(), "must not exceed") {
		t.Fatalf("err = %v, code = %d", err, cli.ExitCode(err))
	}
}

func TestPrintMockEventIncludesSequenceProgress(t *testing.T) {
	var output bytes.Buffer
	printMockEvent(log.New(&output, "", 0), mock.Event{
		Method:        "GET",
		Target:        "/payments/1",
		Status:        200,
		Scenario:      "polling",
		SequenceStep:  2,
		SequenceTotal: 3,
		Duration:      time.Millisecond,
	})
	if got := output.String(); !strings.Contains(got, "[polling 2/3]") {
		t.Fatalf("event output = %q", got)
	}
}

func TestMockControlCommandsResetClearAndVerify(t *testing.T) {
	file := filepath.Join(t.TempDir(), "payments.http")
	writeMockFile(t, file, `### Poll
# @mock method=GET path=/payments sequence=polling
# @expect calls=1
HTTP/1.1 503 Service Unavailable

pending
---
HTTP/1.1 200 OK

done`)
	handler, err := mock.Load(file, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	server, err := mock.Start("127.0.0.1:0", handler, mock.Options{EnableControl: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	url := "http://" + server.Addr()
	call := func() int {
		t.Helper()
		response, err := http.Get(url + "/payments")
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	if got := call(); got != http.StatusServiceUnavailable {
		t.Fatalf("first status = %d", got)
	}

	var out, errOut bytes.Buffer
	if err := runMockVerify([]string{"--url", url, file}, &out, &errOut); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(out.String(), "PASS") {
		t.Fatalf("verify output = %q", out.String())
	}

	out.Reset()
	if err := runMockReset([]string{"--url", url, "polling"}, &out, &errOut); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !strings.Contains(out.String(), "Reset 1") {
		t.Fatalf("reset output = %q", out.String())
	}
	if got := call(); got != http.StatusServiceUnavailable {
		t.Fatalf("status after reset = %d", got)
	}

	out.Reset()
	err = runMockVerify([]string{"--url", url, file}, &out, &errOut)
	if cli.ExitCode(err) != 1 || !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("mismatched verify err=%v output=%q", err, out.String())
	}

	out.Reset()
	if err := runMockClear([]string{"--url", url}, &out, &errOut); err != nil {
		t.Fatalf("clear: %v", err)
	}
	count, err := server.Count(context.Background(), mock.RequestPattern{})
	if err != nil || count != 0 || len(server.Logs()) != 0 {
		t.Fatalf("after clear count=%d err=%v logs=%d", count, err, len(server.Logs()))
	}
}

func writeMockFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
