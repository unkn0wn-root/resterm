package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openEnvironmentSelector() {
	m.showEnvSelector = true
	m.showHelp = false
	if m.cfg.EnvironmentName == "" {
		if len(m.envList.Items()) > 0 {
			m.envList.Select(0)
		}
		return
	}

	for i, item := range m.envList.Items() {
		if env, ok := item.(envItem); ok && env.name == m.cfg.EnvironmentName {
			m.envList.Select(i)
			return
		}
	}

	if len(m.envList.Items()) > 0 {
		m.envList.Select(0)
	}
}

func (m *Model) toggleHelp() {
	if m.showHelp {
		m.showHelp = false
		m.helpJustOpened = false
		return
	}
	m.showHelp = true
	m.helpJustOpened = true
	m.showEnvSelector = false
}

func (m *Model) applyEnvironmentSelection() tea.Cmd {
	item, ok := m.envList.SelectedItem().(envItem)
	if !ok {
		m.showEnvSelector = false
		return nil
	}

	m.showEnvSelector = false
	if m.cfg.EnvironmentName == item.name {
		return nil
	}

	m.cfg.EnvironmentName = item.name
	m.statusMessage = statusMsg{text: fmt.Sprintf("Environment set to %s", item.name), level: statusInfo}
	m.syncHistory()
	return nil
}
