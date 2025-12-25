package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const compareHeaderLineCount = 4

func (m *Model) moveCompareFocus(delta int) tea.Cmd {
	pane := m.focusedPane()
	return m.setCompareFocusAt(pane, m.compareRowIndex+delta)
}

func (m *Model) handleCompareTabKey(msg tea.KeyMsg, pane *responsePaneState) tea.Cmd {
	if pane == nil || pane.activeTab != responseTabCompare {
		return nil
	}
	bundle := m.compareBundleForPane(pane)
	if bundle == nil || len(bundle.Rows) == 0 {
		return nil
	}
	switch msg.String() {
	case "down", "j":
		return m.moveCompareFocus(1)
	case "up", "k":
		return m.moveCompareFocus(-1)
	case "pgdown":
		step := pane.viewport.Height - 1
		if step < 1 {
			step = 1
		}
		return m.moveCompareFocus(step)
	case "pgup":
		step := pane.viewport.Height - 1
		if step < 1 {
			step = 1
		}
		return m.moveCompareFocus(-step)
	case "home":
		return m.setCompareFocusAt(pane, 0)
	case "end":
		return m.setCompareFocusAt(pane, len(bundle.Rows)-1)
	case "enter":
		return m.selectCompareFocus()
	}
	return nil
}

func (m *Model) setCompareFocusAt(pane *responsePaneState, index int) tea.Cmd {
	bundle := m.compareBundleForPane(pane)
	if bundle == nil || len(bundle.Rows) == 0 {
		return nil
	}
	if index < 0 {
		index = 0
	}
	if index >= len(bundle.Rows) {
		index = len(bundle.Rows) - 1
	}
	if index == m.compareRowIndex && m.compareFocusedEnv != "" {
		return nil
	}
	m.compareRowIndex = index
	m.compareFocusedEnv = strings.TrimSpace(bundle.Rows[index].Result.Environment)
	m.invalidateCompareTabCaches()
	m.ensureCompareRowVisible(pane, bundle)
	return m.syncResponsePane(m.responsePaneFocus)
}

func (m *Model) selectCompareFocus() tea.Cmd {
	paneID := m.responsePaneFocus
	pane := m.pane(paneID)
	bundle := m.compareBundleForPane(pane)
	if bundle == nil || len(bundle.Rows) == 0 {
		return nil
	}
	targetEnv := strings.TrimSpace(m.compareFocusedEnv)
	if targetEnv == "" && m.compareRowIndex >= 0 && m.compareRowIndex < len(bundle.Rows) {
		targetEnv = strings.TrimSpace(bundle.Rows[m.compareRowIndex].Result.Environment)
	}
	if targetEnv == "" {
		return nil
	}
	baselineEnv := strings.TrimSpace(bundle.Baseline)
	if baselineEnv == "" && len(bundle.Rows) > 0 {
		baselineEnv = strings.TrimSpace(bundle.Rows[0].Result.Environment)
	}
	targetSnap := m.compareSnapshot(targetEnv)
	if targetSnap == nil {
		m.setStatusMessage(
			statusMsg{
				text:  fmt.Sprintf("Response for %s unavailable", targetEnv),
				level: statusWarn,
			},
		)
		return nil
	}
	baselineSnap := m.compareSnapshot(baselineEnv)
	if baselineSnap == nil {
		baselineSnap = targetSnap
	}
	var cmds []tea.Cmd
	if cmd := m.ensureCompareSplit(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	primary := m.pane(responsePanePrimary)
	secondary := m.pane(responsePaneSecondary)
	if primary != nil {
		primary.snapshot = targetSnap
		primary.followLatest = false
		primary.invalidateCaches()
		primary.setActiveTab(responseTabDiff)
	}
	if secondary != nil {
		secondary.snapshot = baselineSnap
		secondary.followLatest = false
		secondary.invalidateCaches()
	}
	m.compareSelectedEnv = targetEnv
	m.compareFocusedEnv = targetEnv
	m.compareRowIndex = compareRowIndexForEnv(bundle, targetEnv)
	m.invalidateCompareTabCaches()
	m.ensureCompareRowVisible(pane, bundle)
	m.setStatusMessage(
		statusMsg{text: fmt.Sprintf("Compare %s ↔ %s", targetEnv, baselineEnv), level: statusInfo},
	)
	cmds = append(cmds, m.syncResponsePanes())
	return tea.Batch(cmds...)
}

func (m *Model) ensureCompareSplit() tea.Cmd {
	if m.responseSplit {
		return nil
	}
	orientation := responseSplitHorizontal
	if m.mainSplitOrientation == mainSplitHorizontal {
		orientation = responseSplitVertical
	}
	return m.enableResponseSplit(orientation)
}

func (m *Model) compareBundleForPane(pane *responsePaneState) *compareBundle {
	if pane != nil && pane.snapshot != nil && pane.snapshot.compareBundle != nil {
		return pane.snapshot.compareBundle
	}
	return m.compareBundle
}

func (m *Model) ensureCompareRowVisible(pane *responsePaneState, bundle *compareBundle) {
	if pane == nil || bundle == nil || pane.viewport.Height <= 0 {
		return
	}
	if pane.tabScroll == nil {
		pane.tabScroll = make(map[responseTab]int)
	}
	targetLine := compareHeaderLineCount + m.compareRowIndex
	if targetLine < 0 {
		targetLine = 0
	}
	height := pane.viewport.Height
	offset := pane.tabScroll[responseTabCompare]
	if targetLine < offset {
		pane.tabScroll[responseTabCompare] = targetLine
	} else if targetLine >= offset+height {
		pane.tabScroll[responseTabCompare] = targetLine - height + 1
	}
}

func (m *Model) invalidateCompareTabCaches() {
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.wrapCache == nil {
			continue
		}
		pane.wrapCache[responseTabCompare] = cachedWrap{}
	}
}

func compareRowIndexForEnv(bundle *compareBundle, env string) int {
	if bundle == nil || len(bundle.Rows) == 0 {
		return 0
	}
	trimmed := strings.ToLower(strings.TrimSpace(env))
	if trimmed == "" {
		return 0
	}
	for idx := range bundle.Rows {
		rowEnv := ""
		if bundle.Rows[idx].Result != nil {
			rowEnv = strings.ToLower(strings.TrimSpace(bundle.Rows[idx].Result.Environment))
		}
		if rowEnv != "" && rowEnv == trimmed {
			return idx
		}
	}
	return 0
}
