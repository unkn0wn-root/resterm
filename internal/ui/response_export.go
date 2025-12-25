package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
)

func (m *Model) saveResponseBody() tea.Cmd {
	return m.openResponseSaveModal()
}

func (m *Model) openResponseSaveModal() tea.Cmd {
	snapshot, status := m.activeResponseSnapshot()
	if status != nil {
		msg := *status
		return func() tea.Msg { return msg }
	}

	if len(snapshot.body) == 0 {
		m.setStatusMessage(statusMsg{level: statusInfo, text: "No response body to save"})
		return nil
	}

	m.showResponseSaveModal = true
	m.responseSaveError = ""
	m.responseSaveInput.SetValue(m.defaultResponseSavePath(snapshot))
	m.responseSaveInput.CursorEnd()
	m.responseSaveInput.Focus()
	m.responseSaveJustOpened = true
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.closeOpenModal()
	m.closeNewFileModal()
	return nil
}

func (m *Model) closeResponseSaveModal() {
	m.showResponseSaveModal = false
	m.responseSaveError = ""
	m.responseSaveJustOpened = false
	m.responseSaveInput.Blur()
	m.responseSaveInput.SetValue("")
}

func (m *Model) defaultResponseSavePath(snapshot *responseSnapshot) string {
	base := strings.TrimSpace(m.lastResponseSaveDir)
	if base == "" {
		base = strings.TrimSpace(m.workspaceRoot)
	}
	if base == "" {
		if cwd, err := os.Getwd(); err == nil {
			base = cwd
		} else {
			base = "."
		}
	}
	name := suggestResponseFilename(snapshot)
	if strings.TrimSpace(name) == "" {
		name = "response.bin"
	}
	return filepath.Join(base, name)
}

func (m *Model) openResponseExternally() tea.Cmd {
	snapshot, status := m.activeResponseSnapshot()
	if status != nil {
		msg := *status
		return func() tea.Msg { return msg }
	}
	body := snapshot.body
	if len(body) == 0 {
		m.setStatusMessage(statusMsg{level: statusInfo, text: "No response body to open"})
		return nil
	}

	name := suggestResponseFilename(snapshot)
	ext := filepath.Ext(name)
	if ext == "" {
		ext = ".bin"
	}

	tmpFile, err := os.CreateTemp("", "resterm-*"+ext)
	if err != nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: fmt.Sprintf("Open failed: %v", err)})
		return nil
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(body); err != nil {
		_ = tmpFile.Close()
		m.setStatusMessage(statusMsg{level: statusWarn, text: fmt.Sprintf("Open failed: %v", err)})
		return nil
	}
	if err := tmpFile.Close(); err != nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: fmt.Sprintf("Open failed: %v", err)})
		return nil
	}

	if err := launchFile(tmpPath); err != nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: fmt.Sprintf("Open failed: %v", err)})
		return nil
	}

	m.setStatusMessage(statusMsg{
		level: statusInfo,
		text:  fmt.Sprintf("Opening response body in external app (%s)", filepath.Base(tmpPath)),
	})
	return nil
}

func (m *Model) submitResponseSave() tea.Cmd {
	snapshot, status := m.activeResponseSnapshot()
	if status != nil {
		msg := *status
		m.responseSaveError = msg.text
		return nil
	}
	body := snapshot.body
	if len(body) == 0 {
		m.responseSaveError = "No response body to save"
		return nil
	}

	input := strings.TrimSpace(m.responseSaveInput.Value())
	if input == "" {
		m.responseSaveError = "Enter a path"
		return nil
	}
	resolved, err := m.resolveResponseSavePath(input)
	if err != nil {
		m.responseSaveError = err.Error()
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		m.responseSaveError = fmt.Sprintf("create directories: %v", err)
		return nil
	}
	finalPath, err := ensureUniquePath(resolved)
	if err != nil {
		m.responseSaveError = fmt.Sprintf("resolve path: %v", err)
		return nil
	}
	if err := os.WriteFile(finalPath, body, 0o644); err != nil {
		m.responseSaveError = fmt.Sprintf("save failed: %v", err)
		return nil
	}

	m.lastResponseSaveDir = filepath.Dir(finalPath)
	m.closeResponseSaveModal()
	m.setStatusMessage(statusMsg{
		level: statusInfo,
		text: fmt.Sprintf(
			"Saved response body (%s) to %s",
			formatByteSize(int64(len(body))),
			finalPath,
		),
	})
	return nil
}

func (m *Model) resolveResponseSavePath(input string) (string, error) {
	path := expandHome(input)
	if !filepath.IsAbs(path) {
		base := strings.TrimSpace(m.lastResponseSaveDir)
		if base == "" {
			base = strings.TrimSpace(m.workspaceRoot)
		}
		if base == "" {
			if cwd, err := os.Getwd(); err == nil {
				base = cwd
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

func (m *Model) activeResponseSnapshot() (*responseSnapshot, *statusMsg) {
	if m.focus != focusResponse {
		return nil, &statusMsg{level: statusInfo, text: "Focus the response pane first"}
	}
	pane := m.focusedPane()
	if pane == nil {
		return nil, &statusMsg{level: statusWarn, text: "Response pane unavailable"}
	}
	if pane.snapshot == nil || !pane.snapshot.ready {
		return nil, &statusMsg{level: statusWarn, text: "No response available"}
	}
	return pane.snapshot, nil
}

func suggestResponseFilename(snapshot *responseSnapshot) string {
	if snapshot == nil {
		return "response.bin"
	}
	disposition := ""
	if snapshot.responseHeaders != nil {
		disposition = snapshot.responseHeaders.Get("Content-Disposition")
	}
	return binaryview.FilenameHint(disposition, snapshot.effectiveURL, snapshot.contentType)
}

func ensureUniquePath(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	ext := filepath.Ext(path)
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not create unique path for %s", path)
}

func launchFile(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
