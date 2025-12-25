package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
)

func (m *Model) openOpenModal() {
	m.showOpenModal = true
	m.openPathError = ""
	m.openPathInput.SetValue("")
	m.openPathInput.Focus()
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.closeNewFileModal()
}

func (m *Model) closeOpenModal() {
	m.showOpenModal = false
	m.openPathError = ""
	m.openPathInput.Blur()
	m.openPathInput.SetValue("")
}

func (m *Model) submitOpenPath() tea.Cmd {
	input := strings.TrimSpace(m.openPathInput.Value())
	if input == "" {
		m.openPathError = "Enter a path"
		return nil
	}

	resolved, err := m.resolveOpenPath(input)
	if err != nil {
		m.openPathError = err.Error()
		return nil
	}

	info, err := os.Stat(resolved)
	if err != nil {
		m.openPathError = fmt.Sprintf("stat path: %v", err)
		return nil
	}

	if info.IsDir() {
		return m.applyOpenDirectory(resolved)
	}

	if !isSupportedRequestFile(resolved) {
		m.openPathError = "Only .http or .rest files are supported"
		return nil
	}

	return m.applyOpenFilePath(resolved)
}

func (m *Model) resolveOpenPath(input string) (string, error) {
	path := expandHome(input)
	if !filepath.IsAbs(path) {
		base := m.workspaceRoot
		if base == "" {
			if wd, err := os.Getwd(); err == nil {
				base = wd
			}
		}
		if base != "" {
			path = filepath.Join(base, path)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	return abs, nil
}

func (m *Model) applyOpenDirectory(dir string) tea.Cmd {
	m.closeOpenModal()
	m.forgetFileWatch(m.currentFile)
	m.workspaceRoot = dir
	m.cfg.WorkspaceRoot = dir
	m.cfg.Recursive = m.workspaceRecursive
	m.cfg.FilePath = ""
	m.currentFile = ""
	m.currentRequest = nil
	m.activeRequestKey = ""
	m.activeRequestTitle = ""
	m.doc = nil
	m.editor.SetValue("")
	m.editor.SetCursor(0)
	focusCmd := m.setFocus(focusFile)
	m.requestList.SetItems(nil)
	m.requestItems = nil
	m.requestList.Select(-1)

	entries, err := filesvc.ListRequestFiles(dir, m.workspaceRecursive)
	if err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("workspace error: %v", err), level: statusError}
		}
	}
	m.fileList.SetItems(makeFileItems(entries))
	if len(entries) > 0 {
		m.fileList.Select(0)
	} else {
		m.fileList.Select(-1)
	}
	return batchCommands(
		focusCmd,
		func() tea.Msg {
			return statusMsg{
				text:  fmt.Sprintf("Workspace set to %s", filepath.Base(dir)),
				level: statusInfo,
			}
		},
	)
}

func (m *Model) applyOpenFilePath(path string) tea.Cmd {
	m.closeOpenModal()
	dir := filepath.Dir(path)
	m.workspaceRoot = dir
	m.cfg.WorkspaceRoot = dir
	m.cfg.FilePath = path
	m.cfg.Recursive = m.workspaceRecursive

	entries, err := filesvc.ListRequestFiles(dir, m.workspaceRecursive)
	if err != nil {
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("workspace error: %v", err), level: statusError}
		}
	}
	m.fileList.SetItems(makeFileItems(entries))
	m.selectFileByPath(path)
	focusCmd := m.setFocus(focusEditor)
	return batchCommands(focusCmd, m.openFile(path))
}

func isSupportedRequestFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".http" || ext == ".rest"
}

func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if len(path) == 1 {
		return home
	}
	remainder := path[1:]
	remainder = strings.TrimPrefix(remainder, string(filepath.Separator))
	remainder = strings.TrimPrefix(remainder, "/")
	if remainder == "" {
		return home
	}
	return filepath.Join(home, remainder)
}
