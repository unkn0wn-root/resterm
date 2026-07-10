package ui

import (
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	latOKMax   = 500 * time.Millisecond
	latWarnMax = time.Second
	latLabel   = "Latency "
)

var (
	latWarnFg = lipgloss.Color("#FFD46A")
	latErrFg  = lipgloss.Color("#FF6E6E")
)

func (m Model) renderLatency() string {
	s, ok := m.latencySeries.summary()
	if !ok {
		return m.latMutedStyle().Render(latLabel + m.latencyText())
	}

	muted := m.latMutedStyle()
	curSt := latStyle(m.theme, s.cur)
	p95St := latStyle(m.theme, s.p95)
	cur := formatLatencyDuration(s.cur)
	p95 := formatLatencyDuration(s.p95)

	return muted.Render(latLabel+s.hist) +
		curSt.Render(string(s.last)+" "+cur) +
		muted.Render(latP95Sep) +
		p95St.Render(p95)
}

func (m Model) latMutedStyle() lipgloss.Style {
	if m.themeRuntime.isLight() {
		return m.themeRuntime.subtleTextStyle(m.theme)
	}
	return m.theme.HeaderValue.Faint(true)
}

func latStyle(th theme.Theme, d time.Duration) lipgloss.Style {
	st := th.HeaderValue
	switch {
	case d <= latOKMax:
		return st
	case d <= latWarnMax:
		return st.Foreground(latWarnFg)
	default:
		return st.Foreground(foregroundColor(th.Error, latErrFg))
	}
}
