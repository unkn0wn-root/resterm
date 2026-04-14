package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCommandFileConflict(t *testing.T) {
	err := CommandFileConflict(
		"resterm",
		"run",
		"pass a subcommand like `resterm history export --out ./history.json`",
	)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	got := err.Error()
	want := `run: found file named "run" in the current directory; use ` +
		"`resterm -- run` or `resterm ./run` to open it, or pass a subcommand like `resterm history export --out ./history.json`"
	if got != want {
		t.Fatalf("conflict error = %q, want %q", got, want)
	}
}

func TestHasFileConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run")
	if HasFileConflict(path) {
		t.Fatal("expected missing path to not conflict")
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if !HasFileConflict(path) {
		t.Fatal("expected file path conflict")
	}
	if err := os.Mkdir(filepath.Join(dir, "dir"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if HasFileConflict(filepath.Join(dir, "dir")) {
		t.Fatal("expected directory to not conflict")
	}
}
