package ui

import (
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	latAnimDuration = 3600 * time.Millisecond
	latAnimMinDelay = 80 * time.Millisecond
	latAnimMaxDelay = 240 * time.Millisecond
	latAnimStartHz  = 1.2
	latAnimEndHz    = 0.6
	latAnimStepMax  = 0.75
	latAnimStepExp  = 1.8
)

func (m *Model) initLatencyAnim() {
	m.latAnimOn = true
	m.latAnimSeq++
	m.latAnimFrame = 0
	m.latAnimStart = time.Now()
}

func (m *Model) stopLatencyAnim() {
	m.latAnimOn = false
	m.latAnimFrame = 0
}

func (m *Model) addLatency(d time.Duration) {
	if m.latencySeries == nil || d <= 0 {
		return
	}
	m.latencySeries.add(d)
	m.stopLatencyAnim()
}

func (m Model) latencyText() string {
	if m.latencySeries == nil {
		return ""
	}
	if !m.latencySeries.empty() {
		return m.latencySeries.render()
	}
	if m.latAnimOn {
		return latencyAnimText(time.Since(m.latAnimStart), m.latencySeries.cap)
	}
	return latencyPlaceholder
}

func (m Model) latencyAnimTickCmd() tea.Cmd {
	s := m.latencySeries
	if s == nil || !m.latAnimOn || !s.empty() {
		return nil
	}

	el := time.Since(m.latAnimStart)
	seq := m.latAnimSeq
	if el >= latAnimDuration {
		return func() tea.Msg {
			return latencyAnimMsg{seq: seq}
		}
	}

	d := latencyAnimDelay(el)
	return tea.Tick(d, func(time.Time) tea.Msg {
		return latencyAnimMsg{seq: seq}
	})
}

func (m *Model) handleLatencyAnim(msg latencyAnimMsg) tea.Cmd {
	if msg.seq != m.latAnimSeq {
		return nil
	}
	if !m.latAnimOn {
		return nil
	}
	if !m.latencySeries.empty() {
		m.stopLatencyAnim()
		return nil
	}

	elapsed := time.Since(m.latAnimStart)
	if elapsed >= latAnimDuration {
		m.stopLatencyAnim()
		return nil
	}
	m.latAnimFrame++
	return m.latencyAnimTickCmd()
}

func latencyAnimText(elapsed time.Duration, maxBars int) string {
	if maxBars < 1 {
		maxBars = latPlaceholderBars
	}
	if elapsed >= latAnimDuration {
		return latencyPlaceholder
	}

	p := latAnimProgress(elapsed)
	amp := 1 - p
	hz := latAnimStartHz + (latAnimEndHz-latAnimStartHz)*p
	phase := 2 * math.Pi * hz * elapsed.Seconds()

	out := make([]rune, maxBars)
	for i := 0; i < maxBars; i++ {
		v := math.Sin(phase + float64(i)*1.35)
		if v < 0 {
			v = -v
		}

		v *= amp
		idx := int(v*float64(len(latencyLevels)-1) + 0.5)
		if idx < 0 {
			idx = 0
		}
		if idx >= len(latencyLevels) {
			idx = len(latencyLevels) - 1
		}
		out[i] = latencyLevels[idx]
	}

	bars := latAnimBars(elapsed, maxBars)
	if bars < len(out) {
		out = latAnimTail(out, bars)
	}
	return string(out) + " ms"
}

func latencyAnimDelay(elapsed time.Duration) time.Duration {
	p := latAnimProgress(elapsed)
	if p <= 0 {
		return latAnimMinDelay
	}
	if p >= 1 {
		return latAnimMaxDelay
	}

	ease := p * p
	return latAnimMinDelay + time.Duration(float64(latAnimMaxDelay-latAnimMinDelay)*ease)
}

func latAnimProgress(elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	if elapsed >= latAnimDuration {
		return 1
	}
	return float64(elapsed) / float64(latAnimDuration)
}

func latAnimBars(elapsed time.Duration, maxBars int) int {
	minBars := latPlaceholderBars
	if maxBars < minBars {
		return maxBars
	}

	diff := maxBars - minBars
	if diff <= 0 {
		return minBars
	}

	p := latAnimProgress(elapsed)
	idx := latAnimStepIndex(p, diff)
	bars := maxBars - idx
	if bars < minBars {
		bars = minBars
	}
	return bars
}

func latAnimStepIndex(p float64, steps int) int {
	if p <= 0 || steps <= 0 {
		return 0
	}
	if p >= 1 {
		return steps
	}

	max := latAnimStepMax
	if max <= 0 {
		max = 1
	}
	if max > 1 {
		max = 1
	}

	q := p / max
	if q >= 1 {
		return steps
	}
	if q < 0 {
		q = 0
	}

	q = math.Pow(q, latAnimStepExp)
	idx := int(q * float64(steps))
	if idx < 0 {
		return 0
	}
	if idx > steps {
		return steps
	}
	return idx
}

func latAnimTail(vals []rune, n int) []rune {
	if n <= 0 || len(vals) == 0 {
		return nil
	}
	if n >= len(vals) {
		return vals
	}

	start := len(vals) - n
	if start < 0 {
		start = 0
	}
	return vals[start:]
}
