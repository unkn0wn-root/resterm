package ui

import "strings"

func (m *Model) setStatusMessage(msg statusMsg) {
	msg.text = strings.TrimSpace(msg.text)
	m.statusMessage = msg
	m.syncPulseBase(msg)
	m.handleStatusModal(msg)
}

func (m *Model) clearStatusMessages(texts ...string) {
	for _, text := range texts {
		if text != "" && m.statusMessage.text == text {
			m.setStatusMessage(statusMsg{})
			return
		}
	}
}

func (m *Model) syncPulseBase(msg statusMsg) {
	if msg.level != statusWarn && msg.level != statusError {
		return
	}
	if !m.statusPulseOn && !m.hasActiveRun() {
		return
	}
	if msg.text == "" {
		return
	}
	m.statusPulseBase = msg.text
}

// Errors always open the modal; warnings open it only when the bar truncates them.
func (m *Model) handleStatusModal(msg statusMsg) {
	if msg.text == "" || msg.noModal {
		return
	}
	switch msg.level {
	case statusError:
		m.openStatusModal(msg.level, msg.text)
	case statusWarn:
		if !m.statusBarFits(msg.text, msg.level) {
			m.openStatusModal(msg.level, msg.text)
		}
	}
}
