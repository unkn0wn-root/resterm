package history

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
)

func TestNewTraceSummary(t *testing.T) {
	tl := &nettrace.Timeline{
		Started:   time.Unix(0, 0),
		Completed: time.Unix(0, int64(15*time.Millisecond)),
		Duration:  15 * time.Millisecond,
		Phases: []nettrace.Phase{
			{
				Kind:     nettrace.PhaseDNS,
				Duration: 5 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "example.com", Cached: true},
			},
			{
				Kind:     nettrace.PhaseConnect,
				Duration: 10 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "93.184.216.34:443"},
			},
		},
	}

	report := nettrace.NewReport(tl, nettrace.Budget{Total: 10 * time.Millisecond})
	summary := NewTraceSummary(tl, report)
	if summary == nil {
		t.Fatalf("expected summary")
	}
	if summary.Duration != 15*time.Millisecond {
		t.Fatalf("unexpected duration: %v", summary.Duration)
	}
	if len(summary.Phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(summary.Phases))
	}
	if summary.Phases[0].Kind != string(nettrace.PhaseDNS) {
		t.Fatalf("unexpected first phase kind: %s", summary.Phases[0].Kind)
	}
	if !summary.Phases[0].Meta.Cached {
		t.Fatalf("expected cached flag to propagate")
	}
	if summary.Phases[1].Meta.Addr != "93.184.216.34:443" {
		t.Fatalf("unexpected address metadata: %s", summary.Phases[1].Meta.Addr)
	}
	if summary.Budgets == nil || summary.Budgets.Total != 10*time.Millisecond {
		t.Fatalf("expected budgets to be captured")
	}
	if len(summary.Breaches) == 0 {
		t.Fatalf("expected breach to be present")
	}
}

func TestNewTraceSummaryNil(t *testing.T) {
	if summary := NewTraceSummary(nil, nil); summary != nil {
		t.Fatalf("expected nil summary for nil timeline")
	}
}

func TestTraceSummaryRoundTrip(t *testing.T) {
	tl := &nettrace.Timeline{
		Started:   time.Unix(0, 0),
		Completed: time.Unix(0, int64(120*time.Millisecond)),
		Duration:  120 * time.Millisecond,
		Phases: []nettrace.Phase{
			{
				Kind:     nettrace.PhaseDNS,
				Duration: 40 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "example.com", Cached: true},
			},
			{
				Kind:     nettrace.PhaseConnect,
				Duration: 50 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "93.184.216.34:443"},
			},
			{Kind: nettrace.PhaseTransfer, Duration: 30 * time.Millisecond},
		},
	}
	budget := nettrace.Budget{
		Total: 90 * time.Millisecond,
		Phases: map[nettrace.PhaseKind]time.Duration{
			nettrace.PhaseDNS:     20 * time.Millisecond,
			nettrace.PhaseConnect: 40 * time.Millisecond,
		},
	}
	report := nettrace.NewReport(tl, budget)
	summary := NewTraceSummary(tl, report)
	if summary == nil {
		t.Fatalf("expected summary from report")
	}
	rebuilt := summary.Timeline()
	if rebuilt == nil {
		t.Fatalf("expected timeline reconstruction")
	}
	if len(rebuilt.Phases) != len(tl.Phases) {
		t.Fatalf("expected %d phases, got %d", len(tl.Phases), len(rebuilt.Phases))
	}
	for i, phase := range rebuilt.Phases {
		if phase.Duration != tl.Phases[i].Duration {
			t.Fatalf(
				"phase %d duration mismatch: %s vs %s",
				i,
				phase.Duration,
				tl.Phases[i].Duration,
			)
		}
		if phase.Meta.Addr != tl.Phases[i].Meta.Addr {
			t.Fatalf("phase %d addr mismatch", i)
		}
	}
	rebuiltReport := summary.Report()
	if rebuiltReport == nil {
		t.Fatalf("expected report reconstruction")
	}
	if rebuiltReport.Budget.Total != budget.Total {
		t.Fatalf("expected budget total %s, got %s", budget.Total, rebuiltReport.Budget.Total)
	}
	if len(rebuiltReport.BudgetReport.Breaches) != len(report.BudgetReport.Breaches) {
		t.Fatalf(
			"expected %d breaches, got %d",
			len(report.BudgetReport.Breaches),
			len(rebuiltReport.BudgetReport.Breaches),
		)
	}
}
