package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	barGlyphFilled = "█"
	barGlyphEmpty  = "░"
)

func renderMeter(filled, width int, style lipgloss.Style) string {
	filled = min(max(filled, 0), width)
	return style.Render(strings.Repeat(barGlyphFilled, filled)) + strings.Repeat(barGlyphEmpty, width-filled)
}
