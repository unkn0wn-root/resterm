package ui

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	latAnimHold = 280 * time.Millisecond
	latAnimCol  = 420 * time.Millisecond
	latAnimTick = 45 * time.Millisecond
	latAnimHi   = 700 * time.Millisecond
	latAnimLo   = 280 * time.Millisecond
	latAnimJit  = 1.0
)

var latAnimSteps = []time.Duration{
	0,
	120 * time.Millisecond,
	220 * time.Millisecond,
	200 * time.Millisecond,
	200 * time.Millisecond,
	200 * time.Millisecond,
	200 * time.Millisecond,
	200 * time.Millisecond,
}

var latAnimVals = latAnimRand(len(latAnimSteps), latAnimHi, latAnimLo)

func (m *Model) initLatencyAnim() {
	m.latAnimOn = true
	m.latAnimSeq++
	m.latAnimStart = time.Now()
}

func (m *Model) stopLatencyAnim() {
	m.latAnimOn = false
}

func (m *Model) addLatency(d time.Duration) {
	if m.latencySeries == nil || d <= 0 {
		return
	}
	m.latencySeries.add(d)
	m.stopLatencyAnim()
}

func (m Model) latencyText() string {
	s := m.latencySeries
	if s == nil {
		return ""
	}
	if !s.empty() {
		return s.render()
	}
	if m.latAnimOn {
		return latencyAnimText(time.Since(m.latAnimStart), s.cap)
	}
	return ""
}

func (m Model) latencyAnimTickCmd() tea.Cmd {
	s := m.latencySeries
	if s == nil || !m.latAnimOn || !s.empty() {
		return nil
	}

	el := time.Since(m.latAnimStart)
	seq := m.latAnimSeq
	if el >= latAnimTotal() {
		return func() tea.Msg {
			return latencyAnimMsg{seq: seq}
		}
	}

	return tea.Tick(latAnimTick, func(time.Time) tea.Msg {
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

	el := time.Since(m.latAnimStart)
	if el >= latAnimTotal() {
		m.stopLatencyAnim()
		return nil
	}
	return m.latencyAnimTickCmd()
}

func latencyAnimText(el time.Duration, max int) string {
	if max < 1 {
		max = latPlaceholderBars
	}
	if el >= latAnimTotal() {
		return ""
	}

	vals := latAnimSeq(el)
	if len(vals) == 0 {
		return ""
	}

	w := min(max, len(vals))
	bars := latAnimBars(vals, w)
	last := vals[len(vals)-1]
	lab := latAnimFmt(last)

	p := 0.0
	start := latAnimColStart()
	if el > start {
		if latAnimCol <= 0 || el >= start+latAnimCol {
			p = 1
		} else {
			p = float64(el-start) / float64(latAnimCol)
		}
	}

	if p > 0 {
		bars = latAnimColBars(bars, p)
		if bars == "" {
			return ""
		}
	}
	return bars + " " + lab
}

func latAnimSeq(el time.Duration) []time.Duration {
	n := min(len(latAnimVals), len(latAnimSteps))
	if n <= 0 {
		return nil
	}

	count := 1
	if el > 0 {
		var t time.Duration
		for i := 1; i < n; i++ {
			t += latAnimSteps[i]
			if el < t {
				break
			}
			count = i + 1
		}
	}
	return latAnimVals[:count]
}

func latAnimBars(vals []time.Duration, max int) string {
	if len(vals) == 0 || max <= 0 {
		return ""
	}
	if max < len(vals) {
		vals = vals[len(vals)-max:]
	}

	hi := vals[0]
	for _, v := range vals[1:] {
		if v > hi {
			hi = v
		}
	}
	if hi <= 0 {
		hi = time.Millisecond
	}

	bars := sparkline(vals, 0, hi)
	if pad := max - len(vals); pad > 0 {
		bars = latFill(pad) + bars
	}
	return bars
}

func latAnimColBars(bars string, p float64) string {
	if p <= 0 {
		return bars
	}
	if p >= 1 {
		return ""
	}

	rs := []rune(bars)
	if len(rs) == 0 {
		return ""
	}
	p = 0.5 - 0.5*math.Cos(math.Pi*p)
	if p <= 0 {
		return bars
	}

	scale := 1 - p
	for i, r := range rs {
		idx := -1
		for j, lvl := range latencyLevels {
			if r == lvl {
				idx = j
				break
			}
		}
		if idx < 0 {
			continue
		}

		n := int(math.Round(float64(idx) * scale))
		if n < 0 {
			n = 0
		}
		if n >= len(latencyLevels) {
			n = len(latencyLevels) - 1
		}
		rs[i] = latencyLevels[n]
	}
	return string(rs)
}

func latAnimProgress(el time.Duration) float64 {
	if el <= 0 {
		return 0
	}

	d := latAnimBurst()
	if d <= 0 {
		return 1
	}
	if el >= d {
		return 1
	}
	return float64(el) / float64(d)
}

func latAnimThresholds() (float64, float64) {
	wn := latAnimWarnP
	ok := latAnimOkP
	if ok < wn {
		ok = wn
	}
	return wn, ok
}

func latAnimBurst() time.Duration {
	n := min(len(latAnimVals), len(latAnimSteps))
	if n < 2 {
		return 0
	}

	var total time.Duration
	for i := 1; i < n; i++ {
		total += latAnimSteps[i]
	}
	return total
}

func latAnimRand(n int, hi, lo time.Duration) []time.Duration {
	if n <= 0 {
		return nil
	}
	if hi < lo {
		hi, lo = lo, hi
	}

	vals := make([]time.Duration, n)
	if n == 1 {
		vals[0] = hi
		return vals
	}

	span := hi - lo
	if span <= 0 {
		for i := range vals {
			vals[i] = hi
		}
		return vals
	}

	vals[0] = hi
	vals[n-1] = lo
	st := float64(span) / float64(n-1)
	jit := st * latAnimJit
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	hf := float64(hi)
	lf := float64(lo)
	for i := 1; i < n-1; i++ {
		base := hf - st*float64(i)
		off := (r.Float64()*2 - 1) * jit
		v := base + off
		if v < lf {
			v = lf
		}
		if v > hf {
			v = hf
		}
		vals[i] = time.Duration(v)
	}
	return vals
}

func latAnimFmt(d time.Duration) string {
	if d < time.Millisecond {
		d = time.Millisecond
	}
	if d < time.Second {
		ms := d.Round(time.Millisecond) / time.Millisecond
		return fmt.Sprintf("%dms", ms)
	}
	s := float64(d) / float64(time.Second)
	return fmt.Sprintf("%.2fs", s)
}

func latAnimColStart() time.Duration {
	return latAnimBurst() + latAnimHold
}

func latAnimTotal() time.Duration {
	return latAnimColStart() + latAnimCol
}
