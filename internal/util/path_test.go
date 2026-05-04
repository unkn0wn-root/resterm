package util

import (
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
