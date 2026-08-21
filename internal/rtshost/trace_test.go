package rtshost

import (
	"context"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/stdlib"
)

func TestTraceDisabled(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	rt.Trace = nil
	tests := map[string]bool{
		`trace.enabled()`:      false,
		`trace.withinBudget()`: true,
	}
	for src, want := range tests {
		v := evalHost(t, eng, rt, src)
		if v.K != rts.VBool || v.B != want {
			t.Errorf("%s = %+v, want %t", src, v, want)
		}
	}
	for _, src := range []string{`len(trace.breaches())`, `len(trace.phases())`} {
		v := evalHost(t, eng, rt, src)
		if v.K != rts.VNum || v.N != 0 {
			t.Errorf("%s = %+v, want 0", src, v)
		}
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
				Duration: time.Millisecond,
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
		Details: &nettrace.TraceDetails{
			Connection: &nettrace.ConnDetails{
				Reused: true, IdleTime: 5 * time.Millisecond,
				ResolvedAddrs: []string{"93.184.216.34"},
				K8s:           "default/api:8080", Protocol: "HTTP/2.0",
			},
			TLS: &nettrace.TLSDetails{
				Version: "TLS 1.3", Cipher: "TLS_AES_128_GCM_SHA256", Verified: true,
				Certificates: []nettrace.TLSCert{
					{Subject: "example.com", Issuer: "Example CA", NotAfter: t0.Add(24 * time.Hour), Serial: "01"},
				},
			},
		},
	}
	bud := nettrace.Budget{
		Total: 60 * time.Millisecond, Tolerance: 5 * time.Millisecond,
		Phases: map[nettrace.PhaseKind]time.Duration{
			nettrace.PhaseDNS: 5 * time.Millisecond, nettrace.PhaseConnect: 15 * time.Millisecond,
		},
	}
	rt := testRuntime(t)
	rt.Trace = &Trace{Report: nettrace.NewReport(tl, bud)}
	eng := NewEngine(stdlib.New)
	tests := map[string]string{
		`str(trace.enabled())`:                "true",
		`str(len(trace.breaches()))`:          "2",
		`str(trace.withinBudget())`:           "false",
		`str(trace.getPhase("dns").count)`:    "1",
		`str(trace.budgets().enabled)`:        "true",
		`str(trace.budgets().phases.connect)`: "15",
		`str(len(trace.phaseNames()))`:        "7",
		`str(trace.connection().available)`:   "true",
		`trace.connection().protocol`:         "HTTP/2.0",
		`trace.connection().k8s`:              "default/api:8080",
		`trace.tls().version`:                 "TLS 1.3",
		`str(len(trace.tls().certs))`:         "1",
	}
	for src, want := range tests {
		v := evalHost(t, eng, rt, src)
		if v.K != rts.VStr || v.S != want {
			t.Errorf("%s = %+v, want %q", src, v, want)
		}
	}
}

func TestGetPhaseIgnoresBlankNames(t *testing.T) {
	rep := &nettrace.Report{Timeline: &nettrace.Timeline{Phases: []nettrace.Phase{
		{Kind: "", Duration: time.Millisecond},
		{Kind: "dns", Duration: 2 * time.Millisecond},
	}}}
	rt := testRuntime(t)
	rt.Trace = &Trace{Report: rep}
	eng := NewEngine(stdlib.New)
	for _, src := range []string{`trace.getPhase("")`, `trace.getPhase("   ")`} {
		v, err := eng.Eval(context.Background(), rt, src, testPos)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if v.K != rts.VNull {
			t.Errorf("%s = %+v, want null", src, v)
		}
	}
	v := evalHost(t, eng, rt, `trace.getPhase("DNS").name`)
	if v.K != rts.VStr || v.S != "dns" {
		t.Fatalf("trace.getPhase(DNS).name = %+v, want dns", v)
	}
}
