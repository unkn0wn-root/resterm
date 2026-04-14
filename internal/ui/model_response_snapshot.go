package ui

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) setResponseSnapshotContent(snapshot *responseSnapshot) {
	if snapshot == nil {
		return
	}
	m.responsePending = nil
	m.responseRenderToken = ""
	m.responseLoading = false
	m.responseLoadingFrame = 0
	m.lastResponse = nil
	m.lastGRPC = nil
	m.responseLatest = snapshot

	target := m.responseTargetPane()
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		pane.snapshot = snapshot
		pane.invalidateCaches()
		width := pane.viewport.Width
		if width <= 0 {
			width = defaultResponseViewportWidth
		}
		pane.viewport.SetContent(wrapToWidth(snapshot.pretty, width))
		pane.viewport.GotoTop()
		pane.setCurrPosition()
	}
	m.setLivePane(target)
}

func (m *Model) activateProfileStatsTab(snapshot *responseSnapshot) tea.Cmd {
	if snapshot == nil {
		return nil
	}
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot != snapshot {
			continue
		}
		pane.setActiveTab(responseTabStats)
	}
	return m.syncResponsePanes()
}
