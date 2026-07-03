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
	s := m.latencySeries
	if s == nil || s.empty() {
		return m.themeRuntime.inactiveStyle(m.theme.HeaderValue)
	}
	v, _ := s.last()
	return latStyle(m.theme, v)
}

func latStyle(th theme.Theme, d time.Duration) lipgloss.Style {
	st := th.HeaderValue
	if d <= 0 {
		return st
	}

	ok := latOkMax
	wn := latWarnMax
	if wn < ok {
		wn = ok
	}
	if d <= ok {
		return st.Foreground(latFg(th.Success, latOkFg))
	}
	if d <= wn {
		return st.Foreground(latWarnFg)
	}
	return st.Foreground(latFg(th.Error, latErrFg))
}

func latFg(st lipgloss.Style, fb lipgloss.Color) lipgloss.TerminalColor {
	fg := st.GetForeground()
	if fg == nil {
		return fb
	}
	if c, ok := fg.(lipgloss.Color); ok && c == "" {
		return fb
	}
	return fg
}
