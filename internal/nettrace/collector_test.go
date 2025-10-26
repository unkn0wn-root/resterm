package nettrace

import (
	"testing"
	"time"
)

func TestCollectorRecordsPhases(t *testing.T) {
	t0 := time.Unix(0, 0)
	c := NewCollector()

	c.Begin(PhaseDNS, t0)
	c.End(PhaseDNS, t0.Add(5*time.Millisecond), nil)

	c.Begin(PhaseConnect, t0.Add(5*time.Millisecond))
	c.UpdateMeta(PhaseConnect, func(meta *PhaseMeta) {
		meta.Addr = "93.184.216.34:443"
	})
	c.End(PhaseConnect, t0.Add(15*time.Millisecond), nil)

	c.Begin(PhaseTLS, t0.Add(15*time.Millisecond))
	c.UpdateMeta(PhaseTLS, func(meta *PhaseMeta) {
		meta.Cached = true
	})
	c.End(PhaseTLS, t0.Add(25*time.Millisecond), nil)

	c.Begin(PhaseReqHdrs, t0.Add(25*time.Millisecond))
	c.End(PhaseReqHdrs, t0.Add(30*time.Millisecond), nil)

	c.Begin(PhaseTTFB, t0.Add(30*time.Millisecond))
	c.End(PhaseTTFB, t0.Add(40*time.Millisecond), nil)

	c.Begin(PhaseTransfer, t0.Add(40*time.Millisecond))
	c.End(PhaseTransfer, t0.Add(60*time.Millisecond), nil)

	c.Complete(t0.Add(60 * time.Millisecond))
	tl := c.Timeline()
	if tl == nil {
		t.Fatalf("expected timeline")
	}
	if len(tl.Phases) != 6 {
		t.Fatalf("expected 6 phases, got %d", len(tl.Phases))
	}
	if want, got := 60*time.Millisecond, tl.Duration; got != want {
		t.Fatalf("expected total duration %v, got %v", want, got)
	}

	phases := aggregateDurations(tl)
	if got := phases[PhaseConnect]; got != 10*time.Millisecond {
		t.Fatalf("connect duration mismatch: %v", got)
	}
	if tl.Phases[1].Meta.Addr != "93.184.216.34:443" {
		t.Fatalf("connect meta address not set: %+v", tl.Phases[1].Meta)
	}
	if !tl.Phases[2].Meta.Cached {
		t.Fatalf("tls meta cached flag not set")
	}
}

func TestCollectorCompletesDanglingPhases(t *testing.T) {
	t0 := time.Now()
	c := NewCollector()
	c.Begin(PhaseConnect, t0)
	c.Complete(t0.Add(10 * time.Millisecond))

	tl := c.Timeline()
	if tl == nil {
		t.Fatalf("expected timeline")
	}
	if len(tl.Phases) != 1 {
		t.Fatalf("expected one phase, got %d", len(tl.Phases))
	}
	if tl.Phases[0].Err != "incomplete" {
		t.Fatalf("expected incomplete marker, got %q", tl.Phases[0].Err)
	}
}
