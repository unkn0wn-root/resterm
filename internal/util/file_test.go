package util

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.json")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := WriteFileAtomic(p, []byte("new"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil || string(got) != "new" {
		t.Fatalf("content = %q, %v, want new", got, err)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", st.Mode().Perm())
		}
	}
	es, err := os.ReadDir(dir)
	if err != nil || len(es) != 1 {
		t.Fatalf("dir holds %d entries, %v, want only the target", len(es), err)
	}
}

func TestWriteFileAtomicMissingDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing", "out.json")
	if err := WriteFileAtomic(p, []byte("x"), 0o600); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}
