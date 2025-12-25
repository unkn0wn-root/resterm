package ui

import "strings"

func (m *Model) setStatusMessage(msg statusMsg) {
	m.statusMessage = msg
	showModal := msg.level == statusError && strings.TrimSpace(msg.text) != "" &&
		!m.suppressNextErrorModal
	m.suppressNextErrorModal = false
	if showModal {
		m.openErrorModal(msg.text)
	}
}
