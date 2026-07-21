package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type mockVerifyMsg struct {
	server  *mock.Server
	results []mock.VerificationResult
}

func (m *Model) resetMockSequences(args []string) tea.Cmd {
	server := m.activeMockServer()
	if server == nil {
		return statusCmd(statusInfo, "Mock server is stopped")
	}
	name := ""
	if len(args) == 1 {
		name = strings.TrimSpace(args[0])
		if name == "" || !restfile.ValidMockName(name) {
			return statusCmd(statusWarn, fmt.Sprintf("Invalid mock sequence name %q", args[0]))
		}
	}
	reset := server.ResetSequences(name)
	if name != "" && reset == 0 {
		return statusCmd(statusWarn, fmt.Sprintf("Mock sequence %q was not found", name))
	}
	return statusCmd(statusSuccess, fmt.Sprintf("Reset %d mock sequence cursor(s)", reset))
}

// verifyMockRequests counts journal matches off the update loop. The modal
// opens when the mockVerifyMsg result arrives.
func (m *Model) verifyMockRequests() tea.Cmd {
	server := m.activeMockServer()
	if server == nil {
		return statusCmd(statusInfo, "Mock server is stopped")
	}
	expectations := server.Expectations()
	if len(expectations) == 0 {
		return statusCmd(statusWarn, "No # @expect declarations are active")
	}
	return func() tea.Msg {
		return mockVerifyMsg{
			server:  server,
			results: mock.Verify(context.Background(), server, expectations),
		}
	}
}

func (m *Model) handleMockVerify(msg mockVerifyMsg) tea.Cmd {
	if msg.server != m.mock.server {
		return nil
	}
	var content strings.Builder
	passed := 0
	for i, result := range msg.results {
		if i > 0 {
			content.WriteByte('\n')
		}
		status := "PASS"
		switch {
		case result.Err != nil:
			status = "ERROR"
		case !result.Passed:
			status = "FAIL"
		default:
			passed++
		}
		fmt.Fprintf(&content, "%-6s %s\n       %s", status, result.Expectation.Label(), result.Detail())
	}
	m.mockVerificationText = content.String()
	m.showMockVerification = true
	m.showMockLogs = false
	m.showHelp = false
	m.showEnvSelector = false
	m.showThemeSelector = false
	if m.mockVerificationViewport == nil {
		vp := viewport.New(0, 0)
		m.mockVerificationViewport = &vp
	}
	m.mockVerificationViewport.SetContent(m.mockVerificationText)
	m.mockVerificationViewport.GotoTop()
	level := statusSuccess
	if passed != len(msg.results) {
		level = statusWarn
	}
	return statusCmd(level, fmt.Sprintf("Mock verification: %d/%d passed", passed, len(msg.results)))
}

func (m *Model) closeMockVerification() {
	m.showMockVerification = false
	m.mockVerificationText = ""
}

func (m Model) renderMockVerificationModal() string {
	size := m.modalSize(110, 24)
	body := m.mockVerificationText
	if vp := m.mockVerificationViewport; vp != nil {
		if vp.Width != size.view || vp.Height != size.body {
			vp.Width = size.view
			vp.Height = size.body
			vp.SetContent(m.mockVerificationText)
		}
		body = vp.View()
	}
	bodyView := lipgloss.NewStyle().Padding(0, 2).Width(size.content).Render(body)
	instructions := fmt.Sprintf(
		"%s / %s Close  j/k Scroll",
		m.theme.CommandBarHint.Render("Esc"),
		m.theme.CommandBarHint.Render("Enter"),
	)
	return m.renderModalBox("Mock Verification", bodyView, instructions, size.width)
}
