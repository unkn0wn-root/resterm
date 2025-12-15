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
	snapshot, status := m.activeResponseSnapshot()
	if status != nil {
		msg := *status
		return func() tea.Msg { return msg }
	}
	body := snapshot.body
	if len(body) == 0 {
		m.setStatusMessage(statusMsg{level: statusInfo, text: "No response body to save"})
		return nil
	}

	dir := m.workspaceRoot
	if strings.TrimSpace(dir) == "" {
		if cwd, err := os.Getwd(); err == nil {
			dir = cwd
		} else {
			dir = "."
		}
	}

	name := suggestResponseFilename(snapshot)
	path := filepath.Join(dir, name)
	finalPath, err := ensureUniquePath(path)
	if err != nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: fmt.Sprintf("Save failed: %v", err)})
		return nil
	}
	if err := os.WriteFile(finalPath, body, 0o644); err != nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: fmt.Sprintf("Save failed: %v", err)})
		return nil
	}

	m.setStatusMessage(statusMsg{
		level: statusInfo,
		text:  fmt.Sprintf("Saved response body (%s) to %s", formatByteSize(int64(len(body))), finalPath),
	})
	return nil
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
