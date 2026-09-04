package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitTakesDirFromFlagOrPositional(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "positional", args: []string{"target"}},
		{name: "flag", args: []string{"--dir", "target"}},
		{name: "flag after positional", args: []string{"--template", "minimal", "target"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			if _, _, err := captureRunIO(t, func() error { return runInit(test.args) }); err != nil {
				t.Fatalf("runInit(%q): %v", test.args, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "target", "requests.http")); err != nil {
				t.Fatalf("runInit(%q) did not write into target: %v", test.args, err)
			}
		})
	}
}

// A --dir given alongside a positional directory is ambiguous, whatever value
// the flag carries.
func TestRunInitRejectsDirFlagWithPositional(t *testing.T) {
	for _, args := range [][]string{
		{"--dir", ".", "target"},
		{"--dir", " . ", "target"},
		{"--dir", "other", "target"},
		{"target", "--dir", "."},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Chdir(t.TempDir())
			_, _, err := captureRunIO(t, func() error { return runInit(args) })
			if err == nil {
				t.Fatalf("runInit(%q) = nil, want an ambiguity error", args)
			}
		})
	}
}
