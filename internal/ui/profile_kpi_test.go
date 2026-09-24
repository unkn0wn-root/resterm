package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
)

func TestProfileUnitDelta(t *testing.T) {
	ms := float64(time.Millisecond)
	tests := []struct {
		unit  profileUnit
		cur   float64
		base  float64
		text  string
		trend profileTrend
	}{
		{unit: unitLatency, cur: 452 * ms, base: 400 * ms, text: "+52ms", trend: trendWorse},
		{unit: unitLatency, cur: 300 * ms, base: 400 * ms, text: "-100ms", trend: trendBetter},
		{unit: unitLatency, cur: 390 * ms, base: 400 * ms, text: "-10ms", trend: trendFlat},
		{unit: unitPercent, cur: 50, base: 0, text: "+50%", trend: trendBetter},
		{unit: unitRate, cur: 2.04, base: 2, text: "=", trend: trendFlat},
	}
	for _, test := range tests {
		if got := test.unit.formatDelta(test.cur - test.base); got != test.text {
			t.Fatalf("unit %d formatDelta(%v - %v) = %q, want %q", test.unit, test.cur, test.base, got, test.text)
		}
		if got := test.unit.trend(test.cur, test.base); got != test.trend {
			t.Fatalf("unit %d trend(%v, %v) = %d, want %d", test.unit, test.cur, test.base, got, test.trend)
		}
	}
}

func TestProfileKPITrendNeedsSamples(t *testing.T) {
	run := func(n int, d time.Duration) core.ProfileSnapshot {
		samples := slices.Repeat([]time.Duration{d}, n)
		return core.ProfileSnapshot{
			Progress: core.ProfileProgress{Count: n, Measured: n, Passed: n},
			Stats:    analysis.ComputeLatencyStats(samples, analysis.DefaultProfilePercentiles(), 10),
		}
	}
	tests := []struct {
		cur, base int
		want      map[string]profileTrend
	}{
		{10, 10, map[string]profileTrend{"P50": trendWorse, "P90": trendWorse, "P95": trendFlat, "P99": trendFlat}},
		{20, 100, map[string]profileTrend{"P95": trendWorse, "P99": trendFlat}},
		{100, 10, map[string]profileTrend{"P95": trendFlat, "P99": trendFlat}},
		{100, 100, map[string]profileTrend{"P95": trendWorse, "P99": trendWorse}},
	}
	for _, test := range tests {
		base := run(test.base, 400*time.Millisecond)
		v := &profileStatsView{snap: run(test.cur, 500*time.Millisecond), base: &base}
		for _, c := range v.kpiCells() {
			want, ok := test.want[c.label]
			if !ok {
				continue
			}
			if c.delta != "+100ms" || c.trend != want {
				t.Fatalf("%d vs %d runs: %s = %q trend %d, want \"+100ms\" trend %d",
					test.cur, test.base, c.label, c.delta, c.trend, want)
			}
		}
	}
}
