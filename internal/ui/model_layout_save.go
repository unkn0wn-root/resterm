package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/config"
)

func (m *Model) openLayoutSaveModal() {
	m.showLayoutSaveModal = true
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	if m.showHistoryPreview {
		m.showHistoryPreview = false
	}
	m.closeOpenModal()
	m.closeNewFileModal()
}

func (m *Model) closeLayoutSaveModal() {
	m.showLayoutSaveModal = false
}

func (m *Model) saveLayoutSettings() tea.Cmd {
	layout := m.currentLayoutSettings()
	m.cfg.Settings.Layout = layout
	if err := config.SaveSettings(m.cfg.Settings, m.settingsHandle); err != nil {
		m.closeLayoutSaveModal()
		return func() tea.Msg {
			return statusMsg{text: fmt.Sprintf("layout save error: %v", err), level: statusError}
		}
	}
	m.closeLayoutSaveModal()
	return func() tea.Msg {
		return statusMsg{text: "Layout saved", level: statusSuccess}
	}
}
