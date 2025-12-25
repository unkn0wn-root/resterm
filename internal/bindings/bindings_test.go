package bindings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultMapContainsExpectedBindings(t *testing.T) {
	m := DefaultMap()

	if binding, ok := m.MatchSingle("ctrl+g"); !ok || binding.Action != ActionShowGlobals {
		t.Fatalf("expected ctrl+g -> ActionShowGlobals, got %+v (ok=%v)", binding, ok)
	}

	if binding, ok := m.MatchSingle("ctrl+enter"); !ok || binding.Action != ActionSendRequest {
		t.Fatalf("expected ctrl+enter -> ActionSendRequest, got %+v (ok=%v)", binding, ok)
	}

	if binding, ok := m.MatchSingle("ctrl+c"); !ok || binding.Action != ActionCancelRun {
		t.Fatalf("expected ctrl+c -> ActionCancelRun, got %+v (ok=%v)", binding, ok)
	}

	if binding, ok := m.ResolveChord(
		"g",
		"s",
	); !ok ||
		binding.Action != ActionSetMainSplitHorizontal {
		t.Fatalf("expected g s -> ActionSetMainSplitHorizontal, got %+v (ok=%v)", binding, ok)
	}

	if !m.HasChordPrefix("g") {
		t.Fatalf("expected HasChordPrefix('g') to be true")
	}
}

func TestLoadOverridesBindings(t *testing.T) {
	dir := t.TempDir()
	payload := `
[bindings]
save_file = ["ctrl+shift+s"]
toggle_help = ["ctrl+shift+/"]
`
	path := filepath.Join(dir, "bindings.toml")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write bindings: %v", err)
	}

	m, _, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if binding, ok := m.MatchSingle("ctrl+s"); ok {
		t.Fatalf("expected ctrl+s to be unbound, got %v", binding.Action)
	}

	if binding, ok := m.MatchSingle("ctrl+shift+s"); !ok || binding.Action != ActionSaveFile {
		t.Fatalf("expected ctrl+shift+s -> save_file, got %+v (ok=%v)", binding, ok)
	}

	if binding, ok := m.MatchSingle("ctrl+shift+/"); !ok || binding.Action != ActionToggleHelp {
		t.Fatalf("expected ctrl+shift+/ -> toggle_help, got %+v (ok=%v)", binding, ok)
	}
}

func TestLoadRejectsConflictingBindings(t *testing.T) {
	dir := t.TempDir()
	payload := `
[bindings]
save_file = ["ctrl+s"]
show_globals = ["ctrl+s"]
`
	path := filepath.Join(dir, "bindings.toml")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write bindings: %v", err)
	}

	if _, _, err := Load(dir); err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}
