package ui

import (
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	pasteSourceClipboard     = "clipboard"
	pasteSourceRegisterEmpty = "register-empty"
	pasteSourceRegisterError = "register-error"
)

func (e *requestEditor) writeClipboardWithFallback(
	text string,
	success string,
) statusMsg {
	trimmed := normalizeClipboardText(text)
	e.registerText = trimmed
	if trimmed == "" {
		return statusMsg{text: success, level: statusInfo}
	}

	if err := clipboard.WriteAll(trimmed); err != nil {
		return statusMsg{
			level: statusWarn,
			text:  "Clipboard unavailable; saved in editor register",
		}
	}
	return statusMsg{text: success, level: statusInfo}
}

func (e *requestEditor) resolvePasteBuffer() (
	string,
	string,
	bool,
	*statusMsg,
) {
	text, err := clipboard.ReadAll()
	if err == nil {
		if text != "" {
			normalized := normalizeClipboardText(text)
			e.registerText = normalized
			return normalized, pasteSourceClipboard, true, nil
		}

		if e.registerText != "" {
			normalized := normalizeClipboardText(e.registerText)
			e.registerText = normalized
			return normalized, pasteSourceRegisterEmpty, true, nil
		}

		msg := statusMsg{text: "Clipboard empty", level: statusWarn}
		return "", "", false, &msg
	}

	if e.registerText != "" {
		normalized := normalizeClipboardText(e.registerText)
		e.registerText = normalized
		return normalized, pasteSourceRegisterError, true, nil
	}

	msg := statusMsg{text: "Clipboard unavailable", level: statusWarn}
	return "", "", false, &msg
}

func (e *requestEditor) copyToClipboard(text string, success string) tea.Cmd {
	trimmed := normalizeClipboardText(text)
	e.registerText = trimmed
	if trimmed == "" {
		return func() tea.Msg {
			return editorEvent{}
		}
	}

	if strings.TrimSpace(success) == "" {
		success = "Copied selection"
	}

	return func() tea.Msg {
		status := e.writeClipboardWithFallback(trimmed, success)
		return editorEvent{status: &status}
	}
}

func normalizeClipboardText(text string) string {
	if text == "" {
		return text
	}
	if !strings.ContainsRune(text, '\r') {
		return text
	}
	withoutCRLF := strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(withoutCRLF, "\r", "\n")
}
