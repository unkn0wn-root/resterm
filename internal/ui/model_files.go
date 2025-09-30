package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

func (m *Model) openSelectedFile() tea.Cmd {
	path := selectedFilePath(m.fileList.SelectedItem())
	if path == "" {
		return nil
	}
	return m.openFile(path)
}

func (m *Model) openFile(path string) tea.Cmd {
	data, err := os.ReadFile(path)
	if err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("open failed: %v", err), level: statusError}
		}
	}
	m.currentFile = path
	m.cfg.FilePath = path
	m.editor.SetValue(string(data))
	m.editor.SetCursor(0)
	m.doc = parser.Parse(path, data)
	m.syncRequestList(m.doc)
	m.currentRequest = nil
	m.activeRequestTitle = ""
	m.activeRequestKey = ""
	m.dirty = false
	m.statusMessage = statusMsg{text: fmt.Sprintf("Opened %s", filepath.Base(path)), level: statusSuccess}
	m.syncHistory()
	return nil
}

func (m *Model) saveFile() tea.Cmd {
	if m.currentFile == "" {
		return func() tea.Msg {
			return statusMsg{text: "No file selected", level: statusWarn}
		}
	}
	content := []byte(m.editor.Value())
	if err := os.WriteFile(m.currentFile, content, 0o644); err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("save failed: %v", err), level: statusError}
		}
	}
	m.doc = parser.Parse(m.currentFile, content)
	m.syncRequestList(m.doc)
	if req := m.findRequestByKey(m.activeRequestKey); req != nil {
		m.currentRequest = req
	}
	m.dirty = false
	return func() tea.Msg {
		return statusMsg{text: fmt.Sprintf("Saved %s", filepath.Base(m.currentFile)), level: statusSuccess}
	}
}

func (m *Model) reloadWorkspace() tea.Cmd {
	entries, err := filesvc.ListRequestFiles(m.workspaceRoot, m.workspaceRecursive)
	if err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("workspace error: %v", err), level: statusError}
		}
	}
	m.fileList.SetItems(makeFileItems(entries))
	return func() tea.Msg {
		return statusMsg{text: "Workspace refreshed", level: statusSuccess}
	}
}

func (m *Model) selectFileByPath(path string) bool {
	items := m.fileList.Items()
	for i, item := range items {
		if fi, ok := item.(fileItem); ok {
			if filepath.Clean(fi.entry.Path) == filepath.Clean(path) {
				m.fileList.Select(i)
				return true
			}
		}
	}
	return false
}

func (m *Model) ensureWorkspaceFile(path string) bool {
	clean := filepath.Clean(path)
	root := filepath.Clean(m.workspaceRoot)
	rel, err := filepath.Rel(root, clean)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}

func (m *Model) reparseDocument() tea.Cmd {
	m.doc = parser.Parse(m.currentFile, []byte(m.editor.Value()))
	m.syncRequestList(m.doc)
	return func() tea.Msg {
		return statusMsg{text: "Document reloaded", level: statusInfo}
	}
}
