package ui

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) scrollResponseToTop() tea.Cmd {
	return m.scrollResponseToEdge(true)
}

func (m *Model) scrollResponseToBottom() tea.Cmd {
	return m.scrollResponseToEdge(false)
}

func (m *Model) scrollResponseToEdge(top bool) tea.Cmd {
	if m.focus != focusResponse {
		return nil
	}
	pane := m.focusedPane()
	if !isScrollableResponsePane(pane) {
		return nil
	}
	if top {
		pane.viewport.GotoTop()
	} else {
		pane.viewport.GotoBottom()
	}
	pane.setCurrPosition()
	return nil
}

func isScrollableResponsePane(pane *responsePaneState) bool {
	if pane == nil {
		return false
	}
	switch pane.activeTab {
	case responseTabPretty, responseTabRaw, responseTabHeaders:
		return true
	default:
		return false
	}
}
