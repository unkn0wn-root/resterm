package ui

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser"
)

func (m *Model) openStatusModal(level statusLevel, message string) {
	m.showStatusModal = true
	m.statusModalLevel = level
	m.statusModalMessage = displayLines(message)
	m.closeHelp()
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.showOpenModal = false
	m.showNewFileModal = false
	m.resetStatusModalScroll()
}

func (m *Model) openStatusMessageModal() {
	if m.docMatchesEditor() && len(m.doc.Warnings) > 0 {
		message := strings.Join(parser.WarningTexts(m.doc), "\n")
		m.openStatusModal(statusWarn, message)
		return
	}
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
