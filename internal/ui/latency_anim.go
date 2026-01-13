package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	latAnimDuration      = 2400 * time.Millisecond
	latAnimFadeDuration  = 600 * time.Millisecond
	latAnimPlaceDuration = 450 * time.Millisecond
	latAnimFadeEnd       = latAnimDuration + latAnimFadeDuration
	latAnimTotalDuration = latAnimFadeEnd + latAnimPlaceDuration
	latAnimTickFast      = 70 * time.Millisecond
	latAnimTickSlow      = 110 * time.Millisecond
	latAnimStartHz       = 1.4
	latAnimEndHz         = 0.95
	latAnimBaseHi        = 0.6
	latAnimBaseLo        = 0.0
	latAnimHiDur         = 3200 * time.Millisecond
	latAnimMidDur        = 1800 * time.Millisecond
	latAnimLoDur         = 380 * time.Millisecond
	latAnimMinDur        = 120 * time.Millisecond
	latAnimJitHi         = 0.45
	latAnimJitMid        = 0.22
	latAnimJitLo         = 0.1
	latAnimJitMin        = 0.04
)

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
	return latencyPlaceholder
}

func (m Model) latencyAnimTickCmd() tea.Cmd {
	s := m.latencySeries
	if s == nil || !m.latAnimOn || !s.empty() {
		return nil
	}

	el := time.Since(m.latAnimStart)
	seq := m.latAnimSeq
	if el >= latAnimTotalDuration {
		return func() tea.Msg {
			return latencyAnimMsg{seq: seq}
		}
	}

	d := latAnimTick(el)
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

	el := time.Since(m.latAnimStart)
	if el >= latAnimTotalDuration {
		m.stopLatencyAnim()
		return nil
	}
	return m.latencyAnimTickCmd()
}

func latencyAnimText(el time.Duration, max int) string {
	if max < 1 {
		max = latPlaceholderBars
	}
	if el >= latAnimTotalDuration {
		if max > latPlaceholderBars {
			max = latPlaceholderBars
		}
		return latPlaceholder(max)
	}
	if el >= latAnimFadeEnd {
		return latAnimPlaceText(el, max)
	}
	p := latAnimProgress(el)
	amp := latAnimAmp(el)
	hz := latAnimHz(p)
	base := latAnimBase(p)
	out := latAnimWave(el, max, amp, hz, base)
	val := latAnimVal(el, hz)
	return string(out) + " " + val
}

func latAnimWave(el time.Duration, max int, amp, hz, base float64) []rune {
	ph := latAnimPhase(el, hz)
	out := make([]rune, max)
	for i := 0; i < max; i++ {
		v := math.Sin(ph + float64(i)*1.35)
		if v < 0 {
			v = -v
		}
		v = base + (1-base)*v
		v *= amp
		out[i] = latencyLevels[latAnimLevelIdx(v)]
	}
	return out
}

func latAnimLevelIdx(v float64) int {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	idx := int(v*float64(len(latencyLevels)-1) + 0.5)
	if idx < 0 {
		return 0
	}
	if idx >= len(latencyLevels) {
		return len(latencyLevels) - 1
	}
	return idx
}

func latAnimAmp(el time.Duration) float64 {
	return latAnimLerp(1, 0, latAnimOutP(el))
}

func latAnimVal(el time.Duration, hz float64) string {
	d := latAnimDur(el, hz)
	return latAnimFmt(d)
}

func latAnimDur(el time.Duration, hz float64) time.Duration {
	b := latAnimBaseDur(el)
	j := latAnimJit(el, hz)
	v := time.Duration(float64(b) * j)
	if v < latAnimMinDur {
		return latAnimMinDur
	}
	return v
}

func latAnimBaseDur(el time.Duration) time.Duration {
	p := latAnimProgress(el)
	wn, ok := latAnimThresholds()
	var d time.Duration
	if p <= wn {
		d = latAnimLerpDur(latAnimHiDur, latAnimMidDur, latAnimNorm(p, wn))
	} else if p <= ok {
		d = latAnimLerpDur(latAnimMidDur, latAnimLoDur, latAnimNorm(p-wn, ok-wn))
	} else {
		d = latAnimLoDur
	}
	f := latAnimFadeP(el)
	if f > 0 {
		d = latAnimLerpDur(d, latAnimMinDur, f)
	}
	return d
}

func latAnimOutP(el time.Duration) float64 {
	if el <= 0 {
		return 0
	}
	if el >= latAnimFadeEnd {
		return 1
	}
	if latAnimFadeEnd <= 0 {
		return 1
	}
	return float64(el) / float64(latAnimFadeEnd)
}

func latAnimTick(el time.Duration) time.Duration {
	p := latAnimProgress(el)
	wn, ok := latAnimThresholds()
	if p <= wn {
		return latAnimTickFast
	}
	if p >= ok {
		return latAnimTickSlow
	}
	return latAnimLerpDur(latAnimTickFast, latAnimTickSlow, latAnimNorm(p-wn, ok-wn))
}

func latAnimProgress(el time.Duration) float64 {
	if el <= 0 {
		return 0
	}
	if el >= latAnimDuration {
		return 1
	}
	return float64(el) / float64(latAnimDuration)
}

func latAnimThresholds() (float64, float64) {
	wn := latAnimWarnP
	ok := latAnimOkP
	if ok < wn {
		ok = wn
	}
	return wn, ok
}

func latAnimFadeP(el time.Duration) float64 {
	if el <= latAnimDuration {
		return 0
	}
	if el >= latAnimFadeEnd {
		return 1
	}
	if latAnimFadeDuration <= 0 {
		return 1
	}
	return float64(el-latAnimDuration) / float64(latAnimFadeDuration)
}

func latAnimPlaceP(el time.Duration) float64 {
	if el <= latAnimFadeEnd {
		return 0
	}
	if el >= latAnimTotalDuration {
		return 1
	}
	if latAnimPlaceDuration <= 0 {
		return 1
	}
	return float64(el-latAnimFadeEnd) / float64(latAnimPlaceDuration)
}

func latAnimPlaceText(el time.Duration, max int) string {
	p := latAnimPlaceP(el)
	n := latAnimPlaceBars(max, p)
	val := latAnimPlaceVal(el, p)
	return latFill(n) + " " + val
}

func latAnimPlaceBars(max int, p float64) int {
	if max < 1 {
		return 0
	}
	min := latPlaceholderBars
	if max < min {
		min = max
	}
	if p <= 0 {
		return max
	}
	if p >= 1 {
		return min
	}
	n := latAnimLerp(float64(max), float64(min), p)
	return int(math.Round(n))
}

func latAnimPlaceVal(el time.Duration, p float64) string {
	val := latAnimVal(el, latAnimEndHz)
	return latAnimTrim(val, p)
}

func latAnimTrim(val string, p float64) string {
	if p <= 0 {
		return val
	}
	if p >= 1 {
		return "ms"
	}
	if !strings.HasSuffix(val, "ms") {
		return val
	}
	num := strings.TrimSuffix(val, "ms")
	if num == "" {
		return "ms"
	}
	n := len(num)
	keep := int(math.Round(float64(n) * (1 - p)))
	if keep <= 0 {
		return "ms"
	}
	if keep >= n {
		return val
	}
	return num[:keep] + "ms"
}

func latAnimHz(p float64) float64 {
	return latAnimLerp(latAnimStartHz, latAnimEndHz, p)
}

func latAnimBase(p float64) float64 {
	return latAnimLerp(latAnimBaseHi, latAnimBaseLo, p)
}

func latAnimLerp(a, b, p float64) float64 {
	if p <= 0 {
		return a
	}
	if p >= 1 {
		return b
	}
	return a + (b-a)*p
}

func latAnimLerpDur(a, b time.Duration, p float64) time.Duration {
	if p <= 0 {
		return a
	}
	if p >= 1 {
		return b
	}
	return time.Duration(float64(a) + (float64(b)-float64(a))*p)
}

func latAnimNorm(p, d float64) float64 {
	if d <= 0 {
		return 1
	}
	if p <= 0 {
		return 0
	}
	return p / d
}

func latAnimJit(el time.Duration, hz float64) float64 {
	a := latAnimJitAmp(el)
	ph := latAnimPhase(el, hz)
	j := 0.6*math.Sin(ph) + 0.4*math.Sin(ph*1.7+1.1)
	return 1 + a*j
}

func latAnimJitAmp(el time.Duration) float64 {
	p := latAnimProgress(el)
	wn, ok := latAnimThresholds()
	var a float64
	if p <= wn {
		a = latAnimJitHi
	} else if p <= ok {
		a = latAnimLerp(latAnimJitHi, latAnimJitMid, latAnimNorm(p-wn, ok-wn))
	} else {
		a = latAnimJitLo
	}
	f := latAnimFadeP(el)
	if f > 0 {
		a = latAnimLerp(a, latAnimJitMin, f)
	}
	return a
}

func latAnimPhase(el time.Duration, hz float64) float64 {
	return 2 * math.Pi * hz * el.Seconds()
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
