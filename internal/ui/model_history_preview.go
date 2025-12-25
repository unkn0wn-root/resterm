package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func (m *Model) openHistoryPreview(entry history.Entry) {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		m.setStatusMessage(
			statusMsg{level: statusError, text: fmt.Sprintf("preview error: %v", err)},
		)
		return
	}
	m.historyPreviewContent = string(data)
	m.historyPreviewTitle = historyPreviewTitle(entry)
	m.showHistoryPreview = true
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	if vp := m.historyPreviewViewport; vp != nil {
		vp.SetYOffset(0)
		vp.GotoTop()
	}
}

func (m *Model) closeHistoryPreview() {
	m.showHistoryPreview = false
	m.historyPreviewContent = ""
	m.historyPreviewTitle = ""
	if vp := m.historyPreviewViewport; vp != nil {
		vp.SetYOffset(0)
		vp.GotoTop()
	}
}

func historyPreviewTitle(entry history.Entry) string {
	if label := strings.TrimSpace(entry.RequestName); label != "" {
		return label
	}
	method := strings.TrimSpace(entry.Method)
	url := strings.TrimSpace(entry.URL)
	switch {
	case method != "" && url != "":
		return fmt.Sprintf("%s %s", method, url)
	case method != "":
		return method
	case url != "":
		return url
	default:
		return "History Entry"
	}
}

func (m *Model) selectedHistoryEntry() (history.Entry, bool) {
	idx := m.historyList.Index()
	if idx < 0 || idx >= len(m.historyEntries) {
		return history.Entry{}, false
	}
	return m.historyEntries[idx], true
}
