package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/watcher"
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

func TestReparseDocumentPreservesDirtyState(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	path := filepath.Join(tmp, "sample.http")
	content := "GET https://example.com\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	model := New(Config{
		WorkspaceRoot:  tmp,
		Theme:          &th,
		FilePath:       path,
		InitialContent: content,
	})
	m := &model
	m.editor.SetValue("GET https://changed.example\n")
	m.markDirty()

	if cmd := m.reparseDocument(); cmd != nil {
		cmd()
	}

	if !m.dirty {
		t.Fatalf("expected reparse to preserve unsaved dirty state")
	}
	if len(m.doc.Requests) != 1 || m.doc.Requests[0].URL != "https://changed.example" {
		t.Fatalf("expected document to reparse editor contents, got %#v", m.doc.Requests)
	}
}

func TestFileChangeAutoReloadsCleanBuffer(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	path := filepath.Join(tmp, "changed.http")
	original := "GET https://old.example\n"
	updated := "GET https://new.example\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write original file: %v", err)
	}

	model := New(Config{
		WorkspaceRoot:  tmp,
		Theme:          &th,
		FilePath:       path,
		InitialContent: original,
	})
	m := &model
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("write updated file: %v", err)
	}

	cmd := m.handleFileChangeEvent(fileChangedMsg{path: path, kind: watcher.EventChanged})
	if cmd == nil {
		t.Fatalf("expected auto reload status command")
	}
	if got := m.editor.Value(); got != updated {
		t.Fatalf("expected editor to reload updated content, got %q", got)
	}
	if m.dirty {
		t.Fatalf("expected auto-reloaded buffer to stay clean")
	}
	if m.showFileChangeModal {
		t.Fatalf("did not expect file change modal for clean auto reload")
	}
	if m.fileStale || m.fileMissing {
		t.Fatalf("expected stale/missing flags to clear, got stale=%v missing=%v", m.fileStale, m.fileMissing)
	}
	if len(m.doc.Requests) != 1 || m.doc.Requests[0].URL != "https://new.example" {
		t.Fatalf("expected parsed document to refresh, got %#v", m.doc.Requests)
	}

	msg, ok := cmd().(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg response, got %T", msg)
	}
	if msg.text != "↻ Reloaded changed.http (file changed outside Resterm)" {
		t.Fatalf("unexpected status text: %q", msg.text)
	}
	if msg.level != statusWarn {
		t.Fatalf("expected warning status level, got %v", msg.level)
	}
}

func TestFileChangeDirtyBufferDoesNotAutoReload(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	path := filepath.Join(tmp, "changed.http")
	original := "GET https://old.example\n"
	updated := "GET https://disk.example\n"
	local := "GET https://local.example\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatalf("write original file: %v", err)
	}

	model := New(Config{
		WorkspaceRoot:  tmp,
		Theme:          &th,
		FilePath:       path,
		InitialContent: original,
	})
	m := &model
	m.editor.SetValue(local)
	m.markDirty()
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("write updated file: %v", err)
	}

	if cmd := m.handleFileChangeEvent(fileChangedMsg{path: path, kind: watcher.EventChanged}); cmd != nil {
		t.Fatalf("did not expect dirty buffer warning to return command")
	}
	if got := m.editor.Value(); got != local {
		t.Fatalf("expected dirty local buffer to be preserved, got %q", got)
	}
	if !m.dirty {
		t.Fatalf("expected dirty state to be preserved")
	}
	if !m.showFileChangeModal {
		t.Fatalf("expected file change modal for dirty buffer")
	}
	if !m.fileStale || m.fileMissing {
		t.Fatalf("expected stale non-missing file, got stale=%v missing=%v", m.fileStale, m.fileMissing)
	}
	want := "changed.http changed on disk. Using current buffer."
	if m.statusMessage.text != want {
		t.Fatalf("expected status message %q, got %q", want, m.statusMessage.text)
	}
	if m.statusMessage.level != statusWarn {
		t.Fatalf("expected warning status level, got %v", m.statusMessage.level)
	}
}

func TestFileMissingDoesNotClearCleanBuffer(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	path := filepath.Join(tmp, "missing.http")
	content := "GET https://old.example\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write original file: %v", err)
	}

	model := New(Config{
		WorkspaceRoot:  tmp,
		Theme:          &th,
		FilePath:       path,
		InitialContent: content,
	})
	m := &model
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}

	if cmd := m.handleFileChangeEvent(fileChangedMsg{path: path, kind: watcher.EventMissing}); cmd != nil {
		t.Fatalf("did not expect missing-file warning to return command")
	}
	if got := m.editor.Value(); got != content {
		t.Fatalf("expected clean buffer to be preserved after deletion, got %q", got)
	}
	if !m.showFileChangeModal {
		t.Fatalf("expected file change modal for missing file")
	}
	if !m.fileStale || !m.fileMissing {
		t.Fatalf("expected stale missing file, got stale=%v missing=%v", m.fileStale, m.fileMissing)
	}
	want := "missing.http removed on disk. Using current buffer."
	if m.statusMessage.text != want {
		t.Fatalf("expected status message %q, got %q", want, m.statusMessage.text)
	}
	if m.statusMessage.level != statusWarn {
		t.Fatalf("expected warning status level, got %v", m.statusMessage.level)
	}
}

func TestOpenFileSetsHistoryScopeToRequest(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	fileA := filepath.Join(tmp, "a.http")
	fileB := filepath.Join(tmp, "b.http")
	if err := os.WriteFile(fileA, []byte("GET https://a.test\n"), 0o644); err != nil {
		t.Fatalf("write file A: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("GET https://b.test\n"), 0o644); err != nil {
		t.Fatalf("write file B: %v", err)
	}

	model := New(Config{WorkspaceRoot: tmp, Theme: &th})
	m := &model
	m.historyScope = historyScopeGlobal

	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}
	if m.historyScope != historyScopeRequest {
		t.Fatalf("expected history scope request, got %v", m.historyScope)
	}
	if m.currentRequest == nil || m.currentRequest.URL != "https://a.test" {
		t.Fatalf("expected current request for file A, got %#v", m.currentRequest)
	}

	if cmd := m.openFile(fileB); cmd != nil {
		cmd()
	}
	if m.historyScope != historyScopeRequest {
		t.Fatalf("expected history scope request after file B, got %v", m.historyScope)
	}
	if m.currentRequest == nil || m.currentRequest.URL != "https://b.test" {
		t.Fatalf("expected current request for file B, got %#v", m.currentRequest)
	}
}

func TestReloadWarnUpdatesFileChangeModal(t *testing.T) {
	tmp := t.TempDir()
	th := theme.DefaultTheme()
	path := filepath.Join(tmp, "changed.http")
	if err := os.WriteFile(path, []byte("body"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	model := New(Config{WorkspaceRoot: tmp, Theme: &th, FilePath: path, InitialContent: "body"})
	m := &model
	m.markDirty()
	m.handleFileChangeEvent(fileChangedMsg{path: path, kind: watcher.EventChanged})
	if !m.showFileChangeModal {
		t.Fatalf("expected file change modal to be visible")
	}

	cmd := m.reloadFileFromDisk()
	if cmd == nil {
		t.Fatalf("expected warning command on first reload attempt")
	}
	if !m.pendingReloadConfirm {
		t.Fatalf("expected reload confirmation to be pending")
	}
	want := "Reload will discard unsaved changes. Press reload again to confirm."
	if m.fileChangeMessage != want {
		t.Fatalf("expected modal message %q, got %q", want, m.fileChangeMessage)
	}
}
