package nettrace

import (
	"testing"
	"time"
)

func TestEvaluateBudgetDetectsBreaches(t *testing.T) {
	tl := &Timeline{
		Started:   time.Unix(0, 0),
		Completed: time.Unix(0, int64(120*time.Millisecond)),
		Duration:  120 * time.Millisecond,
		Phases: []Phase{
			{Kind: PhaseDNS, Duration: 20 * time.Millisecond},
			{Kind: PhaseConnect, Duration: 60 * time.Millisecond},
			{Kind: PhaseTransfer, Duration: 30 * time.Millisecond},
		},
	}

	budget := Budget{
		Total:     100 * time.Millisecond,
		Tolerance: 5 * time.Millisecond,
		Phases: map[PhaseKind]time.Duration{
			PhaseConnect: 40 * time.Millisecond,
			PhaseDNS:     10 * time.Millisecond,
		},
	}

	report := EvaluateBudget(tl, budget)
	if report.WithinLimit() {
		t.Fatalf("expected budget breaches")
	}
	if len(report.Breaches) != 3 {
		t.Fatalf("expected 3 breaches, got %d", len(report.Breaches))
	}

	var totalBreach, dnsBreach, connBreach bool
	for _, br := range report.Breaches {
		switch br.Kind {
		case PhaseTotal:
			totalBreach = true
		case PhaseDNS:
			dnsBreach = true
		case PhaseConnect:
			connBreach = true
		}
	}
	if !totalBreach || !dnsBreach || !connBreach {
		t.Fatalf("missing expected breaches: %+v", report.Breaches)
	}
}

func TestEvaluateBudgetHonoursTolerance(t *testing.T) {
	tl := &Timeline{
		Duration: 60 * time.Millisecond,
		Phases:   []Phase{{Kind: PhaseConnect, Duration: 35 * time.Millisecond}},
	}
	budget := Budget{
		Total:     50 * time.Millisecond,
		Tolerance: 15 * time.Millisecond,
		Phases: map[PhaseKind]time.Duration{
			PhaseConnect: 30 * time.Millisecond,
		},
	}

	report := EvaluateBudget(tl, budget)
	if !report.WithinLimit() {
		t.Fatalf("expected budget within limits, got %+v", report.Breaches)
	}
}
