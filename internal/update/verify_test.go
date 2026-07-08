package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseChecksum(t *testing.T) {
	const bin = "resterm_Linux_x86_64"
	sum := sha256.Sum256([]byte("x"))
	h := hex.EncodeToString(sum[:])

	valid := []struct {
		name string
		body string
	}{
		{"hash only", h + "\n"},
		{"no newline", h},
		{"gnu format", h + "  " + bin + "\n"},
		{"binary mode", h + " *" + bin + "\n"},
		{"crlf", h + "  " + bin + "\r\n"},
		{"uppercase", strings.ToUpper(h) + "\n"},
		{"extra lines", h + "\ngarbage\n"},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseChecksum(strings.NewReader(tc.body), bin)
			if err != nil {
				t.Fatalf("parse err: %v", err)
			}
			if got != sum {
				t.Fatalf("digest mismatch: got %x want %x", got, sum)
			}
		})
	}

	invalid := []struct {
		name string
		body string
	}{
		{"wrong filename", h + "  resterm\n"},
		{"short token", h[:63] + "\n"},
		{"long token", h + "ab\n"},
		{"non-hex", strings.Repeat("z", 64) + "\n"},
		{"empty", ""},
		{"blank first line", "\n" + h + "\n"},
		{"three fields", h + " a b\n"},
		{"oversized line", strings.Repeat("a", 8<<10)},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseChecksum(strings.NewReader(tc.body), bin); err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestFetchChecksum(t *testing.T) {
	const bin = "resterm_Linux_x86_64"
	sum := sha256.Sum256([]byte("x"))
	body := hex.EncodeToString(sum[:]) + "  " + bin + "\n"

	tr := stubTransport{res: map[string]stubResponse{
		"https://mock/sum": {body: body},
	}}
	cl, err := NewClient(&http.Client{Transport: tr}, "unkn0wn-root/resterm")
	if err != nil {
		t.Fatalf("client err: %v", err)
	}

	got, err := cl.fetchChecksum(context.Background(), Asset{Name: bin + ".sha256", URL: "https://mock/sum"}, bin)
	if err != nil {
		t.Fatalf("fetch err: %v", err)
	}
	if got != sum {
		t.Fatalf("digest mismatch: got %x want %x", got, sum)
	}

	missing := Asset{Name: bin + ".sha256", URL: "https://mock/missing"}
	if _, err := cl.fetchChecksum(context.Background(), missing, bin); err == nil {
		t.Fatal("expected fetch error")
	}
}

func TestVerifyVersionMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "resterm-check")
	body := "#!/bin/sh\necho \"resterm v1.0.0\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	err := verifyVersion(context.Background(), path, "v2.0.0")
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
}
