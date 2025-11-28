package ui

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) currentWorkflowStats() (*responseSnapshot, *workflowStatsView) {
	pane := m.focusedPane()
	if pane == nil || pane.activeTab != responseTabStats {
		return nil, nil
	}
	snapshot := pane.snapshot
	if snapshot == nil || snapshot.statsKind != statsReportKindWorkflow {
		return nil, nil
	}
	view := snapshot.workflowStats
	if view == nil {
		return nil, nil
	}
	return snapshot, view
}

func (m *Model) jumpWorkflowStatsSelection(delta int) tea.Cmd {
	snapshot, view := m.currentWorkflowStats()
	if view == nil {
		return nil
	}
	if !view.move(delta) {
		return nil
	}

	needsAlign := false
	statsPaneCount := 0
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot != snapshot || pane.activeTab != responseTabStats {
			continue
		}
		statsPaneCount++
		width := pane.viewport.Width
		if width <= 0 {
			width = defaultResponseViewportWidth
		}
		render := view.render(width)
		if view.selected < 0 || view.selected >= len(render.metrics) {
			continue
		}
		metric := render.metrics[view.selected]
		height := pane.viewport.Height
		if height <= 0 {
			height = 1
		}
		offset := pane.viewport.YOffset
		if metric.start < offset || metric.end > offset+height-1 {
			needsAlign = true
		}
	}

	view.alignTopSelected = needsAlign
	if needsAlign {
		view.alignTopRemaining = statsPaneCount
		if view.alignTopRemaining == 0 {
			view.alignTopRemaining = 1
		}
	} else {
		view.alignTopRemaining = 0
		m.ensureWorkflowStatsVisible(snapshot)
	}
	m.invalidateWorkflowStatsCaches(snapshot)
	return m.syncResponsePanes()
}

func (m *Model) moveWorkflowStatsSelection(delta int) tea.Cmd {
	snapshot, view := m.currentWorkflowStats()
	if view == nil {
		return nil
	}
	if !view.move(delta) {
		return nil
	}
	m.invalidateWorkflowStatsCaches(snapshot)
	m.ensureWorkflowStatsVisible(snapshot)
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
	view.alignTopSelected = true
	view.alignTopRemaining = m.workflowStatsPaneCount(snapshot)
	if view.alignTopRemaining == 0 {
		view.alignTopRemaining = 1
	}
	m.invalidateWorkflowStatsCaches(snapshot)
	m.ensureWorkflowStatsVisible(snapshot)
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
	}
}

func (m *Model) ensureWorkflowStatsVisible(snapshot *responseSnapshot) {
	if snapshot == nil || snapshot.workflowStats == nil {
		return
	}
	view := snapshot.workflowStats
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot != snapshot {
			continue
		}
		width := pane.viewport.Width
		if width <= 0 {
			width = defaultResponseViewportWidth
		}
		render := view.render(width)
		view.ensureVisible(pane, render)
		pane.setCurrPosition()
	}
}

func (m *Model) activateWorkflowStatsView(snapshot *responseSnapshot) tea.Cmd {
	if snapshot == nil || snapshot.workflowStats == nil {
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

func (m *Model) workflowStatsPaneCount(snapshot *responseSnapshot) int {
	if snapshot == nil || snapshot.workflowStats == nil {
		return 0
	}
	count := 0
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot != snapshot || pane.activeTab != responseTabStats {
			continue
		}
		count++
	}
	return count
}
