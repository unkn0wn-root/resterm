package update

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type recordProgress struct {
	started bool
	doneErr error
	done    bool
}

func (r *recordProgress) Start(int64)   { r.started = true }
func (r *recordProgress) Advance(int64) {}
func (r *recordProgress) Done(err error) {
	r.doneErr = err
	r.done = true
}

func TestApplyPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}

	body := "#!/bin/sh\necho \"resterm v1.1.0\"\n"

	tr := stubTransport{res: map[string]stubResponse{
		"https://mock/bin": {body: body},
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
		Digest: sha256.Sum256([]byte(body)),
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "resterm")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	prog := &recordProgress{}
	if err := cl.Apply(context.Background(), res, exe, prog); err != nil {
		t.Fatalf("apply err: %v", err)
	}
	if !prog.started || !prog.done || prog.doneErr != nil {
		t.Fatalf("unexpected progress state: %+v", prog)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read new: %v", err)
	}
	if strings.TrimSpace(string(got)) != strings.TrimSpace(body) {
		t.Fatalf("unexpected binary content: %q", string(got))
	}
}

func TestApplyChecksumMismatch(t *testing.T) {
	body := "#!/bin/sh\necho \"resterm v1.1.0\"\n"

	tr := stubTransport{res: map[string]stubResponse{
		"https://mock/bin": {body: body},
	}}

	cl, err := NewClient(&http.Client{Transport: tr}, "unkn0wn-root/resterm")
	if err != nil {
		t.Fatalf("client err: %v", err)
	}

	// the zero Digest never matches the downloaded body
	res := Result{
		Info: Info{Version: "v1.1.0"},
		Bin: Asset{
			Name: "resterm_Linux_x86_64",
			URL:  "https://mock/bin",
			Size: int64(len(body)),
		},
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "resterm")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	err = cl.Apply(context.Background(), res, exe, nil)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read exe: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("binary replaced despite mismatch: %q", string(got))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("leftover temp files: %d entries", len(entries))
	}
}

func TestApplySizeExceeded(t *testing.T) {
	body := "#!/bin/sh\necho \"resterm v1.1.0\"\n"

	tr := stubTransport{res: map[string]stubResponse{
		"https://mock/bin": {body: body},
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
			Size: int64(len(body)) - 1,
		},
		Digest: sha256.Sum256([]byte(body)),
	}

	dir := t.TempDir()
	exe := filepath.Join(dir, "resterm")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	prog := &recordProgress{}
	err = cl.Apply(context.Background(), res, exe, prog)
	if err == nil || !strings.Contains(err.Error(), "download size mismatch") {
		t.Fatalf("expected size mismatch, got %v", err)
	}
	if !prog.done || prog.doneErr == nil {
		t.Fatalf("progress not told about failure: %+v", prog)
	}
}

func TestSwapBinary(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "resterm")
	tmp := filepath.Join(dir, "resterm-update")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatalf("write exe: %v", err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0o755); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := os.WriteFile(exe+".new", []byte("stale"), 0o755); err != nil {
		t.Fatalf("write stale .new: %v", err)
	}

	if err := swapBinary(tmp, exe); err != nil {
		t.Fatalf("swap: %v", err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read exe: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("unexpected exe content: %q", got)
	}
	old, err := os.ReadFile(exe + ".old")
	if err != nil {
		t.Fatalf("read .old: %v", err)
	}
	if string(old) != "old" {
		t.Fatalf("unexpected .old content: %q", old)
	}
	if _, err := os.Stat(exe + ".new"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale .new not removed: %v", err)
	}
}

func TestSwapBinaryRollback(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "resterm")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatalf("write exe: %v", err)
	}

	if err := swapBinary(filepath.Join(dir, "missing"), exe); err == nil {
		t.Fatal("expected swap error")
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read exe: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("exe not restored: %q", got)
	}
	if _, err := os.Stat(exe + ".old"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".old left behind: %v", err)
	}
}
