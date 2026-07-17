package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/mock"
)

func TestServeMocksStartsAndStopsWithContext(t *testing.T) {
	file := filepath.Join(t.TempDir(), "api.http")
	writeMockFile(t, file, `# @mock method=GET path=/value default=true
HTTP/1.1 200 OK

ok`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errOut bytes.Buffer
	err := serveMocks(ctx, mockConfig{
		path:  file,
		addr:  "127.0.0.1:0",
		cors:  "off",
		watch: false,
	}, &out, &errOut)
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

func writeMockFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
