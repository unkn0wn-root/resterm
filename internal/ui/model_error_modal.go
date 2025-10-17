package ui

import "strings"

func (m *Model) openErrorModal(message string) {
	m.showErrorModal = true
	m.errorModalMessage = strings.TrimSpace(message)
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.showOpenModal = false
	m.showNewFileModal = false
}

func (m *Model) closeErrorModal() {
	m.showErrorModal = false
	m.errorModalMessage = ""
}
