package ui

import (
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	latOkMax   = 500 * time.Millisecond
	latWarnMax = 1000 * time.Millisecond
)

var (
	latOkFg   = lipgloss.Color("#6EF17E")
	latWarnFg = lipgloss.Color("#FFD46A")
	latErrFg  = lipgloss.Color("#FF6E6E")
)

func (m Model) latencyStyle() lipgloss.Style {
	if m.latencySeries.empty() {
		return m.themeRuntime.inactiveStyle(m.theme.HeaderValue)
	}
	return latStyle(m.theme, m.latencySeries.last())
}

func latStyle(th theme.Theme, d time.Duration) lipgloss.Style {
	st := th.HeaderValue
	switch {
	case d <= latOkMax:
		return st.Foreground(latFg(th.Success, latOkFg))
	case d <= latWarnMax:
		return st.Foreground(latWarnFg)
	default:
		return st.Foreground(latFg(th.Error, latErrFg))
	}
}

func latFg(st lipgloss.Style, fb lipgloss.Color) lipgloss.TerminalColor {
	if fg := st.GetForeground(); theme.ColorDefined(fg) {
		return fg
	}
	return fb
}
