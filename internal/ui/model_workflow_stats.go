package ui

import tea "github.com/charmbracelet/bubbletea"

func workflowStatsFromPane(pane *responsePaneState) *workflowStatsView {
	if pane == nil || pane.activeTab != responseTabStats {
		return nil
	}
	snapshot := pane.snapshot
	if snapshot == nil || snapshot.statsKind != statsReportKindWorkflow {
		return nil
	}
	return snapshot.workflowStats
}

func (m *Model) currentWorkflowStats() (*responseSnapshot, *workflowStatsView) {
	pane := m.focusedPane()
	view := workflowStatsFromPane(pane)
	if view == nil {
		return nil, nil
	}
	if pane == nil || pane.snapshot == nil {
		return nil, nil
	}
	return pane.snapshot, view
}

func (m *Model) jumpWorkflowStatsSelection(delta int) tea.Cmd {
	snapshot, view := m.currentWorkflowStats()
	if view == nil {
		return nil
	}
	if !view.move(delta) {
		return nil
	}
	m.invalidateWorkflowStatsCaches(snapshot)
	return m.syncResponsePanes()
}

func (m *Model) jumpWorkflowStatsEdge(top bool) tea.Cmd {
	snapshot, view := m.currentWorkflowStats()
	if view == nil {
		return nil
	}
	if !view.selectEdge(top) {
		return nil
	}
	m.invalidateWorkflowStatsCaches(snapshot)
	return m.syncResponsePanes()
}

func (m *Model) scrollWorkflowStatsDetail(delta int) tea.Cmd {
	snapshot, view := m.currentWorkflowStats()
	if view == nil || delta == 0 {
		return nil
	}
	pane := m.focusedPane()
	if pane == nil {
		return nil
	}
	if !view.scrollDetail(pane.viewport.Width, pane.viewport.Height, delta) {
		return nil
	}
	m.invalidateWorkflowStatsCaches(snapshot)
	return m.syncResponsePanes()
}

func (m *Model) scrollWorkflowStatsDetailEdge(top bool) tea.Cmd {
	snapshot, view := m.currentWorkflowStats()
	if view == nil {
		return nil
	}
	pane := m.focusedPane()
	if pane == nil {
		return nil
	}
	if !view.scrollDetailEdge(pane.viewport.Width, pane.viewport.Height, top) {
		return nil
	}
	m.invalidateWorkflowStatsCaches(snapshot)
	return m.syncResponsePanes()
}

func (m *Model) toggleWorkflowStatsExpansion() tea.Cmd {
	snapshot, view := m.currentWorkflowStats()
	if view == nil {
		return nil
	}
	if !view.toggle() {
		return nil
	}
	m.invalidateWorkflowStatsCaches(snapshot)
	return m.syncResponsePanes()
}

func (m *Model) blurWorkflowStatsDetail() tea.Cmd {
	snapshot, view := m.currentWorkflowStats()
	if view == nil {
		return nil
	}
	if !view.blurDetail() {
		return nil
	}
	m.invalidateWorkflowStatsCaches(snapshot)
	return m.syncResponsePanes()
}

func (m *Model) invalidateWorkflowStatsCaches(snapshot *responseSnapshot) {
	if snapshot == nil {
		return
	}
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot != snapshot {
			continue
		}
		pane.wrapCache[responseTabStats] = cachedWrap{}
		pane.search.markStale()
	}
}

func (m *Model) ensureWorkflowStatsVisible(snapshot *responseSnapshot) {
	if snapshot == nil || snapshot.workflowStats == nil {
		return
	}
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot != snapshot || pane.activeTab != responseTabStats {
			continue
		}
		pane.viewport.SetYOffset(0)
		pane.setCurrPosition()
	}
}

func (m *Model) activateWorkflowStatsView(snapshot *responseSnapshot) tea.Cmd {
	if snapshot == nil || snapshot.workflowStats == nil {
		return nil
	}
	var (
		focusTarget responsePaneID
		haveTarget  bool
	)
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot != snapshot {
			continue
		}
		pane.setActiveTab(responseTabStats)
		if !haveTarget {
			focusTarget = id
			haveTarget = true
		}
	}
	var focusCmd tea.Cmd
	if haveTarget && m.focusVisible(focusResponse) {
		m.focusResponsePane(focusTarget)
		focusCmd = m.setFocus(focusResponse)
	}
	return batchCommands(focusCmd, m.syncResponsePanes())
}
