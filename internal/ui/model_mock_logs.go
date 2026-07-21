package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/mock"
)

func (m *Model) openMockLogs() tea.Cmd {
	if m.activeMockServer() == nil {
		return statusCmd(statusInfo, "Mock server is stopped")
	}
	m.showMockLogs = true
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	m.syncMockLogs()
	if m.mockLogsViewport != nil {
		m.mockLogsViewport.GotoBottom()
	}
	return nil
}

func (m *Model) closeMockLogs() {
	m.showMockLogs = false
}

func (m *Model) syncMockLogs() {
	if m.mockLogsViewport == nil {
		vp := viewport.New(0, 0)
		m.mockLogsViewport = &vp
	}
	follow := m.mockLogsViewport.AtBottom()
	m.mockLogsViewport.SetContent(m.mockLogText())
	if follow {
		m.mockLogsViewport.GotoBottom()
	}
}

func (m *Model) mockLogText() string {
	server := m.activeMockServer()
	if server == nil {
		return "Mock server is stopped."
	}
	logs := server.Logs()
	if len(logs) == 0 {
		return "No mock requests yet."
	}

	var b strings.Builder
	for i, event := range logs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(mockLogLine(event))
	}
	return b.String()
}

func mockLogLine(e mock.Event) string {
	when := e.Time.Format("15:04:05")
	if e.Reload {
		if e.Error == "" {
			return when + " RELOAD ok"
		}
		return when + " RELOAD error  " + oneLine(e.Error)
	}
	name := e.ScenarioLabel()
	if name == "" {
		name = e.Route
	}
	if name == "" {
		name = "-"
	}
	return fmt.Sprintf(
		"%s %-7s %3d %-24s %s  %s",
		when,
		e.Method,
		e.Status,
		truncateRunes(name, 24),
		e.Target,
		e.Duration.Round(time.Microsecond),
	)
}

func (m *Model) handleMockLogsKey(msg tea.KeyMsg) tea.Cmd {
	if key := msg.String(); key != "c" {
		return modalKey(key, m.closeMockLogs, m.mockLogsViewport)
	}
	if server := m.activeMockServer(); server != nil {
		server.ClearLogs()
	}
	m.syncMockLogs()
	return nil
}

func (m Model) renderMockLogsModal() string {
	size := m.modalSize(120, 30)
	var body string
	if vp := m.mockLogsViewport; vp != nil {
		if vp.Width != size.view || vp.Height != size.body {
			follow := vp.AtBottom()
			vp.Width = size.view
			vp.Height = size.body
			vp.SetContent(m.mockLogText())
			if follow {
				vp.GotoBottom()
			}
		}
		body = vp.View()
	} else {
		body = m.mockLogText()
	}
	bodyView := lipgloss.NewStyle().Padding(0, 2).Width(size.content).Render(body)
	title := "Mock Requests"
	if server := m.activeMockServer(); server != nil {
		title += " - " + server.Addr()
	}
	instructions := fmt.Sprintf(
		"%s Close  %s Clear  j/k Scroll",
		m.theme.CommandBarHint.Render("Esc"),
		m.theme.CommandBarHint.Render("c"),
	)
	return m.renderModalBox(title, bodyView, instructions, size.width)
}
