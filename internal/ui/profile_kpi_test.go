package ui

import (
	"testing"
	"time"
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
