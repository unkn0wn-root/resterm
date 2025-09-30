package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) openSearchPrompt() tea.Cmd {
	if m.showSearchPrompt {
		return nil
	}
	m.showHelp = false
	m.showEnvSelector = false
	m.closeNewFileModal()
	m.closeOpenModal()
	m.showSearchPrompt = true
	m.searchIsRegex = m.editor.search.isRegex
	if strings.TrimSpace(m.searchInput.Value()) == "" {
		m.searchInput.SetValue(m.editor.search.query)
	}
	m.searchInput.CursorEnd()
	return m.searchInput.Focus()
}

func (m *Model) closeSearchPrompt() {
	if !m.showSearchPrompt {
		return
	}
	m.showSearchPrompt = false
	m.searchInput.Blur()
}

func (m *Model) toggleSearchMode() {
	m.searchIsRegex = !m.searchIsRegex
	mode := "Literal search"
	if m.searchIsRegex {
		mode = "Regex search"
	}
	m.setStatusMessage(statusMsg{text: mode, level: statusInfo})
}

func (m *Model) submitSearchPrompt() tea.Cmd {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		m.setStatusMessage(statusMsg{text: "Enter a search pattern", level: statusWarn})
		return nil
	}
	m.searchInput.SetValue(query)
	m.closeSearchPrompt()
	updated, cmd := m.editor.ApplySearch(query, m.searchIsRegex)
	m.editor = updated
	return cmd
}
