package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

type diskContentOptions struct {
	PreserveView bool
}

type editorContentOptions struct {
	PreserveView bool
}

type editorDocumentReplacement struct {
	path                  string
	value                 string
	doc                   *restfile.Document
	cacheCurrent          bool
	reportWorkspaceErrors bool
}

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
	doc := parseEditableDocument(path, data)
	if err := m.replaceEditorWithDocument(editorDocumentReplacement{
		path:  path,
		value: string(data),
		doc:   doc,
	}); err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("workspace error: %v", err), level: statusError}
		}
	}
	m.watchFile(path, data)
	m.setHistoryScopeForFile(path)
	m.syncHistory()
	if len(m.requestItems) > 0 {
		m.syncEditorWithRequestSelection(-1)
	}
	return nil
}

func (m *Model) setHistoryScopeForFile(path string) {
	if !filesvc.IsRequestFile(path) {
		return
	}
	m.historyScope = historyScopeRequest
}

func (m *Model) openTemporaryDocument() tea.Cmd {
	_ = m.replaceEditorWithDocument(editorDocumentReplacement{
		doc:                   parser.Parse("", nil),
		reportWorkspaceErrors: true,
	})
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
		return statusMsg{
			text:  fmt.Sprintf("Saved %s", filepath.Base(m.currentFile)),
			level: statusSuccess,
		}
	}
}

func (m *Model) reloadWorkspace() tea.Cmd {
	entries, err := m.listWorkspaceEntries()
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
			if util.SamePath(fi.entry.Path, path) {
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
	wasDirty := m.dirty
	m.refreshCurrentDocument([]byte(m.editor.Value()))
	// Reparse refreshes derived UI state from the editor buffer. It is not a save.
	m.dirty = wasDirty
	return func() tea.Msg {
		return statusMsg{text: "Document reloaded", level: statusInfo}
	}
}

func (m *Model) reloadFileFromDisk() tea.Cmd {
	path := m.currentFile
	if path == "" {
		return func() tea.Msg {
			return statusMsg{text: "No file selected", level: statusWarn}
		}
	}
	if m.dirty && !m.pendingReloadConfirm {
		m.pendingReloadConfirm = true
		m.openReloadConfirmModal(manualDirtyReloadMessage())
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("reload failed: %v", err), level: statusError}
		}
	}

	m.applyDiskContent(path, data, diskContentOptions{})

	return func() tea.Msg {
		return statusMsg{text: fmt.Sprintf("Reloaded %s", filepath.Base(path)), level: statusInfo}
	}
}

func manualDirtyReloadMessage() string {
	return "Reload from disk? Unsaved changes in Resterm will be discarded."
}

func (m *Model) applyDiskContent(path string, data []byte, opt diskContentOptions) {
	m.replaceEditorContent(string(data), editorContentOptions{
		PreserveView: opt.PreserveView,
	})
	m.refreshCurrentDocument(data)
	m.watchFile(path, data)
}

func (m *Model) replaceEditorWithDocument(
	repl editorDocumentReplacement,
) error {
	if repl.cacheCurrent && m.currentFile != "" && m.doc != nil {
		m.cacheDoc(m.currentFile, m.doc)
	}

	m.forgetFileWatch(m.currentFile)
	m.currentFile = repl.path
	m.cfg.FilePath = repl.path
	m.updateEditorStyler(repl.path)
	m.resetCursorSync()
	m.replaceEditorContent(repl.value, editorContentOptions{})

	m.currentRequest = nil
	m.activeRequestKey = ""
	m.activeRequestTitle = ""
	m.doc = repl.doc
	m.syncRegistry(m.doc)
	m.syncRequestList(m.doc)

	entries, err := m.syncWorkspaceEntries()
	if err != nil {
		if !repl.reportWorkspaceErrors {
			return err
		}
		m.setStatusMessage(statusMsg{
			text:  fmt.Sprintf("workspace error: %v", err),
			level: statusError,
		})
	}
	m.rebuildNavigator(entries)
	m.markClean()
	return nil
}

func (m *Model) replaceEditorContent(value string, opt editorContentOptions) {
	pos := cursorPosition{}
	viewStart := 0
	if opt.PreserveView {
		pos = m.editor.caretPosition()
		viewStart = m.editor.ViewStart()
	}

	_ = m.setInsertMode(false, false)
	m.editor.SetValue(value)
	m.editor.ResetUndo()
	if opt.PreserveView {
		m.editor.moveCursorTo(pos.Line, pos.Column)
		m.editor.SetViewStart(viewStart)
	} else {
		m.editor.SetViewStart(0)
		m.editor.moveCursorTo(0, 0)
	}
	m.editor.ClearSelection()
}

func (m *Model) refreshCurrentDocument(content []byte) {
	m.doc = parseEditableDocument(m.currentFile, content)
	m.syncRegistry(m.doc)
	m.syncRequestList(m.doc)
	entries := m.syncWorkspaceEntriesStatus()
	m.rebuildNavigator(entries)
	m.resetCursorSync()
	m.updateEditorStyler(m.currentFile)
	if req := m.findRequestByKey(m.activeRequestKey); req != nil {
		m.currentRequest = req
	}
	m.markClean()
}

func (m *Model) markDirty() {
	m.dirty = true
	m.clearPendingCrossFileNavigation()
}

func (m *Model) markClean() {
	m.dirty = false
	m.clearPendingCrossFileNavigation()
}

func (m *Model) updateEditorStyler(path string) {
	m.editor.SetRuneStyler(selectEditorRuneStyler(path, m.theme.EditorMetadata))
}

func parseEditableDocument(path string, data []byte) *restfile.Document {
	if !filesvc.IsRequestFile(path) {
		return &restfile.Document{Path: path, Raw: append([]byte(nil), data...)}
	}
	return parser.Parse(path, data)
}
