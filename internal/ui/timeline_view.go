package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/traceutil"
)

const (
	barRowWidth    = 34
	barGlyphFilled = "█"
	barGlyphEmpty  = "░"
)

type timelineStatus int

const (
	timelineStatusNone timelineStatus = iota
	timelineStatusOK
	timelineStatusWarn
)

type timelineStyles struct {
	title      lipgloss.Style
	phase      lipgloss.Style
	barOK      lipgloss.Style
	barWarn    lipgloss.Style
	meta       lipgloss.Style
	emph       lipgloss.Style
	statusOK   lipgloss.Style
	statusWarn lipgloss.Style
}

func newTimelineStyles(th *theme.Theme) timelineStyles {
	styles := timelineStyles{
		title:      lipgloss.NewStyle().Bold(true),
		phase:      lipgloss.NewStyle().Bold(true),
		barOK:      lipgloss.NewStyle(),
		barWarn:    lipgloss.NewStyle().Bold(true),
		meta:       lipgloss.NewStyle().Faint(true),
		emph:       lipgloss.NewStyle().Bold(true),
		statusOK:   lipgloss.NewStyle().Bold(true),
		statusWarn: lipgloss.NewStyle().Bold(true),
	}
	if th == nil {
		return styles
	}

	styles.title = th.HeaderTitle.Bold(true)
	styles.phase = th.ResponseContent.Bold(true)
	styles.emph = th.ResponseContent.Bold(true)
	styles.meta = th.ResponseContent.Faint(true)
	styles.barOK = th.Success
	styles.barWarn = th.Error.Bold(true)
	styles.statusOK = th.Success.Bold(true)
	styles.statusWarn = th.Error.Bold(true)
	return styles
}

// timelineReport encapsulates everything required to render the timeline pane.
type timelineReport struct {
	title         string
	summary       []string
	rows          []timelineRow
	breaches      []nettrace.BudgetBreach
	budgets       nettrace.Budget
	totalLimit    time.Duration
	totalDuration time.Duration
	hasBudget     bool
	details       *nettrace.TraceDetails
	detailsNow    time.Time
	styles        timelineStyles
}

type timelineRow struct {
	Phase    nettrace.PhaseKind
	Name     string
	Duration time.Duration
	Percent  float64
	Budget   time.Duration
	Overrun  time.Duration
	Meta     nettrace.PhaseMeta
	Error    string
	Status   timelineStatus
}

func buildTimelineReport(
	tl *nettrace.Timeline,
	spec *restfile.TraceSpec,
	rep *nettrace.Report,
	styles timelineStyles,
) timelineReport {
	report := timelineReport{styles: styles}
	if tl == nil {
		report.title = "Timeline"
		report.summary = []string{styles.meta.Render("Trace data unavailable.")}
		return report
	}
	if spec != nil && !spec.Enabled {
		spec = nil
	}

	report.title = fmt.Sprintf("Timeline – %s", tl.Duration.Round(time.Microsecond))
	report.summary = buildTimelineSummary(tl, styles)
	rows := buildTimelineRows(tl)
	report.totalDuration = tl.Duration
	if tl.Details != nil {
		report.details = tl.Details.Clone()
	}
	report.detailsNow = timelineDetailsClock(tl)

	var (
		budget    nettrace.Budget
		hasBudget bool
		breaches  []nettrace.BudgetBreach
	)

	if rep != nil {
		budget = rep.Budget
		hasBudget = traceutil.HasBudget(budget)
		if len(rep.BudgetReport.Breaches) > 0 {
			breaches = append([]nettrace.BudgetBreach(nil), rep.BudgetReport.Breaches...)
		}
	}

	if !hasBudget && spec != nil {
		if b, ok := traceutil.BudgetFromSpec(spec); ok {
			budget = b
			hasBudget = true
		}
	}

	if hasBudget && budget.Total > 0 {
		totalRow := timelineRow{
			Phase:    nettrace.PhaseTotal,
			Name:     humanPhaseName(nettrace.PhaseTotal),
			Duration: tl.Duration,
			Percent:  100,
		}
		rows = append([]timelineRow{totalRow}, rows...)
	}

	report.rows = rows
	report.hasBudget = hasBudget

	if hasBudget {
		report.budgets = budget
		report.totalLimit = budget.Total
		if len(breaches) == 0 {
			report.breaches = nettrace.EvaluateBudget(tl, budget).Breaches
		} else {
			report.breaches = breaches
		}
		applyBudgetToRows(report.rows, report.budgets, report.breaches)
	}

	return report
}

func buildTimelineSummary(tl *nettrace.Timeline, styles timelineStyles) []string {
	if tl == nil {
		return nil
	}
	lines := []string{
		styles.meta.Render(fmt.Sprintf("Started:   %s", formatTime(tl.Started))),
	}
	if !tl.Completed.IsZero() {
		lines = append(
			lines,
			styles.meta.Render(fmt.Sprintf("Completed: %s", formatTime(tl.Completed))),
		)
	}
	if trimmed := strings.TrimSpace(tl.Err); trimmed != "" {
		lines = append(lines, styles.statusWarn.Render("Error: "+trimmed))
	}
	return lines
}

func buildTimelineRows(tl *nettrace.Timeline) []timelineRow {
	if tl == nil || len(tl.Phases) == 0 {
		return nil
	}
	total := tl.Duration
	if total <= 0 {
		for _, phase := range tl.Phases {
			if phase.Duration > total {
				total = phase.Duration
			}
		}
	}

	combined := combinePhases(tl.Phases)
	rows := make([]timelineRow, 0, len(combined))
	for _, phase := range combined {
		percent := 0.0
		if total > 0 {
			percent = float64(phase.Duration) / float64(total) * 100
		}
		rows = append(rows, timelineRow{
			Phase:    phase.Kind,
			Name:     humanPhaseName(phase.Kind),
			Duration: phase.Duration,
			Percent:  percent,
			Meta:     phase.Meta,
			Error:    phase.Err,
		})
	}
	return rows
}

func combinePhases(phases []nettrace.Phase) []nettrace.Phase {
	aggregated := make(map[nettrace.PhaseKind]nettrace.Phase)
	order := make([]nettrace.PhaseKind, 0, len(phases))
	for _, phase := range phases {
		if phase.Kind == "" {
			continue
		}
		if _, seen := aggregated[phase.Kind]; !seen {
			order = append(order, phase.Kind)
		}
		entry := aggregated[phase.Kind]
		entry.Kind = phase.Kind
		entry.Duration += phase.Duration
		if phase.Meta.Addr != "" {
			entry.Meta = phase.Meta
		}
		if phase.Err != "" {
			entry.Err = phase.Err
		}
		aggregated[phase.Kind] = entry
	}
	sort.SliceStable(order, func(i, j int) bool {
		return phaseOrder(order[i]) < phaseOrder(order[j])
	})
	result := make([]nettrace.Phase, 0, len(aggregated))
	for _, kind := range order {
		result = append(result, aggregated[kind])
	}
	return result
}

func phaseOrder(kind nettrace.PhaseKind) int {
	switch kind {
	case nettrace.PhaseDNS:
		return 0
	case nettrace.PhaseConnect:
		return 1
	case nettrace.PhaseTLS:
		return 2
	case nettrace.PhaseReqHdrs:
		return 3
	case nettrace.PhaseReqBody:
		return 4
	case nettrace.PhaseTTFB:
		return 5
	case nettrace.PhaseTransfer:
		return 6
	case nettrace.PhaseTotal:
		return 7
	default:
		return 8
	}
}

func humanPhaseName(kind nettrace.PhaseKind) string {
	switch kind {
	case nettrace.PhaseDNS:
		return "DNS lookup"
	case nettrace.PhaseConnect:
		return "TCP connect"
	case nettrace.PhaseTLS:
		return "TLS handshake"
	case nettrace.PhaseReqHdrs:
		return "Request headers"
	case nettrace.PhaseReqBody:
		return "Request body"
	case nettrace.PhaseTTFB:
		return "TTFB"
	case nettrace.PhaseTransfer:
		return "Transfer"
	case nettrace.PhaseTotal:
		return "Total"
	default:
		return strings.ToUpper(string(kind))
	}
}

func applyBudgetToRows(
	rows []timelineRow,
	budget nettrace.Budget,
	breaches []nettrace.BudgetBreach,
) {
	breachMap := make(map[nettrace.PhaseKind]nettrace.BudgetBreach, len(breaches))
	for _, br := range breaches {
		breachMap[br.Kind] = br
	}
	for idx := range rows {
		row := &rows[idx]
		if limit, ok := budget.Phases[row.Phase]; ok {
			row.Budget = limit
		}
		if br, ok := breachMap[row.Phase]; ok {
			row.Overrun = br.Over
		}
	}
	if br, ok := breachMap[nettrace.PhaseTotal]; ok {
		for idx := range rows {
			if rows[idx].Phase == nettrace.PhaseTotal {
				rows[idx].Budget = br.Limit
				rows[idx].Overrun = br.Over
				break
			}
		}
	}
	for idx := range rows {
		switch {
		case rows[idx].Overrun > 0:
			rows[idx].Status = timelineStatusWarn
		case rows[idx].Budget > 0:
			rows[idx].Status = timelineStatusOK
		default:
			rows[idx].Status = timelineStatusNone
		}
	}
}

func renderTimeline(report timelineReport, width int) string {
	if width <= 0 {
		width = defaultResponseViewportWidth
	}

	builder := strings.Builder{}
	builder.WriteString(report.styles.title.Render(report.title))
	builder.WriteString("\n")

	if len(report.summary) > 0 {
		for _, line := range report.summary {
			builder.WriteString("  " + line + "\n")
		}
		builder.WriteString("\n")
	}

	if len(report.rows) == 0 {
		builder.WriteString("Trace timeline unavailable.\n")
		return builder.String()
	}

	total := report.totalDuration
	if total <= 0 {
		total = sumRowDurations(report.rows)
	}
	if total <= 0 {
		total = maxRowDuration(report.rows)
	}

	maxWidth := width - 32
	if maxWidth < 10 {
		maxWidth = 10
	}
	barWidth := clamp(barRowWidth, 10, maxWidth)
	for _, row := range report.rows {
		builder.WriteString(renderTimelineRow(row, total, barWidth, report.styles))
	}

	if len(report.breaches) > 0 {
		builder.WriteString("\n")
		builder.WriteString(report.styles.statusWarn.Render("Budget breaches:"))
		builder.WriteString("\n")
		for _, br := range report.breaches {
			msg := fmt.Sprintf(
				"  %s: over by %s (limit %s)",
				humanPhaseName(
					br.Kind,
				),
				br.Over.Round(time.Millisecond),
				br.Limit.Round(time.Millisecond),
			)
			builder.WriteString(report.styles.meta.Render(msg))
			builder.WriteString("\n")
		}
	}
	if !report.hasBudget {
		builder.WriteString("\n")
		builder.WriteString(report.styles.meta.Render("Define @trace budget to enable gating."))
		builder.WriteString("\n")
	}

	if details := renderTraceDetails(
		report.details,
		report.styles,
		report.detailsNow,
	); len(
		details,
	) > 0 {
		builder.WriteString("\n")
		for _, line := range details {
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}

	return strings.TrimRight(builder.String(), "\n") + "\n"
}

func renderTimelineRow(
	row timelineRow,
	total time.Duration,
	barWidth int,
	styles timelineStyles,
) string {
	bar := renderTimelineBar(row.Duration, total, barWidth, row.Overrun > 0, styles)
	status := renderTimelineStatus(row.Status, styles)
	label := styles.phase.Render(fmt.Sprintf("%-16s", row.Name))
	duration := styles.emph.Render(row.Duration.Round(time.Millisecond).String())
	percent := styles.meta.Render(fmt.Sprintf("%5.1f%%", row.Percent))
	extra := renderBudgetNote(row, styles)

	var builder strings.Builder
	builder.WriteString(status)
	builder.WriteString(" ")
	builder.WriteString(label)
	builder.WriteString(" ")
	builder.WriteString(bar)
	builder.WriteString(" ")
	builder.WriteString(duration)
	builder.WriteString(" ")
	builder.WriteString(percent)
	if extra != "" {
		builder.WriteString(" ")
		builder.WriteString(extra)
	}
	builder.WriteString("\n")

	if trimmed := strings.TrimSpace(row.Error); trimmed != "" {
		builder.WriteString("  ")
		builder.WriteString(styles.statusWarn.Render("error: " + trimmed))
		builder.WriteString("\n")
	}
	if meta := renderPhaseMeta(row.Meta); meta != "" {
		builder.WriteString("  ")
		builder.WriteString(styles.meta.Render(meta))
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderTimelineBar(
	duration time.Duration,
	total time.Duration,
	width int,
	warn bool,
	styles timelineStyles,
) string {
	if width <= 0 {
		return ""
	}
	var ratio float64
	if total > 0 {
		ratio = float64(duration) / float64(total)
	}
	filled := int(math.Round(ratio * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	empty := width - filled
	filledGlyph := strings.Repeat(barGlyphFilled, filled)
	if warn {
		filledGlyph = styles.barWarn.Render(filledGlyph)
	} else {
		filledGlyph = styles.barOK.Render(filledGlyph)
	}
	return filledGlyph + strings.Repeat(barGlyphEmpty, empty)
}

func renderTimelineStatus(status timelineStatus, styles timelineStyles) string {
	switch status {
	case timelineStatusWarn:
		return styles.statusWarn.Render("!")
	case timelineStatusOK:
		return styles.statusOK.Render("✔")
	default:
		return " "
	}
}

func renderBudgetNote(row timelineRow, styles timelineStyles) string {
	var parts []string
	if row.Budget > 0 {
		label := fmt.Sprintf("budget %s", row.Budget.Round(time.Millisecond))
		parts = append(parts, styles.meta.Render(label))
	}
	if row.Overrun > 0 {
		flag := fmt.Sprintf("(over +%s)", row.Overrun.Round(time.Millisecond))
		parts = append(parts, styles.statusWarn.Render(flag))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func renderPhaseMeta(meta nettrace.PhaseMeta) string {
	var parts []string
	if strings.TrimSpace(meta.Addr) != "" {
		parts = append(parts, fmt.Sprintf("addr=%s", meta.Addr))
	}
	if meta.Reused {
		parts = append(parts, "reused")
	}
	if meta.Cached {
		parts = append(parts, "cached")
	}
	return strings.Join(parts, " ")
}

func sumRowDurations(rows []timelineRow) time.Duration {
	var total time.Duration
	for _, row := range rows {
		if row.Phase == nettrace.PhaseTotal {
			continue
		}
		total += row.Duration
	}
	return total
}

func maxRowDuration(rows []timelineRow) time.Duration {
	var max time.Duration
	for _, row := range rows {
		if row.Duration > max {
			max = row.Duration
		}
	}
	return max
}

// cloneTraceSpec returns a deep copy of the provided trace spec so downstream consumers
// can safely mutate budget limits without affecting the source request metadata.
func cloneTraceSpec(spec *restfile.TraceSpec) *restfile.TraceSpec {
	if spec == nil {
		return nil
	}
	clone := &restfile.TraceSpec{Enabled: spec.Enabled}
	clone.Budgets.Total = spec.Budgets.Total
	clone.Budgets.Tolerance = spec.Budgets.Tolerance
	if len(spec.Budgets.Phases) > 0 {
		phases := make(map[string]time.Duration, len(spec.Budgets.Phases))
		for name, limit := range spec.Budgets.Phases {
			phases[name] = limit
		}
		clone.Budgets.Phases = phases
	}
	return clone
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

func timelineDetailsClock(tl *nettrace.Timeline) time.Time {
	if tl == nil {
		return time.Now()
	}
	if !tl.Completed.IsZero() {
		return tl.Completed
	}
	if !tl.Started.IsZero() {
		return tl.Started
	}
	return time.Now()
}
