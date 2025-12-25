package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/config"
)

func (m *Model) openThemeSelector() {
	m.showThemeSelector = true
	m.showHelp = false
	m.showEnvSelector = false
	if m.showHistoryPreview {
		m.showHistoryPreview = false
	}
	m.refreshThemeList()
	if len(m.themeList.Items()) == 0 {
		m.showThemeSelector = false
		m.setStatusMessage(statusMsg{level: statusWarn, text: "No themes available"})
	}
}

func (m *Model) refreshThemeList() {
	items := makeThemeItems(m.themeCatalog, m.activeThemeKey)
	m.themeList.SetItems(items)
	if len(items) == 0 {
		m.themeList.Select(-1)
		return
	}
	desired := strings.TrimSpace(m.activeThemeKey)
	for idx, item := range items {
		if entry, ok := item.(themeItem); ok && strings.EqualFold(entry.key, desired) {
			m.themeList.Select(idx)
			return
		}
	}
	m.themeList.Select(0)
}

func (m *Model) applyThemeSelection() tea.Cmd {
	item, ok := m.themeList.SelectedItem().(themeItem)
	if !ok {
		m.showThemeSelector = false
		return nil
	}
	m.showThemeSelector = false
	if item.key == "" {
		return nil
	}
	if strings.EqualFold(item.key, m.activeThemeKey) {
		return nil
	}
	def, ok := m.themeCatalog.Get(item.key)
	if !ok {
		m.setStatusMessage(
			statusMsg{level: statusWarn, text: fmt.Sprintf("theme %q unavailable", item.key)},
		)
		return nil
	}

	m.theme = def.Theme
	m.activeThemeKey = def.Key
	m.editor.SetRuneStyler(selectEditorRuneStyler(m.currentFile, m.theme.EditorMetadata))
	m.refreshThemeList()
	m.applyThemeToLists()

	m.cfg.Settings.DefaultTheme = def.Key
	if err := config.SaveSettings(m.cfg.Settings, m.settingsHandle); err != nil {
		m.setStatusMessage(
			statusMsg{level: statusWarn, text: fmt.Sprintf("theme save error: %v", err)},
		)
		return nil
	}
	label := def.DisplayName
	if strings.TrimSpace(label) == "" {
		label = humaniseKey(def.Key)
	}
	m.setStatusMessage(statusMsg{level: statusInfo, text: fmt.Sprintf("Theme set to %s", label)})
	return nil
}
