package ui

import "strings"

func (m *Model) setStatusMessage(msg statusMsg) {
	m.statusMessage = msg
	if msg.level == statusError && strings.TrimSpace(msg.text) != "" {
		m.openErrorModal(msg.text)
	}
}
