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
	tabs := m.availableResponseTabsFor(id)
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
	m.closeResponseTabPrompts(pane.activeTab)
	return m.syncResponsePane(id)
}

func (m *Model) activateNextTabFor(id responsePaneID) tea.Cmd {
	pane := m.pane(id)
	if pane == nil {
		return nil
	}
	tabs := m.availableResponseTabsFor(id)
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
	m.closeResponseTabPrompts(pane.activeTab)
	return m.syncResponsePane(id)
}

func (m *Model) closeResponseTabPrompts(active responseTab) {
	if active != responseTabHistory {
		m.closeHistoryFilterPrompt()
	}
	if active != responseTabStream {
		m.closeStreamFilterPrompt()
	}
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
	return m.availableResponseTabsFor(m.responsePaneFocus)
}

// availableResponseTabsFor answers per pane, not per model: Explain, Stream and
// the report tabs belong to whatever that pane is showing, so a split never
// offers a tab that would render the other pane's response.
func (m *Model) availableResponseTabsFor(id responsePaneID) []responseTab {
	var snap *responseSnapshot
	if pane := m.pane(id); pane != nil {
		snap = pane.snapshot
	}
	tabs := []responseTab{responseTabPretty, responseTabRaw, responseTabHeaders}
	if snap != nil && snap.explain.report != nil {
		tabs = append(tabs, responseTabExplain)
	}
	if m.streamIDForPane(id) != "" {
		tabs = append(tabs, responseTabStream)
	}
	if snap != nil && strings.TrimSpace(snap.stats) != "" {
		tabs = append(tabs, responseTabStats)
	}
	if snapshotHasTrace(snap) {
		tabs = append(tabs, responseTabTimeline)
	}
	if m.compareTabAvailable() {
		tabs = append(tabs, responseTabCompare)
	}
	if m.diffAvailable() {
		tabs = append(tabs, responseTabDiff)
	}
	tabs = append(tabs, responseTabHistory)
	return tabs
}

func snapshotHasTrace(snap *responseSnapshot) bool {
	if snap == nil {
		return false
	}
	return snap.timeline != nil || snap.traceSpec != nil && snap.traceSpec.Enabled
}

func responseTabLabelForSnapshot(tab responseTab, snapshot *responseSnapshot) string {
	switch tab {
	case responseTabPretty:
		return "Pretty"
	case responseTabRaw:
		return "Raw"
	case responseTabHeaders:
		return "Headers"
	case responseTabExplain:
		return "Explain"
	case responseTabStream:
		return "Stream"
	case responseTabStats:
		switch snapshotStatsKind(snapshot) {
		case statsReportKindProfile:
			return "Profile"
		case statsReportKindWorkflow:
			return "Workflow"
		default:
			return "Stats"
		}
	case responseTabTimeline:
		return "Timeline"
	case responseTabCompare:
		return "Compare"
	case responseTabDiff:
		return "Diff"
	case responseTabHistory:
		return "History"
	default:
		return "?"
	}
}

func snapshotStatsKind(snapshot *responseSnapshot) statsReportKind {
	if snapshot == nil {
		return statsReportKindNone
	}
	return snapshot.statsKind
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

func (m *Model) snapshotHasTimeline() bool {
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		if snapshotHasTrace(pane.snapshot) {
			return true
		}
	}
	return snapshotHasTrace(m.responseLatest)
}

func (m *Model) compareTabAvailable() bool {
	if m.compareBundle != nil {
		return true
	}
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot == nil {
			continue
		}
		if pane.snapshot.compareBundle != nil {
			return true
		}
	}
	if m.responseLatest != nil && m.responseLatest.compareBundle != nil {
		return true
	}
	return false
}
