package ui

import (
	"math"
	"strconv"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
)

// Changes within this share of the baseline value are shown without color.
const profileNoise = 0.05

type profileUnit uint8

const (
	unitPercent profileUnit = iota
	unitLatency
	unitRate
)

type profileTrend uint8

const (
	trendFlat profileTrend = iota
	trendBetter
	trendWorse
)

type profileKPI struct {
	label string
	unit  profileUnit
	read  func(core.ProfileSnapshot) (float64, bool)
}

var profileKPIs = []profileKPI{
	{"SUCCESS", unitPercent, readSuccess},
	{"P50", unitLatency, readPercentile(50)},
	{"P90", unitLatency, readPercentile(90)},
	{"P95", unitLatency, readPercentile(95)},
	{"P99", unitLatency, readPercentile(99)},
	{"RATE", unitRate, readRate},
}

type kpiCell struct {
	label string
	value string
	delta string
	trend profileTrend
}

func (v *profileStatsView) kpiCells() []kpiCell {
	cells := make([]kpiCell, len(profileKPIs))
	for i, k := range profileKPIs {
		c := kpiCell{label: k.label, value: "-"}
		cur, ok := k.read(v.snap)
		if ok {
			c.value = k.unit.format(cur)
		}
		if v.base != nil {
			if base, found := k.read(*v.base); ok && found {
				c.delta, c.trend = k.unit.formatDelta(cur-base), k.unit.trend(cur, base)
			}
		}
		cells[i] = c
	}
	return cells
}

func readSuccess(s core.ProfileSnapshot) (float64, bool) {
	p := s.Progress
	if p.Measured == 0 {
		return 0, false
	}
	return float64(p.Passed * 100 / p.Measured), true
}

func readPercentile(p int) func(core.ProfileSnapshot) (float64, bool) {
	return func(s core.ProfileSnapshot) (float64, bool) {
		d, ok := s.Stats.Percentiles[p]
		return float64(d), ok
	}
}

func readRate(s core.ProfileSnapshot) (float64, bool) {
	r := s.WallRate()
	return r, r > 0
}

func (u profileUnit) format(x float64) string {
	if u == unitRate {
		return u.amount(x) + " wall"
	}
	return u.amount(x)
}

func (u profileUnit) formatDelta(d float64) string {
	mag := u.amount(math.Abs(d))
	switch {
	case mag == u.amount(0):
		return "="
	case d < 0:
		return "-" + mag
	default:
		return "+" + mag
	}
}

func (u profileUnit) amount(x float64) string {
	switch u {
	case unitPercent:
		return strconv.Itoa(int(x)) + "%"
	case unitLatency:
		return formatDurationShort(time.Duration(x))
	default:
		return strconv.FormatFloat(x, 'f', 1, 64) + "/s"
	}
}

func (u profileUnit) trend(cur, base float64) profileTrend {
	d := cur - base
	switch {
	case math.Abs(d) <= base*profileNoise:
		return trendFlat
	case (d < 0) == (u == unitLatency):
		return trendBetter
	default:
		return trendWorse
	}
}

func (t profileTrend) style(pal statsPalette) lipgloss.Style {
	switch t {
	case trendBetter:
		return pal.Success
	case trendWorse:
		return pal.Warn
	default:
		return pal.SubLabel
	}
}
