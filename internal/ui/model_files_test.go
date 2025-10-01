package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestSaveFileWithoutSelectionAndEmptyEditorWarns(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	model := New(Config{WorkspaceRoot: tmp, Theme: &th})
	m := &model
	m.editor.SetValue("")

	cmd := m.saveFile()
	if cmd == nil {
		t.Fatalf("expected warning command when no file is selected")
	}
	msg, ok := cmd().(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg response, got %T", msg)
	}
	if msg.text != "No file selected" {
		t.Fatalf("unexpected warning text: %q", msg.text)
	}
	if msg.level != statusWarn {
		t.Fatalf("expected warning level, got %v", msg.level)
	}
}

func TestSaveFilePromptsForPathAndSavesContent(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	const content = "GET https://example.com\n"
	model := New(Config{WorkspaceRoot: tmp, Theme: &th, InitialContent: content})
	m := &model

	cmd := m.saveFile()
	if cmd != nil {
		t.Fatalf("expected nil command when prompting for save, got %T", cmd())
	}
	if !m.showNewFileModal {
		t.Fatalf("expected new file modal to open")
	}
	if !m.newFileFromSave {
		t.Fatalf("expected save mode flag to be set")
	}

	m.newFileInput.SetValue("draft")
	m.newFileExtIndex = 0
	saveCmd := m.submitNewFile()
	if saveCmd != nil {
		saveCmd()
	}

	path := filepath.Join(tmp, "draft.http")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file to be written: %v", err)
	}
	if string(data) != content {
		t.Fatalf("unexpected file contents: %q", string(data))
	}
	if m.currentFile != path {
		t.Fatalf("expected current file to be %q, got %q", path, m.currentFile)
	}
	if m.statusMessage.text != "Saved draft.http" {
		t.Fatalf("unexpected status message: %q", m.statusMessage.text)
	}
	if m.showNewFileModal {
		t.Fatalf("expected modal to close after saving")
	}
	if m.newFileFromSave {
		t.Fatalf("expected save flag to reset")
	}
}
