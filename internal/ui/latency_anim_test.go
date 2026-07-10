package ui

import (
	"testing"
	"time"
)

func TestLatClimbBuildsRamp(t *testing.T) {
	if got := len(latRamp); got != latMinBars {
		t.Fatalf("expected startup ramp width %d, got %d", latMinBars, got)
	}
	if got := latClimb(0); got != latFill(latMinBars) {
		t.Fatalf("expected flat start, got %q", got)
	}
	if got := latClimb(0.5); got != "▁▂▄▁▁" {
		t.Fatalf("expected half-built ramp, got %q", got)
	}
	if got := latClimb(1); got != string(latRamp) {
		t.Fatalf("expected placeholder ramp, got %q", got)
	}
}

func TestLatencyTextDuringAnim(t *testing.T) {
	m := Model{latencySeries: newLatencySeries(latCap)}
	m.startLatAnim()

	if got := m.latencyText(); got != latFill(latMinBars)+" ms" {
		t.Fatalf("expected flat climb frame, got %q", got)
	}

	m.latencySeries.add(120 * time.Millisecond)
	s := requireLatencySummary(t, m.latencySeries)
	if got := m.latencyText(); got != formatLatencySummary(s) {
		t.Fatalf("expected series render to win over anim, got %q", got)
	}

	m.latencySeries.vals = nil
	m.latAnimOn = false
	if got := m.latencyText(); got != latPlaceholder {
		t.Fatalf("expected placeholder after anim, got %q", got)
	}
}

func TestHandleLatAnim(t *testing.T) {
	m := &Model{latencySeries: newLatencySeries(latCap)}
	m.startLatAnim()

	if cmd := m.handleLatAnim(); cmd == nil || !m.latAnimOn {
		t.Fatalf("expected running anim to reschedule")
	}

	m.latAnimT0 = time.Now().Add(-latAnimDur)
	if cmd := m.handleLatAnim(); cmd != nil || m.latAnimOn {
		t.Fatalf("expected animation to stop after its duration")
	}

	m.startLatAnim()
	m.latencySeries.add(time.Millisecond)
	if cmd := m.handleLatAnim(); cmd != nil || m.latAnimOn {
		t.Fatalf("expected anim to stop once series has data")
	}
}
