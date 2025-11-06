package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type paneToggleResult struct {
	changed bool
	blocked bool
}

func regionFromFocus(f paneFocus) paneRegion {
	switch f {
	case focusFile, focusRequests, focusWorkflows:
		return paneRegionSidebar
	case focusResponse:
		return paneRegionResponse
	default:
		return paneRegionEditor
	}
}

func (m *Model) collapseState(r paneRegion) bool {
	switch r {
	case paneRegionSidebar:
		return m.sidebarCollapsed
	case paneRegionResponse:
		return m.responseCollapsed
	default:
		return m.editorCollapsed
	}
}

func (m *Model) setCollapseState(r paneRegion, collapsed bool) paneToggleResult {
	prev := m.collapseState(r)
	if prev == collapsed {
		return paneToggleResult{changed: false}
	}
	expanded := m.expandedRegionCount()
	if collapsed && !prev && expanded == 1 {
		return paneToggleResult{blocked: true}
	}
	switch r {
	case paneRegionSidebar:
		m.sidebarCollapsed = collapsed
	case paneRegionResponse:
		m.responseCollapsed = collapsed
	default:
		m.editorCollapsed = collapsed
	}
	if collapsed && m.zoomActive && m.zoomRegion == r {
		m.zoomActive = false
	}
	return paneToggleResult{changed: true}
}

func (m *Model) expandedRegionCount() int {
	count := 0
	if !m.sidebarCollapsed {
		count++
	}
	if !m.editorCollapsed {
		count++
	}
	if !m.responseCollapsed {
		count++
	}
	if count == 0 {
		return 0
	}
	return count
}

func (m *Model) effectiveRegionCollapsed(r paneRegion) bool {
	if m.zoomActive {
		return m.zoomRegion != r
	}
	return m.collapseState(r)
}

func (m *Model) setZoomRegion(r paneRegion) bool {
	if r == paneRegionSidebar {
		return false
	}
	if m.zoomActive && m.zoomRegion == r {
		return false
	}
	m.zoomActive = true
	m.zoomRegion = r
	if state := m.setCollapseState(r, false); state.blocked {
		m.zoomActive = false
		return false
	}
	return true
}

func (m *Model) clearZoom() bool {
	if !m.zoomActive {
		return false
	}
	m.zoomActive = false
	return true
}

func (m *Model) collapsedStatusLabel(r paneRegion) string {
	switch r {
	case paneRegionSidebar:
		return "Sidebar"
	case paneRegionResponse:
		return "Response"
	default:
		return "Editor"
	}
}

func (m *Model) togglePaneCollapse(r paneRegion) tea.Cmd {
	current := m.collapseState(r)
	res := m.setCollapseState(r, !current)
	if res.blocked {
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("Cannot hide %s", strings.ToLower(m.collapsedStatusLabel(r))), level: statusWarn})
		return nil
	}
	if !res.changed {
		return nil
	}
	label := m.collapsedStatusLabel(r)
	var msg string
	if current {
		msg = fmt.Sprintf("%s restored", label)
	} else {
		msg = fmt.Sprintf("%s minimized", label)
	}
	m.setStatusMessage(statusMsg{text: msg, level: statusInfo})
	return m.applyLayout()
}

func (m *Model) toggleZoomForRegion(r paneRegion) tea.Cmd {
	if r == paneRegionSidebar {
		m.setStatusMessage(statusMsg{text: "Requests pane cannot be zoomed", level: statusWarn})
		return nil
	}
	if m.zoomActive && m.zoomRegion == r {
		return m.clearZoomCmd()
	}
	if !m.setZoomRegion(r) {
		return nil
	}
	label := m.collapsedStatusLabel(r)
	m.setStatusMessage(statusMsg{text: fmt.Sprintf("Zooming %s", label), level: statusInfo})
	return m.applyLayout()
}

func (m *Model) clearZoomCmd() tea.Cmd {
	if !m.clearZoom() {
		m.setStatusMessage(statusMsg{text: "Zoom already cleared", level: statusInfo})
		return nil
	}
	m.setStatusMessage(statusMsg{text: "Zoom cleared", level: statusInfo})
	return m.applyLayout()
}
