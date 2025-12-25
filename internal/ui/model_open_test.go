package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestSubmitOpenPathOpensFile(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "demo.http")
	if err := os.WriteFile(file, []byte("GET https://example.com"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	th := theme.DefaultTheme()
	model := New(Config{WorkspaceRoot: tmp, Theme: &th})
	m := &model
	m.openOpenModal()
	m.openPathInput.SetValue(file)
	if cmd := m.submitOpenPath(); cmd != nil {
		cmd()
	}

	if m.currentFile != file {
		t.Fatalf("expected current file %q, got %q", file, m.currentFile)
	}
	if filepath.Clean(m.workspaceRoot) != filepath.Clean(filepath.Dir(file)) {
		t.Fatalf("expected workspace to switch to file directory")
	}
	selected := selectedFilePath(m.fileList.SelectedItem())
	if filepath.Clean(selected) != filepath.Clean(file) {
		t.Fatalf("expected file list to select opened file")
	}
}

func TestSubmitOpenPathSwitchesWorkspace(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "sample.http"),
		[]byte("GET https://example.com"),
		0o644,
	); err != nil {
		t.Fatalf("write file: %v", err)
	}

	th := theme.DefaultTheme()
	model := New(Config{WorkspaceRoot: tmp, Theme: &th})
	m := &model
	m.openOpenModal()
	m.openPathInput.SetValue(dir)
	if cmd := m.submitOpenPath(); cmd != nil {
		cmd()
	}

	if filepath.Clean(m.workspaceRoot) != filepath.Clean(dir) {
		t.Fatalf("expected workspace root to switch to directory")
	}
	if len(m.fileList.Items()) == 0 {
		t.Fatalf("expected file list to populate after switching workspace")
	}
}

func TestSubmitOpenPathRejectsInvalidFile(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "invalid.txt")
	if err := os.WriteFile(file, []byte("GET https://example.com"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	th := theme.DefaultTheme()
	model := New(Config{WorkspaceRoot: tmp, Theme: &th})
	m := &model
	m.openOpenModal()
	m.openPathInput.SetValue(file)
	if cmd := m.submitOpenPath(); cmd != nil {
		cmd()
	}
	if m.openPathError == "" {
		t.Fatalf("expected validation error for unsupported file extension")
	}
	if !m.showOpenModal {
		t.Fatalf("modal should remain open on error")
	}
}
