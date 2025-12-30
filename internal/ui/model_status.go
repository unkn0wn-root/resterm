package ui

import "strings"

func (m *Model) setStatusMessage(msg statusMsg) {
	m.statusMessage = msg
	m.syncPulseBase(msg)
	m.handleStatusModal(msg)
}

func (m *Model) syncPulseBase(msg statusMsg) {
	if msg.level != statusWarn && msg.level != statusError {
		return
	}
	if !m.statusPulseOn && !m.hasActiveRun() {
		return
	}
	txt := strings.TrimSpace(msg.text)
	if txt == "" {
		return
	}
	m.statusPulseBase = txt
}

func (m *Model) handleStatusModal(msg statusMsg) {
	show := msg.level == statusError && strings.TrimSpace(msg.text) != "" &&
		!m.suppressNextErrorModal
	m.suppressNextErrorModal = false
	if show {
		m.openErrorModal(msg.text)
	}
}
