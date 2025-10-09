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

func TestOpenTemporaryDocumentResetsState(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	path := filepath.Join(tmp, "sample.http")
	content := "GET https://example.com\n\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	model := New(Config{WorkspaceRoot: tmp, Theme: &th})
	m := &model
	if cmd := m.openFile(path); cmd != nil {
		cmd()
	}
	if m.currentFile != path {
		t.Fatalf("expected model to load file before temporary document")
	}

	if cmd := m.openTemporaryDocument(); cmd != nil {
		cmd()
	}

	if m.currentFile != "" {
		t.Fatalf("expected current file to clear, got %q", m.currentFile)
	}
	if m.cfg.FilePath != "" {
		t.Fatalf("expected config file path to clear, got %q", m.cfg.FilePath)
	}
	if m.editor.Value() != "" {
		t.Fatalf("expected editor to be empty, got %q", m.editor.Value())
	}
	if m.doc == nil {
		t.Fatal("expected document to be initialised")
	}
	if len(m.doc.Requests) != 0 {
		t.Fatalf("expected no requests in temporary document, got %d", len(m.doc.Requests))
	}
	if len(m.requestList.Items()) != 0 {
		t.Fatalf("expected request list to clear, got %d items", len(m.requestList.Items()))
	}
	if m.focus != focusEditor {
		t.Fatalf("expected focus to switch to editor, got %v", m.focus)
	}
	if m.statusMessage.text != "Temporary document" {
		t.Fatalf("unexpected status message: %q", m.statusMessage.text)
	}
	if m.dirty {
		t.Fatalf("expected clean editor state for temporary document")
	}
}
