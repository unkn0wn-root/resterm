package nettrace

import (
	"testing"
	"time"
)

func TestNewReportClonesTimelineAndEvaluatesBudgets(t *testing.T) {
	tl := &Timeline{
		Started:   time.Unix(0, 0),
		Completed: time.Unix(0, int64(20*time.Millisecond)),
		Duration:  20 * time.Millisecond,
		Phases: []Phase{
			{Kind: PhaseDNS, Duration: 5 * time.Millisecond},
			{Kind: PhaseConnect, Duration: 15 * time.Millisecond},
		},
	}

	budget := Budget{Total: 10 * time.Millisecond}
	report := NewReport(tl, budget)
	if report == nil {
		t.Fatalf("expected report")
	}
	if report.Timeline == tl {
		t.Fatalf("expected timeline clone to avoid sharing")
	}
	if report.Budget.Total != budget.Total {
		t.Fatalf("expected budget to be cloned")
	}
	if len(report.BudgetReport.Breaches) == 0 {
		t.Fatalf("expected breach when total budget exceeded")
	}
}

func TestNewReportHandlesNilTimeline(t *testing.T) {
	if rep := NewReport(nil, Budget{}); rep != nil {
		t.Fatalf("expected nil report for nil timeline")
	}
}
