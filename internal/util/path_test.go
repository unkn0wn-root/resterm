package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSamePath(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("internal", "util", "path.go"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{
			name: "relative absolute",
			a:    filepath.Join("internal", "util", ".", "path.go"),
			b:    abs,
			want: true,
		},
		{
			name: "clean equivalent",
			a:    filepath.Join("internal", "util", "..", "util", "path.go"),
			b:    filepath.Join("internal", "util", "path.go"),
			want: true,
		},
		{
			name: "empty does not match empty",
			a:    "",
			b:    "",
			want: false,
		},
		{
			name: "different",
			a:    filepath.Join("internal", "util", "path.go"),
			b:    filepath.Join("internal", "util", "string.go"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SamePath(tt.a, tt.b); got != tt.want {
				t.Fatalf("SamePath(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSameFile(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	other := filepath.Join(dir, "other.json")
	for _, path := range []string{real, other} {
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(dir, "alias.json")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if !SameFile(real, alias) {
		t.Fatal("a symlink and its target name one file")
	}
	if !SameFile(real, filepath.Join(dir, ".", "real.json")) {
		t.Fatal("lexically equivalent paths name one file")
	}
	if SameFile(real, other) {
		t.Fatal("two distinct files must not match")
	}
	if SameFile(real, filepath.Join(dir, "missing.json")) {
		t.Fatal("a missing path must not match")
	}
	if SameFile("", "") || SameFile(real, "") {
		t.Fatal("an empty path must not match")
	}
}

func TestSamePathOrBothEmpty(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("internal", "util", "path.go"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	if !SamePathOrBothEmpty("", "") {
		t.Fatal("expected two empty paths to match")
	}
	if SamePathOrBothEmpty("", abs) {
		t.Fatal("expected one empty path not to match")
	}
	if !SamePathOrBothEmpty(filepath.Join("internal", "util", "path.go"), abs) {
		t.Fatal("expected relative and absolute paths to match")
	}
}
