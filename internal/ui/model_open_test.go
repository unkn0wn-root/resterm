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

func TestSubmitOpenPathOpensEnvFile(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, ".env.local")
	if err := os.WriteFile(
		file,
		[]byte("workspace=dev\nAPI_URL=https://example.com\n"),
		0o644,
	); err != nil {
		t.Fatalf("write file: %v", err)
	}

	th := theme.DefaultTheme()
	model := New(Config{WorkspaceRoot: tmp, Theme: &th, EnvironmentFile: file})
	m := &model
	m.openOpenModal()
	m.openPathInput.SetValue(file)
	if cmd := m.submitOpenPath(); cmd != nil {
		cmd()
	}

	if m.currentFile != file {
		t.Fatalf("expected current file %q, got %q", file, m.currentFile)
	}
}

func TestSubmitOpenPathOpensAuxiliaryWorkspaceFiles(t *testing.T) {
	tests := []struct {
		name string
		file string
		body string
	}{
		{name: "graphql", file: "query.graphql", body: "query { viewer { id } }"},
		{name: "gql", file: "query.gql", body: "query { viewer { id } }"},
		{name: "json", file: "variables.json", body: `{"id":"1"}`},
		{name: "js", file: "pre.js", body: "request.setHeader('X-Test', '1');"},
		{name: "mjs", file: "pre.mjs", body: "export const value = 1;"},
		{name: "cjs", file: "pre.cjs", body: "module.exports = {};"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			file := filepath.Join(tmp, tt.file)
			if err := os.WriteFile(file, []byte(tt.body), 0o644); err != nil {
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
			if got := m.editor.Value(); got != tt.body {
				t.Fatalf("expected editor body %q, got %q", tt.body, got)
			}
			if len(m.requestItems) != 0 {
				t.Fatalf(
					"expected auxiliary file not to populate requests, got %+v",
					m.requestItems,
				)
			}
		})
	}
}
