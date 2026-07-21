package ui

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func modalKey(key string, close func(), vp *viewport.Model) tea.Cmd {
	switch key {
	case "esc", "enter":
		close()
	case "ctrl+q", "ctrl+d":
		return tea.Quit
	default:
		scrollViewportKey(vp, key)
	}
	return nil
}

// scrollViewportKey applies the shared modal scroll bindings and reports
// whether the key was one of them.
func scrollViewportKey(vp *viewport.Model, key string) bool {
	if vp == nil {
		return false
	}
	switch key {
	case "down", "j":
		vp.ScrollDown(1)
	case "up", "k":
		vp.ScrollUp(1)
	case "pgdown", "ctrl+f":
		vp.ScrollDown(max(vp.Height, 1))
	case "pgup", "ctrl+b", "ctrl+u":
		vp.ScrollUp(max(vp.Height, 1))
	case "home", "g":
		vp.GotoTop()
	case "end", "shift+g", "G":
		vp.GotoBottom()
	default:
		return false
	}
	return true
}

type modalSize struct {
	width   int
	content int
	view    int
	body    int
}

func (m Model) modalSize(maxWidth, maxBody int) modalSize {
	width := min(m.width-6, maxWidth)
	if width < 48 {
		width = max(m.width-4, 36)
	}
	content := max(width-4, 32)
	view := max(content-4, 20)
	body := max(min(m.height-12, maxBody), 8)
	if m.height > 0 {
		body = min(body, max(m.height-6, 4))
	}
	return modalSize{width: width, content: content, view: view, body: body}
}

func (m Model) renderModalBox(title, body, instructions string, width int) string {
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderModalTitle(title, width),
		"",
		body,
		"",
		m.theme.HeaderValue.Padding(0, 2).Render(instructions),
	)
	return m.renderCenteredModal(m.theme.BrowserBorder.Width(width).Render(content))
}
