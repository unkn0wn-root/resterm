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
	headerWarnFallback = lipgloss.Color("#FFD46A")
	latErrFg           = lipgloss.Color("#FF6E6E")
)

func (m Model) renderLatency() string {
	return m.renderLatencyOn(nil)
}

func (m Model) renderLatencyOn(bg lipgloss.TerminalColor) string {
	s, ok := m.latencySeries.summary()
	if !ok {
		return headerStyleOnBackground(m.latMutedStyle(), bg).
			Render(latLabel + " " + m.latIdleText())
	}

	muted := headerStyleOnBackground(m.latMutedStyle(), bg)
	rs := []rune(s.bars)
	last := len(rs) - 1
	cur := formatLatencyDuration(s.cur)
	p95 := formatLatencyDuration(s.p95)

	return muted.Render(latLabel+" "+string(rs[:last])) +
		headerStyleOnBackground(latStyle(m.theme, s.cur), bg).
			Render(string(rs[last])+" "+cur) +
		muted.Render(" · p95 ") +
		headerStyleOnBackground(latStyle(m.theme, s.p95), bg).Render(p95)
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
		return st.Foreground(foregroundColor(th.HeaderWarn, headerWarnFallback))
	default:
		return st.Foreground(foregroundColor(th.Error, latErrFg))
	}
}
