package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestBuildTimelineReportBudgets(t *testing.T) {
	tl := &nettrace.Timeline{
		Started:   time.Unix(0, 0),
		Completed: time.Unix(0, int64(200*time.Millisecond)),
		Duration:  200 * time.Millisecond,
		Phases: []nettrace.Phase{
			{Kind: nettrace.PhaseDNS, Duration: 50 * time.Millisecond},
			{Kind: nettrace.PhaseConnect, Duration: 80 * time.Millisecond},
			{Kind: nettrace.PhaseTransfer, Duration: 70 * time.Millisecond},
		},
	}

	spec := &restfile.TraceSpec{Enabled: true}
	spec.Budgets.Total = 150 * time.Millisecond
	spec.Budgets.Tolerance = 5 * time.Millisecond
	spec.Budgets.Phases = map[string]time.Duration{
		"dns":     20 * time.Millisecond,
		"connect": 60 * time.Millisecond,
	}

	report := buildTimelineReport(tl, spec, nil, newTimelineStyles(nil))
	if len(report.rows) == 0 {
		t.Fatalf("expected rows to be populated")
	}
	if len(report.breaches) == 0 {
		t.Fatalf("expected budget breaches to be detected")
	}

	output := renderTimeline(report, 80)
	if !strings.Contains(output, "DNS lookup") {
		t.Fatalf("expected output to contain phase label, got %q", output)
	}
	if !strings.Contains(output, "budget") {
		t.Fatalf("expected output to mention budget, got %q", output)
	}
	if !strings.Contains(output, "Budget breaches") {
		t.Fatalf("expected breach summary, got %q", output)
	}
	if !strings.Contains(output, "!") {
		t.Fatalf("expected status indicator for breach, got %q", output)
	}
}

func TestCloneTraceSpecIndependence(t *testing.T) {
	src := &restfile.TraceSpec{Enabled: true}
	src.Budgets.Total = time.Second
	src.Budgets.Phases = map[string]time.Duration{"dns": 10 * time.Millisecond}

	clone := cloneTraceSpec(src)
	if clone == nil {
		t.Fatalf("expected clone to be created")
	}
	clone.Budgets.Phases["dns"] = 20 * time.Millisecond
	if src.Budgets.Phases["dns"] != 10*time.Millisecond {
		t.Fatalf("expected original map to remain unchanged, got %s", src.Budgets.Phases["dns"])
	}
}

func TestRenderTimelineSuggestsBudgetsWhenMissing(t *testing.T) {
	tl := &nettrace.Timeline{
		Duration: 100 * time.Millisecond,
		Phases: []nettrace.Phase{
			{Kind: nettrace.PhaseDNS, Duration: 40 * time.Millisecond},
			{Kind: nettrace.PhaseConnect, Duration: 60 * time.Millisecond},
		},
	}
	report := buildTimelineReport(tl, nil, nil, newTimelineStyles(nil))
	output := renderTimeline(report, 60)
	if !strings.Contains(output, "Define @trace budget to enable gating.") {
		t.Fatalf("expected suggestion for missing budgets, got %q", output)
	}
}

func TestRenderTimelineShowsStatusIndicators(t *testing.T) {
	tl := &nettrace.Timeline{
		Duration: 120 * time.Millisecond,
		Phases: []nettrace.Phase{
			{Kind: nettrace.PhaseDNS, Duration: 40 * time.Millisecond},
			{Kind: nettrace.PhaseConnect, Duration: 50 * time.Millisecond},
			{Kind: nettrace.PhaseTransfer, Duration: 30 * time.Millisecond},
		},
	}
	spec := &restfile.TraceSpec{Enabled: true}
	spec.Budgets.Total = 150 * time.Millisecond
	spec.Budgets.Phases = map[string]time.Duration{
		"dns":     80 * time.Millisecond,
		"connect": 30 * time.Millisecond,
	}
	report := buildTimelineReport(tl, spec, nil, newTimelineStyles(nil))
	output := renderTimeline(report, 80)
	if !strings.Contains(output, "✔") {
		t.Fatalf("expected within-budget indicator, got %q", output)
	}
	if !strings.Contains(output, "!") {
		t.Fatalf("expected over-budget indicator, got %q", output)
	}
}

func TestRenderTimelinePlacesTotalFirst(t *testing.T) {
	tl := &nettrace.Timeline{
		Duration: 90 * time.Millisecond,
		Phases: []nettrace.Phase{
			{Kind: nettrace.PhaseDNS, Duration: 30 * time.Millisecond},
			{Kind: nettrace.PhaseConnect, Duration: 60 * time.Millisecond},
		},
	}
	spec := &restfile.TraceSpec{Enabled: true}
	spec.Budgets.Total = 100 * time.Millisecond
	report := buildTimelineReport(tl, spec, nil, newTimelineStyles(nil))
	output := renderTimeline(report, 80)
	lines := strings.Split(output, "\n")
	rowLine := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "Timeline") ||
			strings.HasPrefix(trimmed, "Started:") ||
			strings.HasPrefix(trimmed, "Completed:") {
			continue
		}
		if strings.Contains(line, string(barGlyphFilled)) ||
			strings.Contains(line, string(barGlyphEmpty)) {
			rowLine = trimmed
			break
		}
	}
	if rowLine == "" {
		t.Fatalf("expected to find first timeline row in output: %q", output)
	}
	if !strings.Contains(rowLine, "Total") {
		t.Fatalf("expected total row to appear first, got %q", rowLine)
	}
}
