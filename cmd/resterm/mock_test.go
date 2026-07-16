package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func writeMockFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
