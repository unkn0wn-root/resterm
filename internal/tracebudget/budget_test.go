package tracebudget

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestFromSpecNormalizesPhases(t *testing.T) {
	spec := &restfile.TraceSpec{Enabled: true}
	spec.Budgets.Total = 100 * time.Millisecond
	spec.Budgets.Phases = map[string]time.Duration{
		"DNS":      10 * time.Millisecond,
		"connect":  15 * time.Millisecond,
		"transfer": 50 * time.Millisecond,
	}

	budget, ok := FromSpec(spec)
	if !ok {
		t.Fatalf("expected budget to be detected")
	}
	if budget.Total != spec.Budgets.Total {
		t.Fatalf("expected total %v, got %v", spec.Budgets.Total, budget.Total)
	}
	if budget.Phases["dns"] != 10*time.Millisecond {
		t.Fatalf("expected dns budget to be normalized")
	}
	if len(budget.Phases) != 3 {
		t.Fatalf("expected 3 phase budgets, got %d", len(budget.Phases))
	}
}

func TestFromSpecDisabled(t *testing.T) {
	spec := &restfile.TraceSpec{Enabled: false}
	if _, ok := FromSpec(spec); ok {
		t.Fatalf("expected disabled spec to skip budget")
	}
}

func TestFromTraceClampsNegative(t *testing.T) {
	b := restfile.TraceBudget{
		Total:     -5 * time.Millisecond,
		Tolerance: -10 * time.Millisecond,
		Phases: map[string]time.Duration{
			"dns":      -5 * time.Millisecond,
			"transfer": 30 * time.Millisecond,
		},
	}

	budget := FromTrace(b)
	if budget.Total != 0 {
		t.Fatalf("expected total to clamp to 0, got %v", budget.Total)
	}
	if budget.Tolerance != 0 {
		t.Fatalf("expected tolerance to clamp to 0, got %v", budget.Tolerance)
	}
	if _, ok := budget.Phases[nettrace.PhaseDNS]; ok {
		t.Fatalf("expected negative phase budget to be dropped")
	}
	if budget.Phases == nil {
		t.Fatalf("expected phase map to be initialised")
	}
	if budget.Phases[nettrace.PhaseTransfer] != 30*time.Millisecond {
		t.Fatalf("expected transfer phase to remain, got %v", budget.Phases[nettrace.PhaseTransfer])
	}
}

func TestNormalizePhase(t *testing.T) {
	cs := map[string]nettrace.PhaseKind{
		" DNS ":           nettrace.PhaseDNS,
		"header":          nettrace.PhaseReqHdrs,
		"request-headers": nettrace.PhaseReqHdrs,
		"request-body":    nettrace.PhaseReqBody,
		"req_body":        nettrace.PhaseReqBody,
		"first_byte":      nettrace.PhaseTTFB,
		"overall":         nettrace.PhaseTotal,
	}
	for in, want := range cs {
		if got, ok := NormalizePhase(in); !ok || got != want {
			t.Fatalf("NormalizePhase(%q) = %q, %v, want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"custom", "request-hdrs", ""} {
		if got, ok := NormalizePhase(in); ok {
			t.Fatalf("NormalizePhase(%q) = %q, want no phase", in, got)
		}
	}
}

func TestNormalizePhaseKeepsCanonicalNames(t *testing.T) {
	for _, k := range []nettrace.PhaseKind{
		nettrace.PhaseDNS,
		nettrace.PhaseConnect,
		nettrace.PhaseTLS,
		nettrace.PhaseReqHdrs,
		nettrace.PhaseReqBody,
		nettrace.PhaseTTFB,
		nettrace.PhaseTransfer,
		nettrace.PhaseTotal,
	} {
		if got, ok := NormalizePhase(string(k)); !ok || got != k {
			t.Fatalf("NormalizePhase(%q) = %q, %v", k, got, ok)
		}
	}
}

func TestFromTraceDropsUnknownPhases(t *testing.T) {
	b := FromTrace(restfile.TraceBudget{Phases: map[string]time.Duration{
		"request-headers": 20 * time.Millisecond,
		"custom":          30 * time.Millisecond,
	}})
	if len(b.Phases) != 1 || b.Phases[nettrace.PhaseReqHdrs] != 20*time.Millisecond {
		t.Fatalf("phases = %v, want only request_headers", b.Phases)
	}
}
