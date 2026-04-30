package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderRuleHead(head string, width int, st lipgloss.Style) string {
	if width <= 0 {
		return head
	}
	if rem := width - visibleWidth(head) - 1; rem > 4 {
		return lipgloss.JoinHorizontal(
			lipgloss.Left,
			head,
			" ",
			st.Render(strings.Repeat("─", rem)),
		)
	}
	return head
}
