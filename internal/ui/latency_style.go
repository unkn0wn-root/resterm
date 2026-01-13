package ui

import (
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	latOkMax     = 500 * time.Millisecond
	latWarnMax   = 1000 * time.Millisecond
	latAnimWarnP = 0.3
	latAnimOkP   = 0.65
)

var (
	latOkFg   = lipgloss.Color("#6EF17E")
	latWarnFg = lipgloss.Color("#FFD46A")
	latErrFg  = lipgloss.Color("#FF6E6E")
)

func (m Model) latencyStyle() lipgloss.Style {
	st := m.theme.HeaderValue
	s := m.latencySeries
	if s == nil || s.empty() {
		el := time.Since(m.latAnimStart)
		if m.latAnimOn {
			return latAnimStyle(m.theme, el)
		}
		if !m.latAnimStart.IsZero() {
			return latAnimStyle(m.theme, el)
		}
		return st
	}
	v, ok := s.last()
	if !ok {
		return st
	}
	return latStyle(m.theme, v)
}

func latAnimStyle(th theme.Theme, el time.Duration) lipgloss.Style {
	st := th.HeaderValue
	p := latAnimProgress(el)
	wn, ok := latAnimThresholds()
	if p >= ok {
		return st.Foreground(latFg(th.Success, latOkFg))
	}
	if p >= wn {
		return st.Foreground(latWarnFg)
	}
	return st.Foreground(latFg(th.Error, latErrFg))
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
