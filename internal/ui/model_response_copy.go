package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) copyResponseTab() tea.Cmd {
	label, content, status := m.responseCopyPayload()
	if status != nil {
		msg := *status
		return func() tea.Msg {
			return msg
		}
	}

	size := formatByteSize(int64(len(content)))
	success := fmt.Sprintf("Copied %s tab (%s)", label, size)
	return (&m.editor).copyToClipboard(content, success)
}

func (m *Model) responseCopyPayload() (string, string, *statusMsg) {
	if m.focus != focusResponse {
		return "", "", &statusMsg{
			text:  "Focus the response pane to copy its contents",
			level: statusInfo,
		}
	}

	pane := m.focusedPane()
	if pane == nil {
		return "", "", &statusMsg{text: "Response pane unavailable", level: statusWarn}
	}

	snap := pane.snapshot
	if snap == nil || !snap.ready {
		return "", "", &statusMsg{text: "No response available to copy", level: statusWarn}
	}

	label, ok := responseCopyTabLabel(pane.activeTab)
	if !ok {
		return "", "", &statusMsg{
			text:  "Copy works only in Pretty, Raw, or Headers tabs",
			level: statusInfo,
		}
	}

	content, _ := m.paneContentForTab(m.responsePaneFocus, pane.activeTab)
	plain := stripANSIEscape(content)
	text := ensureTrailingNewline(plain)
	return label, text, nil
}

func responseCopyTabLabel(tab responseTab) (string, bool) {
	switch tab {
	case responseTabPretty:
		return "Pretty", true
	case responseTabRaw:
		return "Raw", true
	case responseTabHeaders:
		return "Headers", true
	default:
		return "", false
	}
}
