package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	barGlyphFilled = "█"
	barGlyphEmpty  = "░"
)

func renderMeter(filled, width int, bar, track lipgloss.Style) string {
	filled = min(max(filled, 0), width)
	return bar.Render(strings.Repeat(barGlyphFilled, filled)) +
		track.Render(strings.Repeat(barGlyphEmpty, width-filled))
}
