package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/util"
)

func (m *Model) openOpenModal() tea.Cmd {
	m.showOpenModal = true
	m.openPathError = ""
	m.closeHelp()
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.closeNewFileModal()
	return m.openPathPrompt.open(m.openPathSource())
}

func (m *Model) closeOpenModal() {
	m.showOpenModal = false
	m.openPathError = ""
	m.openPathPrompt.close()
}

func (m *Model) handleOpenModalKey(msg tea.KeyMsg) tea.Cmd {
	m.openPathError = ""
	src := m.openPathSource()

	switch msg.String() {
	case "esc":
		if m.openPathPrompt.dismiss() {
			return nil
		}
		m.closeOpenModal()
		return nil
	case "ctrl+q", "ctrl+d":
		return tea.Quit
	case "enter":
		cmd, more, err := m.openPathPrompt.accept(src)
		if err != nil {
			m.openPathError = err.Error()
			return nil
		}
		if more {
			return cmd
		}
		return m.submitOpenPath()
	}

	cmd, err := m.openPathPrompt.handleKey(msg, src)
	if err != nil {
		m.openPathError = err.Error()
		return nil
	}
	return cmd
}

func (m *Model) submitOpenPath() tea.Cmd {
	cmd, err := m.openPath(m.openPathPrompt.value())
	if err != nil {
		m.openPathError = err.Error()
		return nil
	}
	return cmd
}

func (m *Model) openPath(input string) (tea.Cmd, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("enter a path")
	}

	resolved, err := m.resolveOpenPath(input)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("stat path: %w", err)
	}

	if info.IsDir() {
		return m.applyOpenDirectory(resolved), nil
	}

	if !m.isSupportedOpenPath(resolved) {
		return nil, errors.New("only Resterm-supported workspace files can be opened")
	}

	return m.applyOpenFilePath(resolved), nil
}

func (m *Model) openPathFromCommand(input string) tea.Cmd {
	cmd, err := m.openPath(input)
	if err != nil {
		return statusCmd(statusWarn, err.Error())
	}
	return cmd
}

func (m *Model) resolveOpenPath(input string) (string, error) {
	path := util.ExpandHome(input)
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
	return files.WorkspacePathFilter(m.ws.envFile).Accept(path)
}
