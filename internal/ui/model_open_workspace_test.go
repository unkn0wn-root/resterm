package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
)

// An --env-file environment is pinned for the session, so a move keeps it. The
// warning fires only when the new root has a discoverable file of its own that
// the pin is overriding, the same rule discovery itself uses.
func TestPlanKeepsPinnedEnvFile(t *testing.T) {
	base := t.TempDir()
	write := func(rel string) string {
		path := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"e":{"a":"1"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	pinned := write("pinned/custom.env.json")
	write("own/resterm.env.json")
	write("nested/x/resterm.env.json")
	if err := os.MkdirAll(filepath.Join(base, "bare"), 0o755); err != nil {
		t.Fatal(err)
	}

	w := workspace{root: filepath.Join(base, "pinned"), envFile: pinned, envPinned: true}
	for _, tc := range []struct {
		name string
		root string
		warn bool
	}{
		{name: "root with its own env file", root: "own", warn: true},
		// Discovery only looks at the root, so a nested file would not be
		// loaded by a fresh start either and changes nothing.
		{name: "nested file only", root: "nested"},
		{name: "no env file at all", root: "bare"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mv, err := w.plan(filepath.Join(base, tc.root))
			if err != nil {
				t.Fatalf("plan: %v", err)
			}
			if mv.envFile != pinned {
				t.Fatalf("envFile = %q, want the pinned %q", mv.envFile, pinned)
			}
			if mv.reset {
				t.Fatal("a pinned environment has nothing to reset")
			}
			warned := mv.status.level == statusWarn &&
				strings.Contains(mv.status.text, "--env-file")
			if warned != tc.warn {
				t.Fatalf("status = %q (level %v), want warn=%v", mv.status.text, mv.status.level, tc.warn)
			}
		})
	}
}

// The ACTIVE badge and the inactive-environment warning answer the same question,
// so they have to agree even when one file reaches them under two names.
func TestActiveEnvBadgeUsesFileIdentity(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "resterm.env.json")
	if err := os.WriteFile(real, []byte(`{"e":{"a":"1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias.env.json")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	badges := fileEntryBadges(envEntry("alias.env.json", alias), real)
	if !slices.Contains(badges, "ACTIVE") {
		t.Fatalf("badges = %v, want ACTIVE for an alias of the active file", badges)
	}
	if got := inactiveEnvStatus([]filesvc.FileEntry{envEntry("alias.env.json", alias)}, real, true); got.text != "" {
		t.Fatalf("warning = %q, want none, so the two agree", got.text)
	}
}
