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

func TestApplyPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}

	body := "#!/bin/sh\necho \"resterm v1.1.0\"\n"
	sum := sha256.Sum256([]byte(body))
	sumLine := hex.EncodeToString(sum[:]) + "  resterm\n"

	tr := stubTransport{res: map[string]stubResponse{
		"https://mock/bin": {body: body},
		"https://mock/sum": {body: sumLine},
	}}

	cl, err := NewClient(&http.Client{Transport: tr}, "unkn0wn-root/resterm")
	if err != nil {
		t.Fatalf("client err: %v", err)
	}

	res := Result{
		Info: Info{Version: "v1.1.0"},
		Bin: Asset{
			Name: "resterm_Linux_x86_64",
			URL:  "https://mock/bin",
			Size: int64(len(body)),
		},
		Sum:    Asset{Name: "resterm_Linux_x86_64.sha256", URL: "https://mock/sum"},
		HasSum: true,
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "resterm")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	st, err := Apply(context.Background(), cl, res, exe)
	if err != nil {
		t.Fatalf("apply err: %v", err)
	}
	if st.Pending {
		t.Fatal("unexpected pending flag")
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(body) {
		t.Fatalf("unexpected binary content: %q", string(got))
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
