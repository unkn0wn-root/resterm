package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) openEnvironmentSelector() {
	m.showEnvSelector = true
	m.showHelp = false
	m.showThemeSelector = false
	m.envList.Title = "Environments · " + m.env.Label()

	for i, item := range m.envList.Items() {
		if env, ok := item.(envItem); ok && env.active {
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
		m.clearHelpFilter()
		return
	}
	m.showHelp = true
	m.helpJustOpened = true
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.clearHelpFilter()
}

func (m *Model) applyEnvironmentSelection() tea.Cmd {
	item, ok := m.envList.SelectedItem().(envItem)
	if !ok {
		m.showEnvSelector = false
		return nil
	}

	m.showEnvSelector = false
	sel := m.cfg.Selection
	if item.group != "" {
		sel = sel.WithGroup(item.group, item.profile)
	} else {
		var err error
		if sel, err = m.cfg.Catalog.Select(item.name, nil); err != nil {
			m.setStatusMessage(statusMsg{level: statusError, text: err.Error()})
			return nil
		}
	}
	env, err := m.cfg.Catalog.Resolve(sel)
	if err != nil {
		m.setStatusMessage(statusMsg{level: statusError, text: err.Error()})
		return nil
	}
	if m.env.Scope() == env.Scope() {
		return nil
	}

	m.cfg.Selection = env.Selection()
	m.env = env
	m.envList.SetItems(makeEnvItems(m.cfg.Catalog, m.cfg.Selection))
	m.envList.Title = "Environments · " + env.Label()
	m.latencySeries.reset()
	if gs := m.globalsStore(); gs != nil {
		gs.Clear(env.Scope())
	}
	if fs := m.fileStore(); fs != nil {
		fs.ClearEnv(env.Scope())
	}
	msg := fmt.Sprintf("Environment set to %s", env.Label())
	m.setStatusMessage(statusMsg{level: statusInfo, text: msg})
	m.syncRequestList(m.doc)
	m.syncHistory()
	return nil
}

// selectEnvironment updates m.cfg.Selection and m.env together so the cached
// environment always matches the active selection.
func (m *Model) selectEnvironment(name string, profiles map[string]string) error {
	sel, err := m.cfg.Catalog.Select(name, profiles)
	if err != nil {
		return err
	}
	env, err := m.cfg.Catalog.Resolve(sel)
	if err != nil {
		return err
	}
	m.cfg.Selection = sel
	m.env = env
	return nil
}

func (m *Model) environment(sel vars.Selection) (vars.Environment, error) {
	if sel.Empty() {
		sel = m.cfg.Selection
	}
	return m.cfg.Catalog.Resolve(sel)
}
