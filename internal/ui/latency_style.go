package ui

import (
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	latOKMax   = 500 * time.Millisecond
	latWarnMax = time.Second
	latLabel   = "RTT"
)

var (
	latWarnFg = lipgloss.Color("#FFD46A")
	latErrFg  = lipgloss.Color("#FF6E6E")
)

func (m Model) renderLatency() string {
	s, ok := m.latencySeries.summary()
	if !ok {
		return m.latMutedStyle().Render(latLabel + " " + m.latIdleText())
	}

	muted := m.latMutedStyle()
	rs := []rune(s.bars)
	last := len(rs) - 1
	cur := formatLatencyDuration(s.cur)
	p95 := formatLatencyDuration(s.p95)

	return muted.Render(latLabel+" "+string(rs[:last])) +
		latStyle(m.theme, s.cur).Render(string(rs[last])+" "+cur) +
		muted.Render(" · p95 ") +
		latStyle(m.theme, s.p95).Render(p95)
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
