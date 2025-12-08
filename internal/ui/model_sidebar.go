package ui

func (m *Model) sidebarTabsMode() bool {
	return m.sidebarMode == sidebarModeTabs
}

func (m *Model) sidebarTabEnabled() bool {
	return len(m.workflowItems) > 0 || m.sidebarModeOverride != nil || m.sidebarMode == sidebarModeTabs
}

func (m *Model) showWorkflowStack() bool {
	if len(m.workflowItems) == 0 {
		return false
	}
	if m.sidebarTabsMode() {
		return true
	}
	if m.workflowPinned && !m.workflowsCollapsed {
		return true
	}
	return false
}

func (m *Model) setSidebarMode(mode sidebarMode) bool {
	if mode != sidebarModeStack && mode != sidebarModeTabs {
		return false
	}
	if m.sidebarMode == mode {
		return false
	}
	m.sidebarMode = mode
	return true
}

func (m *Model) chooseSidebarMode(paneHeight int, hasWorkflow bool) sidebarMode {
	return sidebarModeStack
}

func (m *Model) setSidebarTab(tab paneFocus) {
	switch tab {
	case focusFile, focusRequests, focusWorkflows:
		m.sidebarTab = tab
	}
}

func (m *Model) toggleWorkflowPin() bool {
	m.workflowPinned = !m.workflowPinned
	return true
}

func (m *Model) toggleFilesCollapse() bool {
	m.filesCollapsed = !m.filesCollapsed
	if m.filesCollapsed {
		m.autoCollapseFiles = false
	}
	return true
}

func (m *Model) toggleWorkflowsCollapse() bool {
	m.workflowsCollapsed = !m.workflowsCollapsed
	return true
}

func (m *Model) toggleSidebarModeOverride() bool {
	var next sidebarMode
	if m.sidebarModeOverride == nil {
		if m.sidebarMode == sidebarModeTabs {
			next = sidebarModeStack
		} else {
			next = sidebarModeTabs
		}
		m.sidebarModeOverride = &next
		return true
	}
	if m.sidebarModeOverride != nil && *m.sidebarModeOverride == sidebarModeTabs {
		next = sidebarModeStack
	} else {
		next = sidebarModeTabs
	}
	m.sidebarModeOverride = &next
	return true
}

func (m *Model) clearSidebarModeOverride() {
	m.sidebarModeOverride = nil
}

func (m *Model) activeSidebarTab() paneFocus {
	switch m.focus {
	case focusFile, focusRequests, focusWorkflows:
		return m.focus
	}
	if m.sidebarTab == focusWorkflows && len(m.workflowItems) == 0 {
		return focusFile
	}
	if m.sidebarTab == focusRequests || m.sidebarTab == focusWorkflows {
		return m.sidebarTab
	}
	return focusFile
}

func (m *Model) reqCompactMode() bool {
	return m.reqCompact != nil && *m.reqCompact
}

func (m *Model) setReqCompact(v bool) bool {
	if m.reqCompact == nil {
		m.reqCompact = new(bool)
	}
	if *m.reqCompact == v {
		return false
	}
	*m.reqCompact = v
	return true
}

func (m *Model) wfCompactMode() bool {
	return m.wfCompact != nil && *m.wfCompact
}

func (m *Model) setWfCompact(v bool) bool {
	if m.wfCompact == nil {
		m.wfCompact = new(bool)
	}
	if *m.wfCompact == v {
		return false
	}
	*m.wfCompact = v
	return true
}
