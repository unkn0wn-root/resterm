package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const latAnimDur = 900 * time.Millisecond

var latAnimTick = latAnimDur / time.Duration(len(latencyLevels))

func (m *Model) startLatAnim() {
	m.latAnimOn = true
	m.latAnimT0 = time.Now()
}

func (m Model) scheduleLatAnim() tea.Cmd {
	if !m.latAnimOn || !m.latencySeries.empty() {
		return nil
	}
	return tea.Tick(latAnimTick, func(time.Time) tea.Msg {
		return latAnimMsg{}
	})
}

func (m *Model) handleLatAnim() tea.Cmd {
	if !m.latAnimOn || !m.latencySeries.empty() || m.latAnimP() >= 1 {
		m.latAnimOn = false
		return nil
	}
	return m.scheduleLatAnim()
}

func (m Model) latAnimP() float64 {
	return min(float64(time.Since(m.latAnimT0))/float64(latAnimDur), 1)
}

// latClimb draws one frame of the startup animation: bars rise one by one
// into the placeholder ramp, so at p=1 the frame matches latPlaceholder.
func latClimb(p float64) string {
	n := len(latencyLevels)
	k := min(int(p*float64(n)), n-1)
	return string(latencyLevels[:k+1]) + latFill(n-1-k)
}
