package ui

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/clipboard"
)

const (
	pasteSourceClipboard     = "clipboard"
	pasteSourceRegisterEmpty = "register-empty"
	pasteSourceRegisterError = "register-error"
)

type systemClipboard interface {
	Write(text string) (sent bool, err error)
	Read() (string, error)
}

func (e *requestEditor) writeClipboardWithFallback(
	text string,
	success string,
) statusMsg {
	trimmed := normalizeClipboardText(text)
	e.registerText = trimmed
	if trimmed == "" {
		return statusMsg{text: success, level: statusInfo}
	}

	sent, err := e.clip.Write(trimmed)
	switch {
	case errors.Is(err, clipboard.ErrTooLarge):
		return statusMsg{
			level: statusWarn,
			text:  "Too large for OSC 52. Saved in editor register",
		}
	case err != nil:
		return statusMsg{
			level: statusWarn,
			text:  "Clipboard unavailable; saved in editor register",
		}
	case sent && success != "":
		return statusMsg{text: success + ". Sent via OSC 52", level: statusInfo}
	}
	return statusMsg{text: success, level: statusInfo}
}

func (e *requestEditor) resolvePasteBuffer() (
	string,
	string,
	bool,
	*statusMsg,
) {
	text, err := e.clip.Read()
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
