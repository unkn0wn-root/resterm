package ui

import (
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
	trimmed := text
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
			e.registerText = text
			return text, pasteSourceClipboard, true, nil
		}

		if e.registerText != "" {
			return e.registerText, pasteSourceRegisterEmpty, true, nil
		}

		msg := statusMsg{text: "Clipboard empty", level: statusWarn}
		return "", "", false, &msg
	}

	if e.registerText != "" {
		return e.registerText, pasteSourceRegisterError, true, nil
	}

	msg := statusMsg{text: "Clipboard unavailable", level: statusWarn}
	return "", "", false, &msg
}

func (e *requestEditor) copyToClipboard(text string) tea.Cmd {
	trimmed := text
	e.registerText = trimmed
	if trimmed == "" {
		return func() tea.Msg {
			return editorEvent{}
		}
	}

	return func() tea.Msg {
		status := e.writeClipboardWithFallback(trimmed, "Copied selection")
		return editorEvent{status: &status}
	}
}
