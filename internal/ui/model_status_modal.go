package ui

func (m *Model) openStatusModal(level statusLevel, message string) {
	m.showStatusModal = true
	m.statusModalLevel = level
	m.statusModalMessage = message
	m.closeHelp()
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.showOpenModal = false
	m.showNewFileModal = false
	m.resetStatusModalScroll()
}

func (m *Model) openStatusMessageModal() {
	text, level := m.statusBarMessage()
	m.openStatusModal(level, text)
}

func (m *Model) closeStatusModal() {
	m.showStatusModal = false
	m.statusModalMessage = ""
	m.statusModalLevel = statusInfo
	m.resetStatusModalScroll()
}

func (m *Model) resetStatusModalScroll() {
	if vp := m.statusModalViewport; vp != nil {
		vp.SetYOffset(0)
		vp.GotoTop()
	}
}
