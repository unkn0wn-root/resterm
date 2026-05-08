package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/extedit"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
)

func TestExternalEditorTargetUsesCurrentFileOutsideSidebar(t *testing.T) {
	m := &Model{
		focus:       focusEditor,
		currentFile: "/tmp/current.http",
		navigator: navigator.New[any]([]*navigator.Node[any]{{
			ID:      "file:/tmp/selected.http",
			Kind:    navigator.KindFile,
			Payload: navigator.Payload[any]{FilePath: "/tmp/selected.http"},
		}}),
	}

	if got := m.externalEditorTarget(); got != "/tmp/current.http" {
		t.Fatalf("expected current file target, got %q", got)
	}
}

func TestExternalEditorTargetUsesSelectedNavigatorFile(t *testing.T) {
	selected := "/tmp/selected.http"
	m := &Model{
		focus: focusRequests,
		navigator: navigator.New[any]([]*navigator.Node[any]{{
			ID:      "file:" + selected,
			Kind:    navigator.KindFile,
			Payload: navigator.Payload[any]{FilePath: selected},
		}}),
	}

	if got := m.externalEditorTarget(); got != selected {
		t.Fatalf("expected selected file target, got %q", got)
	}
}

func TestOpenFileInEditorRejectsSelectedNavigatorDir(t *testing.T) {
	dir := t.TempDir()
	current := writeExternalEditorFile(t, "api.http")
	m := &Model{
		focus:       focusFile,
		currentFile: current,
		navigator: navigator.New[any]([]*navigator.Node[any]{{
			ID:      "dir:" + dir,
			Kind:    navigator.KindDir,
			Payload: navigator.Payload[any]{FilePath: dir},
		}}),
	}

	if cmd := m.openFileInEditor(); cmd != nil {
		t.Fatalf("expected no command for directory selection")
	}
	if got := m.statusMessage.text; got != "Select a file to open in editor" {
		t.Fatalf("expected directory warning, got %q", got)
	}
}

func TestOpenFileInEditorWarnsWhenNoEditorConfigured(t *testing.T) {
	clearEditorEnv(t)
	file := writeExternalEditorFile(t, "api.http")
	m := &Model{currentFile: file, focus: focusEditor}

	if cmd := m.openFileInEditor(); cmd != nil {
		t.Fatalf("expected no command when editor is not configured")
	}
	if !strings.Contains(m.statusMessage.text, "Set RESTERM_EDITOR") {
		t.Fatalf("expected editor env warning, got %q", m.statusMessage.text)
	}
}

func TestOpenFileInEditorWarnsForUnsupportedFile(t *testing.T) {
	setTestEditor(t)
	file := writeExternalEditorFile(t, "notes.txt")
	m := &Model{currentFile: file, focus: focusEditor}

	if cmd := m.openFileInEditor(); cmd != nil {
		t.Fatalf("expected no command for unsupported file")
	}
	if !strings.Contains(m.statusMessage.text, "Resterm-supported") {
		t.Fatalf("expected unsupported file warning, got %q", m.statusMessage.text)
	}
}

func TestOpenFileInEditorLaunchesAndWarnsForDirtyCurrentFile(t *testing.T) {
	setTestEditor(t)
	file := writeExternalEditorFile(t, "api.http")
	m := &Model{currentFile: file, focus: focusEditor, dirty: true}

	if cmd := m.openFileInEditor(); cmd == nil {
		t.Fatalf("expected command for supported file")
	}
	if !strings.Contains(m.statusMessage.text, "unsaved Resterm changes") {
		t.Fatalf("expected dirty buffer warning, got %q", m.statusMessage.text)
	}
}

func writeExternalEditorFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("GET https://example.com"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

func clearEditorEnv(t *testing.T) {
	t.Helper()
	t.Setenv(extedit.EnvRestermEditor, "")
	t.Setenv(extedit.EnvVisual, "")
	t.Setenv(extedit.EnvEditor, "")
}

func setTestEditor(t *testing.T) {
	t.Helper()
	clearEditorEnv(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	t.Setenv(extedit.EnvRestermEditor, shellSingleQuote(exe))
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
