package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseDigest(t *testing.T) {
	sum := sha256.Sum256([]byte("x"))
	h := hex.EncodeToString(sum[:])

	got, err := parseDigest("sha256:" + h)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if got != sum {
		t.Fatalf("digest mismatch: got %x want %x", got, sum)
	}

	if _, err := parseDigest(""); !errors.Is(err, ErrNoDigest) {
		t.Fatalf("expected ErrNoDigest, got %v", err)
	}

	invalid := []struct {
		name string
		v    string
	}{
		{"wrong algorithm", "sha512:" + h},
		{"no prefix", h},
		{"short", "sha256:" + h[:63]},
		{"long", "sha256:" + h + "ab"},
		{"non-hex", "sha256:" + strings.Repeat("z", 64)},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseDigest(tc.v); err == nil {
				t.Fatal("expected parse error")
			}
		})
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
