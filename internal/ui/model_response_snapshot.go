package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// setPaneSnapshot hands a pane its response. The snapshot takes over stream
// ownership from the pane, so a pane pinned to an older response keeps showing
// that response's transcript instead of the one still running.
func (m *Model) setPaneSnapshot(id responsePaneID, snap *responseSnapshot) {
	pane := m.pane(id)
	if pane == nil {
		return
	}
	if snap != nil && snap.id == "" {
		snap.id = nextResponseRenderToken()
	}
	pane.snapshot = snap
	pane.streamID = ""
	pane.tail = snap != nil && snap.streamID != ""
	if snap != nil && snap.stream.failed() {
		pane.stopStreamTail()
	}
	pane.invalidateCaches()
	// One side of the comparison moved, so the neighbour's cached Diff has to
	// be recomputed. It keeps its own response and its Diff tab either way.
	m.invalidatePaneDiffs(id)
	m.ensurePaneActiveTabValid(id, pane)
}

// showSnapshot moves the run's target pane onto snap and makes it the live
// pane. Only that pane changes: a split neighbour keeps what it was showing,
// beyond having to recompute a Diff against the new response.
func (m *Model) showSnapshot(id responsePaneID, snap *responseSnapshot) *responsePaneState {
	m.setPaneSnapshot(id, snap)
	m.setLivePane(id)
	return m.pane(id)
}

func (m *Model) setResponseSnapshotContent(snapshot *responseSnapshot) {
	if snapshot == nil {
		return
	}
	if snapshot.id == "" {
		snapshot.id = nextResponseRenderToken()
	}
	m.resetPendingResponse()
	m.lastResponse = nil
	m.lastGRPC = nil
	m.responseLatest = snapshot

	pane := m.showSnapshot(m.responseTargetForStream(snapshot.streamID), snapshot)
	pane.viewport.SetContent(wrapToWidth(snapshot.pretty, paneWidth(pane)))
	pane.resetScroll()
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
