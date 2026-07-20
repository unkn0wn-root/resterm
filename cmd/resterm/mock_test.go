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

func TestDefaultMockConfigUsesPackageLimits(t *testing.T) {
	cfg := defaultMockConfig()
	total, body, err := cfg.parseLimits()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.sequenceKeyLimit; got != mock.DefaultSequenceKeyLimit {
		t.Fatalf("sequence key limit = %d, want %d", got, mock.DefaultSequenceKeyLimit)
	}
	if got := cfg.journalEntries; got != mock.DefaultJournalEntries {
		t.Fatalf("journal entries = %d, want %d", got, mock.DefaultJournalEntries)
	}
	if total != mock.DefaultJournalBytes {
		t.Fatalf("journal bytes = %d, want %d", total, mock.DefaultJournalBytes)
	}
	if body != mock.DefaultJournalBodyLimit {
		t.Fatalf("journal body limit = %d, want %d", body, mock.DefaultJournalBodyLimit)
	}
}

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
	tests := []struct {
		name   string
		change func(*mockConfig)
		want   string
	}{
		{
			name:   "invalid journal bytes",
			change: func(cfg *mockConfig) { cfg.journalBytes = "invalid" },
			want:   "invalid --journal-bytes",
		},
		{
			name:   "zero journal bytes",
			change: func(cfg *mockConfig) { cfg.journalBytes = "0" },
			want:   "invalid --journal-bytes",
		},
		{
			name:   "invalid journal body limit",
			change: func(cfg *mockConfig) { cfg.journalBodyLimit = "invalid" },
			want:   "invalid --journal-body-limit",
		},
		{
			name: "body limit exceeds journal bytes",
			change: func(cfg *mockConfig) {
				cfg.journalBytes = "1KiB"
				cfg.journalBodyLimit = "2KiB"
			},
			want: "must not exceed",
		},
		{
			name:   "non-positive sequence key limit",
			change: func(cfg *mockConfig) { cfg.sequenceKeyLimit = 0 },
			want:   "limits must be positive",
		},
		{
			name:   "non-positive journal entry limit",
			change: func(cfg *mockConfig) { cfg.journalEntries = 0 },
			want:   "limits must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultMockConfig()
			cfg.path = "."
			cfg.addr = "127.0.0.1:0"
			cfg.cors = "off"
			tt.change(&cfg)
			var out, errOut bytes.Buffer
			err := serveMocks(context.Background(), cfg, &out, &errOut)
			if cli.ExitCode(err) != 2 || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, code = %d, want %q", err, cli.ExitCode(err), tt.want)
			}
		})
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
