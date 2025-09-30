package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestSubmitNewFileCreatesFile(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	model := New(Config{WorkspaceRoot: tmp, Theme: &th})
	m := &model
	m.openNewFileModal()
	m.newFileInput.SetValue("sample")
	m.newFileExtIndex = 0
	if cmd := m.submitNewFile(); cmd != nil {
		cmd()
	}
	path := filepath.Join(tmp, "sample.http")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to be created: %v", err)
	}
	if m.currentFile != path {
		t.Fatalf("expected current file to point to new file")
	}
	if m.showNewFileModal {
		t.Fatalf("expected modal to close after creation")
	}
}

func TestSubmitNewFileRejectsInvalidExtension(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	model := New(Config{WorkspaceRoot: tmp, Theme: &th})
	m := &model
	m.openNewFileModal()
	m.newFileInput.SetValue("invalid.txt")
	m.newFileExtIndex = 0
	if cmd := m.submitNewFile(); cmd != nil {
		cmd()
	}
	if m.newFileError == "" {
		t.Fatalf("expected validation error for unsupported extension")
	}
	if !m.showNewFileModal {
		t.Fatalf("modal should remain open on validation error")
	}
	if _, err := os.Stat(filepath.Join(tmp, "invalid.txt")); err == nil {
		t.Fatalf("unexpected file creation for invalid extension")
	}
}
