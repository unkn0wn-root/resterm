package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) openOpenModal() {
	m.showOpenModal = true
	m.openPathError = ""
	m.openPathInput.SetValue("")
	m.openPathInput.Focus()
	m.closeHelp()
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

	if !m.isSupportedOpenPath(resolved) {
		m.openPathError = "Only Resterm-supported workspace files can be opened"
		return nil
	}

	return m.applyOpenFilePath(resolved)
}

func (m *Model) resolveOpenPath(input string) (string, error) {
	path := expandHome(input)
	if !filepath.IsAbs(path) {
		base := m.ws.root
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

	mv, entries, refuse := m.prepareMove(dir, "")
	if refuse != nil {
		return refuse
	}

	status, stopMock := m.commitMove(mv)
	m.forgetFileWatch(m.currentFile)
	m.cfg.FilePath = ""
	m.currentFile = ""
	m.currentRequest = nil
	m.activeRequestKey = ""
	m.activeRequestTitle = ""
	m.setDocument(nil)
	m.editor.SetValue("")
	m.editor.SetCursor(0)
	m.markClean()
	focusCmd := m.setFocus(focusFile)
	// One sync drops everything the old document projected into the UI.
	m.syncRequestList(nil)
	m.syncHistory()

	m.fileList.SetItems(makeFileItems(entries))
	m.rebuildNavigator(entries)
	if len(entries) > 0 {
		m.fileList.Select(0)
	} else {
		m.fileList.Select(-1)
	}

	return batchCommands(
		focusCmd,
		m.refreshGitStatusCmd(),
		stopMock,
		func() tea.Msg { return status },
	)
}

func (m *Model) applyOpenFilePath(path string) tea.Cmd {
	m.closeOpenModal()

	// Read before committing anything. A read failure after the environment
	// swap would leave the new credentials under the old editor.
	data, err := os.ReadFile(path)
	if err != nil {
		return statusCmd(statusError, fmt.Sprintf("open failed: %v", err))
	}

	var status, stopMock tea.Cmd
	// A file outside the current root moves the workspace, so it has to take
	// the environment with it.
	if inside := m.ws.root != "" && m.ensureWorkspaceFile(path); !inside {
		mv, _, refuse := m.prepareMove(filepath.Dir(path), path)
		if refuse != nil {
			return refuse
		}
		moved, stop := m.commitMove(mv)
		stopMock = stop
		status = func() tea.Msg { return moved }
	}
	m.cfg.FilePath = path

	focusCmd := m.setFocus(focusEditor)
	return batchCommands(focusCmd, m.installFile(path, data), stopMock, status)
}

func (m *Model) isSupportedOpenPath(path string) bool {
	switch {
	case filesvc.IsWorkspaceFile(path):
		return true
	case vars.IsDotEnvPath(path):
		return true
	case util.SameFile(path, m.ws.envFile):
		return true
	default:
		return false
	}
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
