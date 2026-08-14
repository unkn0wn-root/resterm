package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/launch"
	"github.com/unkn0wn-root/resterm/internal/util"
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
	m.responseSaveJustOpened = true
	m.closeHelp()
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.closeOpenModal()
	m.closeNewFileModal()
	return m.responseSavePrompt.openWith(
		m.responseSavePathSource(),
		m.defaultResponseSavePath(snapshot),
	)
}

func (m *Model) handleResponseSaveKey(msg tea.KeyMsg) tea.Cmd {
	m.responseSaveError = ""
	src := m.responseSavePathSource()

	switch msg.String() {
	case "esc":
		m.closeResponseSaveModal()
		return nil
	case "ctrl+q", "ctrl+d":
		return tea.Quit
	case "enter":
		cmd, more, err := m.responseSavePrompt.accept(src)
		if err != nil {
			m.responseSaveError = err.Error()
			return nil
		}
		if more {
			return cmd
		}
		return m.submitResponseSave()
	}

	cmd, err := m.responseSavePrompt.handleKey(msg, src)
	if err != nil {
		m.responseSaveError = err.Error()
		return nil
	}
	return cmd
}

func (m *Model) closeResponseSaveModal() {
	m.showResponseSaveModal = false
	m.responseSaveError = ""
	m.responseSaveJustOpened = false
	m.responseSavePrompt.close()
}

func (m *Model) responseSaveDir() string {
	if dir := strings.TrimSpace(m.lastResponseSaveDir); dir != "" {
		return dir
	}
	if dir := strings.TrimSpace(m.ws.root); dir != "" {
		return dir
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

func (m *Model) defaultResponseSavePath(snapshot *responseSnapshot) string {
	base := m.responseSaveDir()
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

	if err := launch.New().Open(tmpPath); err != nil {
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

	input := strings.TrimSpace(m.responseSavePrompt.value())
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
	path := util.ExpandHome(input)
	if !filepath.IsAbs(path) {
		path = filepath.Join(m.responseSaveDir(), path)
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
