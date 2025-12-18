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
	cmd := m.openFile(path)
	return batchCommands(cmd, m.setFocus(focusRequests))
}

func (m *Model) openFile(path string) tea.Cmd {
	data, err := os.ReadFile(path)
	if err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("open failed: %v", err), level: statusError}
		}
	}
	m.forgetFileWatch(m.currentFile)
	m.currentFile = path
	m.cfg.FilePath = path
	_ = m.setInsertMode(false, false)
	m.editor.ClearSelection()
	m.editor.SetValue(string(data))
	m.editor.undoStack = nil
	m.editor.SetViewStart(0)
	m.editor.moveCursorTo(0, 0)
	m.editor.ClearSelection()
	m.currentRequest = nil
	m.activeRequestTitle = ""
	m.activeRequestKey = ""
	m.doc = parser.Parse(path, data)
	m.syncSSHGlobals(m.doc)
	m.syncRequestList(m.doc)
	m.rebuildNavigator(nil)
	m.dirty = false
	m.watchFile(path, data)
	m.setStatusMessage(statusMsg{text: fmt.Sprintf("Opened %s", filepath.Base(path)), level: statusSuccess})
	m.syncHistory()
	if len(m.requestItems) > 0 {
		m.syncEditorWithRequestSelection(-1)
	}
	return nil
}

func (m *Model) openTemporaryDocument() tea.Cmd {
	m.forgetFileWatch(m.currentFile)
	m.cfg.FilePath = ""
	m.currentFile = ""
	m.currentRequest = nil
	m.activeRequestKey = ""
	m.activeRequestTitle = ""
	_ = m.setInsertMode(false, false)
	m.editor.ClearSelection()
	m.editor.SetValue("")
	m.editor.undoStack = nil
	m.editor.SetViewStart(0)
	m.editor.moveCursorTo(0, 0)
	m.editor.ClearSelection()
	m.doc = parser.Parse("", nil)
	m.syncSSHGlobals(m.doc)
	m.syncRequestList(m.doc)
	m.dirty = false
	m.syncHistory()
	focusCmd := m.setFocus(focusEditor)
	m.setStatusMessage(statusMsg{text: "Temporary document", level: statusInfo})
	return focusCmd
}

func (m *Model) saveFile() tea.Cmd {
	if m.currentFile == "" {
		if strings.TrimSpace(m.editor.Value()) == "" {
			return func() tea.Msg {
				return statusMsg{text: "No file selected", level: statusWarn}
			}
		}
		m.openSaveAsModal()
		return nil
	}
	content := []byte(m.editor.Value())
	if err := os.WriteFile(m.currentFile, content, 0o644); err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("save failed: %v", err), level: statusError}
		}
	}
	m.watchFile(m.currentFile, content)
	m.refreshCurrentDocument(content)
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
	m.rebuildNavigator(entries)
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
	m.syncSSHGlobals(m.doc)
	m.syncRequestList(m.doc)
	m.rebuildNavigator(nil)
	return func() tea.Msg {
		return statusMsg{text: "Document reloaded", level: statusInfo}
	}
}

func (m *Model) reloadFileFromDisk() tea.Cmd {
	path := strings.TrimSpace(m.currentFile)
	if path == "" {
		return func() tea.Msg {
			return statusMsg{text: "No file selected", level: statusWarn}
		}
	}
	if m.dirty && !m.pendingReloadConfirm {
		m.pendingReloadConfirm = true
		warn := "Reload will discard unsaved changes. Press reload again to confirm."
		if m.showFileChangeModal {
			m.openFileChangeModal(warn)
		}
		return func() tea.Msg {
			return statusMsg{text: warn, level: statusWarn}
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("reload failed: %v", err), level: statusError}
		}
	}

	m.pendingReloadConfirm = false
	m.fileStale = false
	m.fileMissing = false
	m.closeFileChangeModal()
	_ = m.setInsertMode(false, false)
	m.editor.ClearSelection()
	m.editor.SetValue(string(data))
	m.editor.undoStack = nil
	m.editor.SetViewStart(0)
	m.editor.moveCursorTo(0, 0)
	m.editor.ClearSelection()
	m.refreshCurrentDocument(data)
	m.watchFile(path, data)

	return func() tea.Msg {
		return statusMsg{text: fmt.Sprintf("Reloaded %s", filepath.Base(path)), level: statusInfo}
	}
}

func (m *Model) refreshCurrentDocument(content []byte) {
	m.doc = parser.Parse(m.currentFile, content)
	m.syncSSHGlobals(m.doc)
	m.syncRequestList(m.doc)
	m.rebuildNavigator(nil)
	if req := m.findRequestByKey(m.activeRequestKey); req != nil {
		m.currentRequest = req
	}
	m.dirty = false
}
