package ui

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) scrollShortcutToEdge(top bool) (tea.Cmd, bool) {
	switch m.focus {
	case focusEditor:
		return nil, false
	case focusResponse:
		pane := m.focusedPane()
		if pane != nil && pane.activeTab == responseTabHistory {
			m.scrollHistoryToEdge(top)
			return nil, true
		}
		return m.scrollResponseToEdge(top), true
	case focusFile, focusRequests, focusWorkflows:
		return nil, m.scrollNavigatorToEdge(top)
	default:
		return nil, false
	}
}

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

func (m *Model) scrollHistoryToEdge(top bool) {
	if m.focus != focusResponse {
		return
	}
	pane := m.focusedPane()
	if pane == nil || pane.activeTab != responseTabHistory {
		return
	}
	visible := m.historyList.VisibleItems()
	if len(visible) == 0 {
		return
	}
	if top {
		m.historyList.Select(0)
	} else {
		m.historyList.Select(len(visible) - 1)
	}
	m.captureHistorySelection()
}

func (m *Model) scrollNavigatorToEdge(top bool) bool {
	if m.navigator == nil {
		return false
	}
	if top {
		m.navigator.SelectFirst()
	} else {
		m.navigator.SelectLast()
	}
	m.syncNavigatorSelection()
	return true
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
