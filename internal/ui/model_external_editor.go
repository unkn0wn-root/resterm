package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/extedit"
	"github.com/unkn0wn-root/resterm/internal/util"
)

func (m *Model) openFileInEditor() tea.Cmd {
	path := m.externalEditorTarget()
	if strings.TrimSpace(path) == "" {
		m.setStatusMessage(statusMsg{text: "No file selected", level: statusWarn})
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		m.setStatusMessage(statusMsg{
			text:  fmt.Sprintf("Open editor failed: %v", err),
			level: statusWarn,
		})
		return nil
	}
	if info.IsDir() {
		m.setStatusMessage(statusMsg{text: "Select a file to open in editor", level: statusWarn})
		return nil
	}
	if !m.isSupportedOpenPath(path) {
		m.setStatusMessage(statusMsg{
			text:  "Only Resterm-supported workspace files can be opened in editor",
			level: statusWarn,
		})
		return nil
	}

	cmd, err := extedit.Resolve()
	if err != nil {
		m.setStatusMessage(statusMsg{text: editorResolveMsg(err), level: statusWarn})
		return nil
	}

	if m.dirty && util.SamePath(path, m.currentFile) {
		m.setStatusMessage(statusMsg{
			text:  "Opening on-disk file; unsaved Resterm changes are not included",
			level: statusWarn,
		})
	} else {
		m.setStatusMessage(statusMsg{
			text:  fmt.Sprintf("Opening %s in external editor", filepath.Base(path)),
			level: statusInfo,
		})
	}

	proc := cmd.Exec(path)
	return tea.ExecProcess(proc, func(err error) tea.Msg {
		return externalEditorMsg{path: path, err: err}
	})
}

func (m *Model) externalEditorTarget() string {
	switch m.focus {
	case focusFile, focusRequests, focusWorkflows:
		if path, ok := m.navigatorSelectedPath(); ok {
			return path
		}
		if m.focus == focusFile {
			if path := selectedFilePath(m.fileList.SelectedItem()); path != "" {
				return path
			}
		}
	}
	return m.currentFile
}

func (m *Model) navigatorSelectedPath() (string, bool) {
	if m.navigator == nil {
		return "", false
	}
	n := m.navigator.Selected()
	if n == nil {
		return "", false
	}
	return strings.TrimSpace(n.Payload.FilePath), true
}

func (m *Model) handleExternalEditorMsg(msg externalEditorMsg) tea.Cmd {
	if msg.err != nil {
		m.setStatusMessage(statusMsg{
			text:  fmt.Sprintf("Open editor failed: %v", msg.err),
			level: statusWarn,
		})
		return nil
	}

	if msg.path != "" && util.SamePath(msg.path, m.currentFile) && m.fileWatcher != nil {
		m.fileWatcher.Scan()
	}
	m.setStatusMessage(statusMsg{
		text:  fmt.Sprintf("External editor closed: %s", filepath.Base(msg.path)),
		level: statusInfo,
	})
	return nil
}

func editorResolveMsg(err error) string {
	switch {
	case errors.Is(err, extedit.ErrNoEditor):
		return "Set RESTERM_EDITOR, VISUAL, or EDITOR to open files in an external editor"
	case errors.Is(err, extedit.ErrNoArgs):
		return "External editor command is empty"
	default:
		return fmt.Sprintf("Open editor failed: %v", err)
	}
}
