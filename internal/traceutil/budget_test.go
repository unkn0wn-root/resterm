package traceutil

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestBudgetFromSpecNormalizesPhases(t *testing.T) {
	spec := &restfile.TraceSpec{Enabled: true}
	spec.Budgets.Total = 100 * time.Millisecond
	spec.Budgets.Phases = map[string]time.Duration{
		"DNS":      10 * time.Millisecond,
		"connect":  15 * time.Millisecond,
		"transfer": 50 * time.Millisecond,
	}

	budget, ok := BudgetFromSpec(spec)
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

func TestBudgetFromSpecDisabled(t *testing.T) {
	spec := &restfile.TraceSpec{Enabled: false}
	if _, ok := BudgetFromSpec(spec); ok {
		t.Fatalf("expected disabled spec to skip budget")
	}
}

func TestBudgetFromTraceBudgetClampsNegative(t *testing.T) {
	b := restfile.TraceBudget{
		Total:     -5 * time.Millisecond,
		Tolerance: -10 * time.Millisecond,
		Phases: map[string]time.Duration{
			"dns":      -5 * time.Millisecond,
			"transfer": 30 * time.Millisecond,
		},
	}

	budget := BudgetFromTraceBudget(b)
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
