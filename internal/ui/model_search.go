package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openSearchPrompt() tea.Cmd {
	if m.showSearchPrompt {
		return nil
	}
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.closeNewFileModal()
	m.closeOpenModal()
	m.showSearchPrompt = true
	m.searchJustOpened = true

	switch m.focus {
	case focusResponse:
		pane := m.focusedPane()
		if pane == nil {
			m.prepareEditorSearchContext()
			break
		}
		m.searchTarget = searchTargetResponse
		m.searchResponsePane = m.responsePaneFocus
		m.searchIsRegex = pane.search.isRegex
		if pane.search.hasQuery() {
			m.searchInput.SetValue(pane.search.query)
		} else {
			m.searchInput.SetValue("")
		}
	default:
		m.prepareEditorSearchContext()
	}

	m.searchInput.CursorEnd()
	return m.searchInput.Focus()
}

func (m *Model) closeSearchPrompt() {
	if !m.showSearchPrompt {
		return
	}
	m.showSearchPrompt = false
	m.searchJustOpened = false
	m.searchInput.Blur()
}

func (m *Model) toggleSearchMode() {
	m.searchIsRegex = !m.searchIsRegex
	mode := "Literal search"
	if m.searchIsRegex {
		mode = "Regex search"
	}
	m.setStatusMessage(statusMsg{text: mode, level: statusInfo})
}

func (m *Model) submitSearchPrompt() tea.Cmd {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		m.setStatusMessage(statusMsg{text: "Enter a search pattern", level: statusWarn})
		return nil
	}
	m.searchInput.SetValue(query)
	m.closeSearchPrompt()

	switch m.searchTarget {
	case searchTargetResponse:
		return m.applyResponseSearch(query, m.searchIsRegex)
	default:
		updated, cmd := m.editor.ApplySearch(query, m.searchIsRegex)
		m.editor = updated
		return cmd
	}
}

func (m *Model) prepareEditorSearchContext() {
	m.searchTarget = searchTargetEditor
	m.searchIsRegex = m.editor.search.isRegex
	if strings.TrimSpace(m.searchInput.Value()) == "" {
		m.searchInput.SetValue(m.editor.search.query)
	}
}

func (m *Model) responseSearchContent(
	paneID responsePaneID,
	tab responseTab,
	width int,
) (string, responseTab, string) {
	content, cacheKey := m.paneContentForTab(paneID, tab)
	if tab == responseTabStats {
		if pane := m.pane(paneID); pane != nil {
			snap := pane.snapshot
			if snap != nil && snap.statsKind == statsReportKindWorkflow &&
				snap.workflowStats != nil {
				render := snap.workflowStats.render(width)
				content = ensureTrailingNewline(render.content)
				cacheKey = responseTabStats
				return content, cacheKey, content
			}
		}
	}
	return content, cacheKey, wrapContentForTab(cacheKey, content, width)
}

func (m *Model) clearResponseSearch() tea.Cmd {
	pane := m.focusedPane()
	if pane == nil {
		return nil
	}
	if !pane.search.clear() {
		return nil
	}
	status := statusCmd(statusInfo, "Search cleared")
	if syncCmd := m.syncResponsePane(m.responsePaneFocus); syncCmd != nil {
		return tea.Batch(syncCmd, status)
	}
	return status
}

func (m *Model) applyResponseSearch(query string, isRegex bool) tea.Cmd {
	paneID := m.searchResponsePane
	if paneID != responsePanePrimary && paneID != responsePaneSecondary {
		paneID = m.responsePaneFocus
		m.searchResponsePane = paneID
	}
	pane := m.pane(paneID)
	if pane == nil {
		return statusCmd(statusWarn, "No response pane available")
	}

	tab := pane.activeTab
	if tab == responseTabHistory {
		tab = pane.ensureContentTab()
	}

	width := pane.viewport.Width
	if width <= 0 {
		width = defaultResponseViewportWidth
	}

	_, cacheKey, wrapped := m.responseSearchContent(paneID, tab, width)
	snapshotID := ""
	snapshotReady := false
	if pane.snapshot != nil {
		snapshotID = pane.snapshot.id
		snapshotReady = pane.snapshot.ready
	}

	pane.search.prepare(query, isRegex, cacheKey, snapshotID, width)

	if !snapshotReady {
		pane.search.computed = false
		pane.search.active = false
		status := statusCmd(
			statusInfo,
			fmt.Sprintf("Search queued for %q; waiting for response", query),
		)
		if syncCmd := m.syncResponsePane(paneID); syncCmd != nil {
			return tea.Batch(syncCmd, status)
		}
		return status
	}

	if err := pane.search.computeMatches(wrapped); err != nil {
		pane.search.invalidate()
		status := statusCmd(statusError, fmt.Sprintf("Invalid regex: %v", err))
		if syncCmd := m.syncResponsePane(paneID); syncCmd != nil {
			return tea.Batch(syncCmd, status)
		}
		return status
	}

	if len(pane.search.matches) == 0 {
		pane.search.active = false
		status := statusCmd(statusWarn, fmt.Sprintf("No matches for %q", query))
		if syncCmd := m.syncResponsePane(paneID); syncCmd != nil {
			return tea.Batch(syncCmd, status)
		}
		return status
	}

	pane.search.active = true
	pane.search.index = 0
	match := pane.search.matches[pane.search.index]
	ensureResponseMatchVisible(&pane.viewport, wrapped, match)
	status := statusCmd(
		statusInfo,
		fmt.Sprintf("Match %d/%d for %q", pane.search.index+1, len(pane.search.matches), query),
	)
	if syncCmd := m.syncResponsePane(paneID); syncCmd != nil {
		return tea.Batch(syncCmd, status)
	}
	return status
}

func (m *Model) advanceResponseSearch() tea.Cmd {
	paneID := m.responsePaneFocus
	pane := m.pane(paneID)
	if pane == nil {
		return statusCmd(statusWarn, "No response pane available")
	}
	if !pane.search.hasQuery() {
		return statusCmd(statusWarn, "No active search")
	}

	tab := pane.activeTab
	if tab == responseTabHistory {
		tab = pane.ensureContentTab()
	}

	width := pane.viewport.Width
	if width <= 0 {
		width = defaultResponseViewportWidth
	}

	_, cacheKey, wrapped := m.responseSearchContent(paneID, tab, width)
	snapshotID := ""
	snapshotReady := false
	if pane.snapshot != nil {
		snapshotID = pane.snapshot.id
		snapshotReady = pane.snapshot.ready
	}
	if !snapshotReady {
		return statusCmd(statusWarn, "Response not ready")
	}

	if pane.search.needsRefresh(snapshotID, cacheKey, width) {
		prevIndex := pane.search.index
		pane.search.prepare(pane.search.query, pane.search.isRegex, cacheKey, snapshotID, width)
		if err := pane.search.computeMatches(wrapped); err != nil {
			pane.search.invalidate()
			return statusCmd(statusError, fmt.Sprintf("Invalid regex: %v", err))
		}
		if len(pane.search.matches) == 0 {
			status := statusCmd(statusWarn, fmt.Sprintf("No matches for %q", pane.search.query))
			if syncCmd := m.syncResponsePane(paneID); syncCmd != nil {
				return tea.Batch(syncCmd, status)
			}
			return status
		}
		if prevIndex >= 0 && prevIndex < len(pane.search.matches) {
			pane.search.index = prevIndex
		} else {
			pane.search.index = 0
		}
	}

	if len(pane.search.matches) == 0 {
		return statusCmd(statusWarn, fmt.Sprintf("No matches for %q", pane.search.query))
	}

	next := pane.search.index + 1
	wrappedAround := false
	if next >= len(pane.search.matches) {
		next = 0
		wrappedAround = true
	}
	pane.search.index = next
	match := pane.search.matches[next]
	ensureResponseMatchVisible(&pane.viewport, wrapped, match)
	pane.search.active = true

	statusText := fmt.Sprintf(
		"Match %d/%d for %q",
		next+1,
		len(pane.search.matches),
		pane.search.query,
	)
	if wrappedAround {
		statusText += " (wrapped)"
	}
	status := statusCmd(statusInfo, statusText)
	if syncCmd := m.syncResponsePane(paneID); syncCmd != nil {
		return tea.Batch(syncCmd, status)
	}
	return status
}

func (m *Model) retreatResponseSearch() tea.Cmd {
	paneID := m.responsePaneFocus
	pane := m.pane(paneID)
	if pane == nil {
		return statusCmd(statusWarn, "No response pane available")
	}
	if !pane.search.hasQuery() {
		return statusCmd(statusWarn, "No active search")
	}

	tab := pane.activeTab
	if tab == responseTabHistory {
		tab = pane.ensureContentTab()
	}

	width := pane.viewport.Width
	if width <= 0 {
		width = defaultResponseViewportWidth
	}

	_, cacheKey, wrapped := m.responseSearchContent(paneID, tab, width)
	snapshotID := ""
	snapshotReady := false
	if pane.snapshot != nil {
		snapshotID = pane.snapshot.id
		snapshotReady = pane.snapshot.ready
	}
	if !snapshotReady {
		return statusCmd(statusWarn, "Response not ready")
	}

	if pane.search.needsRefresh(snapshotID, cacheKey, width) {
		prevIndex := pane.search.index
		pane.search.prepare(pane.search.query, pane.search.isRegex, cacheKey, snapshotID, width)
		if err := pane.search.computeMatches(wrapped); err != nil {
			pane.search.invalidate()
			return statusCmd(statusError, fmt.Sprintf("Invalid regex: %v", err))
		}
		if len(pane.search.matches) == 0 {
			status := statusCmd(statusWarn, fmt.Sprintf("No matches for %q", pane.search.query))
			if syncCmd := m.syncResponsePane(paneID); syncCmd != nil {
				return tea.Batch(syncCmd, status)
			}
			return status
		}
		if prevIndex >= 0 && prevIndex < len(pane.search.matches) {
			pane.search.index = prevIndex
		} else {
			pane.search.index = 0
		}
	}

	if len(pane.search.matches) == 0 {
		return statusCmd(statusWarn, fmt.Sprintf("No matches for %q", pane.search.query))
	}

	prev := pane.search.index - 1
	wrappedAround := false
	if prev < 0 {
		prev = len(pane.search.matches) - 1
		wrappedAround = true
	}
	pane.search.index = prev
	match := pane.search.matches[prev]
	ensureResponseMatchVisible(&pane.viewport, wrapped, match)
	pane.search.active = true

	statusText := fmt.Sprintf(
		"Match %d/%d for %q",
		prev+1,
		len(pane.search.matches),
		pane.search.query,
	)
	if wrappedAround {
		statusText += " (wrapped)"
	}
	status := statusCmd(statusInfo, statusText)
	if syncCmd := m.syncResponsePane(paneID); syncCmd != nil {
		return tea.Batch(syncCmd, status)
	}
	return status
}
