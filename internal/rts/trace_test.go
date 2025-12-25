package rts

import (
	"context"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
)

func evalTrace(t *testing.T, rt RT, src string) Value {
	t.Helper()
	e := NewEng()
	v, err := e.Eval(context.Background(), rt, src, Pos{Path: "test", Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return v
}

func TestTraceDisabled(t *testing.T) {
	rt := RT{}
	v := evalTrace(t, rt, "trace.enabled()")
	if v.K != VBool || v.B != false {
		t.Fatalf("expected disabled trace, got %+v", v)
	}
	v = evalTrace(t, rt, "len(trace.breaches())")
	if v.K != VNum || v.N != 0 {
		t.Fatalf("expected no breaches, got %+v", v)
	}
	v = evalTrace(t, rt, "trace.withinBudget()")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected within budget, got %+v", v)
	}
	v = evalTrace(t, rt, "len(trace.phases())")
	if v.K != VNum || v.N != 0 {
		t.Fatalf("expected no phases, got %+v", v)
	}
}

func TestTraceEnabled(t *testing.T) {
	t0 := time.Unix(0, 0)
	tl := &nettrace.Timeline{
		Started:   t0,
		Completed: t0.Add(75 * time.Millisecond),
		Duration:  75 * time.Millisecond,
		Phases: []nettrace.Phase{
			{
				Kind:     nettrace.PhaseDNS,
				Start:    t0,
				End:      t0.Add(5 * time.Millisecond),
				Duration: 5 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "example.com", Cached: true},
			},
			{
				Kind:     nettrace.PhaseConnect,
				Start:    t0.Add(5 * time.Millisecond),
				End:      t0.Add(30 * time.Millisecond),
				Duration: 25 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "93.184.216.34:443"},
			},
			{
				Kind:     nettrace.PhaseTLS,
				Start:    t0.Add(30 * time.Millisecond),
				End:      t0.Add(45 * time.Millisecond),
				Duration: 15 * time.Millisecond,
			},
			{
				Kind:     nettrace.PhaseReqHdrs,
				Start:    t0.Add(45 * time.Millisecond),
				End:      t0.Add(46 * time.Millisecond),
				Duration: 1 * time.Millisecond,
			},
			{
				Kind:     nettrace.PhaseReqBody,
				Start:    t0.Add(46 * time.Millisecond),
				End:      t0.Add(48 * time.Millisecond),
				Duration: 2 * time.Millisecond,
			},
			{
				Kind:     nettrace.PhaseTTFB,
				Start:    t0.Add(48 * time.Millisecond),
				End:      t0.Add(55 * time.Millisecond),
				Duration: 7 * time.Millisecond,
			},
			{
				Kind:     nettrace.PhaseTransfer,
				Start:    t0.Add(55 * time.Millisecond),
				End:      t0.Add(75 * time.Millisecond),
				Duration: 20 * time.Millisecond,
			},
		},
	}
	bud := nettrace.Budget{
		Total:     60 * time.Millisecond,
		Tolerance: 5 * time.Millisecond,
		Phases: map[nettrace.PhaseKind]time.Duration{
			nettrace.PhaseDNS:     5 * time.Millisecond,
			nettrace.PhaseConnect: 15 * time.Millisecond,
		},
	}
	rep := nettrace.NewReport(tl, bud)
	rt := RT{Trace: &Trace{Rep: rep}}

	v := evalTrace(t, rt, "trace.enabled()")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected enabled trace, got %+v", v)
	}
	v = evalTrace(t, rt, "len(trace.breaches())")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected 2 breaches, got %+v", v)
	}
	v = evalTrace(t, rt, "trace.withinBudget()")
	if v.K != VBool || v.B != false {
		t.Fatalf("expected budget failure, got %+v", v)
	}
	v = evalTrace(t, rt, "trace.getPhase(\"dns\").count")
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected dns count 1, got %+v", v)
	}
	v = evalTrace(t, rt, "trace.budgets().enabled")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected budgets enabled, got %+v", v)
	}
	v = evalTrace(t, rt, "trace.budgets().phases.connect")
	if v.K != VNum || v.N != 15 {
		t.Fatalf("expected connect budget 15ms, got %+v", v)
	}
	v = evalTrace(t, rt, "len(trace.phaseNames())")
	if v.K != VNum || v.N != 7 {
		t.Fatalf("expected 7 phase names, got %+v", v)
	}
}
