package runner

import (
	"path/filepath"
	"testing"
)

func TestAbsCleanPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	got, err := absCleanPath("./artifacts")
	if err != nil {
		t.Fatalf("absCleanPath: %v", err)
	}
	want := filepath.Join(dir, "artifacts")
	if got != want {
		t.Fatalf("absCleanPath: got %q want %q", got, want)
	}

	got, err = absCleanPath("")
	if err != nil {
		t.Fatalf("absCleanPath(empty): %v", err)
	}
	if got != "" {
		t.Fatalf("absCleanPath(empty): got %q want empty", got)
	}
}
