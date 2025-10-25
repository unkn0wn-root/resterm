package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) activatePrevTabFor(id responsePaneID) tea.Cmd {
	pane := m.pane(id)
	if pane == nil {
		return nil
	}
	tabs := m.availableResponseTabs()
	idx := indexOfResponseTab(tabs, pane.activeTab)
	if idx == -1 {
		pane.setActiveTab(tabs[0])
	} else {
		idx = (idx - 1 + len(tabs)) % len(tabs)
		pane.setActiveTab(tabs[idx])
	}
	if pane.activeTab == responseTabHistory {
		m.historyJumpToLatest = true
	}
	return m.syncResponsePane(id)
}

func (m *Model) activateNextTabFor(id responsePaneID) tea.Cmd {
	pane := m.pane(id)
	if pane == nil {
		return nil
	}
	tabs := m.availableResponseTabs()
	idx := indexOfResponseTab(tabs, pane.activeTab)
	if idx == -1 {
		pane.setActiveTab(tabs[0])
	} else {
		idx = (idx + 1) % len(tabs)
		pane.setActiveTab(tabs[idx])
	}
	if pane.activeTab == responseTabHistory {
		m.historyJumpToLatest = true
	}
	return m.syncResponsePane(id)
}

func indexOfResponseTab(tabs []responseTab, target responseTab) int {
	for i, tab := range tabs {
		if tab == target {
			return i
		}
	}
	return -1
}

func (m *Model) availableResponseTabs() []responseTab {
	tabs := []responseTab{responseTabPretty, responseTabRaw, responseTabHeaders}
	if m.hasActiveStream() {
		tabs = append(tabs, responseTabStream)
	}
	if m.snapshotHasStats() {
		tabs = append(tabs, responseTabStats)
	}
	if m.snapshotHasTimeline() {
		tabs = append(tabs, responseTabTimeline)
	}
	if m.diffAvailable() {
		tabs = append(tabs, responseTabDiff)
	}
	tabs = append(tabs, responseTabHistory)
	return tabs
}

func (m *Model) responseTabLabel(tab responseTab) string {
	switch tab {
	case responseTabPretty:
		return "Pretty"
	case responseTabRaw:
		return "Raw"
	case responseTabHeaders:
		return "Headers"
	case responseTabStream:
		return "Stream"
	case responseTabStats:
		return "Stats"
	case responseTabTimeline:
		return "Timeline"
	case responseTabDiff:
		return "Diff"
	case responseTabHistory:
		return "History"
	default:
		return "?"
	}
}

func (m *Model) diffAvailable() bool {
	if !m.responseSplit {
		return false
	}
	left := m.pane(responsePanePrimary)
	right := m.pane(responsePaneSecondary)
	if left == nil || right == nil {
		return false
	}
	if left.snapshot == nil || right.snapshot == nil {
		return false
	}
	if !left.snapshot.ready || !right.snapshot.ready {
		return false
	}
	return true
}

func (m *Model) snapshotHasStats() bool {
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot == nil {
			continue
		}
		if strings.TrimSpace(pane.snapshot.stats) != "" {
			return true
		}
	}
	if m.responseLatest != nil && strings.TrimSpace(m.responseLatest.stats) != "" {
		return true
	}
	return false
}

func (m *Model) snapshotHasTimeline() bool {
	hasTrace := func(snapshot *responseSnapshot) bool {
		if snapshot == nil {
			return false
		}
		if snapshot.timeline != nil {
			return true
		}
		if snapshot.traceSpec != nil && snapshot.traceSpec.Enabled {
			return true
		}
		return false
	}

	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		if hasTrace(pane.snapshot) {
			return true
		}
	}
	return hasTrace(m.responseLatest)
}
