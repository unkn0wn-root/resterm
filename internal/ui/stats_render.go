package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/analysis"
)

type statsReportKind int

const (
	statsReportKindNone statsReportKind = iota
	statsReportKindProfile
	statsReportKindWorkflow
)

var (
	statsTitleStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
	statsHeadingStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Bold(true)
	statsHeadingWarn      = lipgloss.NewStyle().Foreground(lipgloss.Color("#F25F5C")).Bold(true)
	statsLabelStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB"))
	statsSubLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Faint(true)
	statsValueStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#E8E9F0")).Bold(true)
	statsSuccessStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#44C25B")).Bold(true)
	statsWarnStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#F25F5C")).Bold(true)
	statsCautionStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD46A")).Bold(true)
	statsNeutralStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))
	statsMessageStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Faint(true)
	statsHeaderValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#D2D4F5"))
	statsDurationStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#56C2F4")).Bold(true)
	statsSelectedStyle    = lipgloss.NewStyle().
				Background(lipgloss.Color("#343B59")).
				Foreground(lipgloss.Color("#E8E9F0"))
)

var latencyHeaderFields = map[string]struct{}{
	"min": {},
	"p50": {},
	"p90": {},
	"p95": {},
	"p99": {},
	"max": {},
}

var durationVal = regexp.MustCompile(`\d+(?:\.\d+)?(?:ns|µs|us|ms|s|m|h)`)

func colorizeStatsReport(
	report string,
	kind statsReportKind,
	profileStats *analysis.LatencyStats,
) string {
	if strings.TrimSpace(report) == "" {
		return report
	}
	switch kind {
	case statsReportKindProfile:
		return colorizeProfileStats(report, profileStats)
	case statsReportKindWorkflow:
		return colorizeWorkflowStats(report)
	default:
		return report
	}
}

func colorizeProfileStats(report string, stats *analysis.LatencyStats) string {
	lines := strings.Split(report, "\n")
	histogram := buildHistogramContext(lines, stats)
	out := make([]string, 0, len(lines))
	inFailureBlock := false
	legendBlock := false
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, line)
			inFailureBlock = false
			legendBlock = false
			continue
		}

		if row, ok := histogram.lines[idx]; ok {
			out = append(out, renderColoredHistogramRow(idx, row, histogram))
			inFailureBlock = false
			legendBlock = false
			continue
		}

		prefix := leadingIndent(line)
		lower := strings.ToLower(trimmed)
		if legendBlock && !strings.HasPrefix(lower, "legend:") {
			out = append(out, prefix+colorizeLegendEntry(trimmed))
			continue
		}

		switch {
		case idx == 0:
			out = append(out, prefix+statsTitleStyle.Render(trimmed))
		case isProfileHeading(lower):
			style := statsHeadingStyle
			if strings.HasPrefix(lower, "failures") {
				style = statsHeadingWarn
				inFailureBlock = true
			} else {
				inFailureBlock = false
			}
			legendBlock = false
			out = append(out, prefix+style.Render(trimmed))
		default:
			if strings.HasPrefix(lower, "legend:") {
				out = append(out, prefix+colorizeLegendHeader(trimmed))
				inFailureBlock = false
				legendBlock = true
				continue
			}
			if isLatencySummaryLine(trimmed) {
				out = append(out, prefix+renderLatencySummaryLine(trimmed))
				inFailureBlock = false
				legendBlock = false
				continue
			}
			if label, value, ok := splitLabelValue(trimmed); ok {
				out = append(out, prefix+colorizeProfileLabel(label, value))
				inFailureBlock = false
				legendBlock = false
				continue
			}
			if inFailureBlock && strings.HasPrefix(trimmed, "-") {
				out = append(out, prefix+statsWarnStyle.Render(trimmed))
				continue
			}
			if histogramRow(trimmed) {
				out = append(out, prefix+statsSubLabelStyle.Render(trimmed))
				legendBlock = false
				continue
			}
			if isLatencyHeader(trimmed) {
				out = append(out, prefix+statsSubLabelStyle.Render(trimmed))
				legendBlock = false
				continue
			}
			if isLatencyVals(trimmed) {
				out = append(out, prefix+statsValueStyle.Render(trimmed))
				legendBlock = false
				continue
			}
			out = append(out, prefix+trimmed)
			inFailureBlock = false
			legendBlock = false
		}
	}
	return strings.Join(out, "\n")
}

func isProfileHeading(line string) bool {
	line = strings.TrimSuffix(line, ":")
	switch {
	case strings.HasPrefix(line, "summary"):
		return true
	case strings.HasPrefix(line, "latency"):
		return true
	case strings.HasPrefix(line, "distribution"):
		return true
	case strings.HasPrefix(line, "failures"):
		return true
	default:
		return false
	}
}

func colorizeProfileLabel(label, value string) string {
	labelStyle := statsLabelStyle
	valueStyle := statsValueStyle

	switch strings.ToLower(label) {
	case "runs":
		valueStyle = statsHeaderValueStyle
	case "success":
		valueStyle = statsSuccessStyle
		if strings.HasPrefix(strings.TrimSpace(strings.ToLower(value)), "0%") ||
			strings.Contains(strings.ToLower(value), "n/a") {
			valueStyle = statsSubLabelStyle
		}
	case "elapsed", "window":
		valueStyle = statsHeaderValueStyle
	case "throughput":
		valueStyle = statsHeaderValueStyle
	case "note":
		labelStyle = statsSubLabelStyle
		valueStyle = statsWarnStyle
	case "status":
		valueStyle = statsHeaderValueStyle
		if strings.Contains(strings.ToLower(value), "cancel") {
			valueStyle = statsWarnStyle
		}
	}
	return renderLabelValue(label, value, labelStyle, valueStyle)
}

func isLatencyHeader(line string) bool {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if _, ok := latencyHeaderFields[strings.ToLower(f)]; !ok {
			return false
		}
	}
	return true
}

func isLatencyVals(line string) bool {
	if line == "" || strings.Contains(line, ":") {
		return false
	}
	return durationVal.MatchString(line)
}

func isLatencySummaryLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(lower, "mean:") ||
		strings.HasPrefix(lower, "median:") ||
		strings.HasPrefix(lower, "stddev:")
}

func renderLatencySummaryLine(line string) string {
	segments := strings.Split(line, "|")
	colored := make([]string, 0, len(segments))
	for _, seg := range segments {
		trimmed := strings.TrimSpace(seg)
		if label, value, ok := splitLabelValue(trimmed); ok {
			colored = append(
				colored,
				renderLabelValue(label, value, statsSubLabelStyle, statsValueStyle),
			)
		} else {
			colored = append(colored, statsSubLabelStyle.Render(trimmed))
		}
	}
	return strings.Join(colored, " | ")
}

func histogramRow(line string) bool {
	if line == "" {
		return false
	}
	return strings.Contains(line, "|") && strings.Contains(line, "(") && strings.Contains(line, ")")
}

type histogramLine struct {
	prefix      string
	from        string
	to          string
	bar         string
	barWidth    int
	count       int
	countText   string
	percentText string
	fromDur     time.Duration
	toDur       time.Duration
	fromOK      bool
	toOK        bool
}

type histogramLayout struct {
	fromWidth    int
	toWidth      int
	barWidth     int
	countWidth   int
	percentWidth int
}

type histogramContext struct {
	lines     map[int]histogramLine
	order     []int
	positions map[int]int
	layout    histogramLayout
	maxCount  int
	p50       time.Duration
	p90       time.Duration
	hasP50P90 bool
}

func buildHistogramContext(lines []string, stats *analysis.LatencyStats) histogramContext {
	p50, p90, ok := latencyThresholds(stats, lines)
	ctx := histogramContext{
		lines:     make(map[int]histogramLine),
		positions: make(map[int]int),
		p50:       p50,
		p90:       p90,
		hasP50P90: ok,
	}
	for idx, line := range lines {
		row, ok := parseHistogramLine(line)
		if !ok {
			continue
		}
		ctx.lines[idx] = row
		ctx.positions[idx] = len(ctx.order)
		ctx.order = append(ctx.order, idx)

		if w := visibleWidth(row.from); w > ctx.layout.fromWidth {
			ctx.layout.fromWidth = w
		}
		if w := visibleWidth(row.to); w > ctx.layout.toWidth {
			ctx.layout.toWidth = w
		}
		if row.barWidth > ctx.layout.barWidth {
			ctx.layout.barWidth = row.barWidth
		}
		if w := visibleWidth(row.countText); w > ctx.layout.countWidth {
			ctx.layout.countWidth = w
		}
		if w := visibleWidth(row.percentText); w > ctx.layout.percentWidth {
			ctx.layout.percentWidth = w
		}
		if row.count > ctx.maxCount {
			ctx.maxCount = row.count
		}
	}
	return ctx
}

func parseHistogramLine(line string) (histogramLine, bool) {
	if strings.TrimSpace(line) == "" {
		return histogramLine{}, false
	}

	pipeIdx := strings.Index(line, "|")
	if pipeIdx == -1 {
		return histogramLine{}, false
	}

	dashIdx, dashLen := histogramDashIndex(line[:pipeIdx])
	if dashIdx == -1 {
		return histogramLine{}, false
	}

	prefix := leadingIndent(line)
	if dashIdx <= len(prefix) || dashIdx+dashLen > pipeIdx {
		return histogramLine{}, false
	}

	openIdx := strings.Index(line[pipeIdx:], "(")
	if openIdx == -1 {
		return histogramLine{}, false
	}

	openIdx += pipeIdx
	closeIdx := strings.Index(line[openIdx:], ")")
	if closeIdx == -1 {
		return histogramLine{}, false
	}
	closeIdx += openIdx

	from := strings.TrimSpace(line[len(prefix):dashIdx])
	to := strings.TrimSpace(line[dashIdx+dashLen : pipeIdx])

	barRaw := line[pipeIdx+1 : openIdx]
	barField := strings.TrimLeft(barRaw, " ")
	bar := strings.TrimSpace(barField)
	barWidth := visibleWidth(barField)
	if openIdx > 0 && line[openIdx-1] == ' ' && barWidth > 0 {
		barWidth--
	}
	if barWidth <= 0 {
		barWidth = visibleWidth(bar)
	}
	if barWidth == 0 {
		barWidth = histogramBarWidth
	}

	countText := strings.TrimSpace(line[openIdx+1 : closeIdx])
	percentText := strings.TrimSpace(line[closeIdx+1:])
	countVal, _ := strconv.Atoi(countText)
	fromDur, fromOK := parseDurationValue(from)
	toDur, toOK := parseDurationValue(to)

	return histogramLine{
		prefix:      prefix,
		from:        from,
		to:          to,
		bar:         bar,
		barWidth:    barWidth,
		count:       countVal,
		countText:   countText,
		percentText: percentText,
		fromDur:     fromDur,
		toDur:       toDur,
		fromOK:      fromOK,
		toOK:        toOK,
	}, true
}

func histogramDashIndex(segment string) (int, int) {
	separators := []string{" – ", " - ", "–", "-"}
	for _, sep := range separators {
		if idx := strings.Index(segment, sep); idx != -1 {
			return idx, len(sep)
		}
	}
	return -1, 0
}

func renderColoredHistogramRow(lineIdx int, row histogramLine, ctx histogramContext) string {
	layout := ctx.layout

	fromWidth := layout.fromWidth
	if fromWidth == 0 {
		fromWidth = visibleWidth(row.from)
	}

	toWidth := layout.toWidth
	if toWidth == 0 {
		toWidth = visibleWidth(row.to)
	}

	barWidth := layout.barWidth
	if barWidth == 0 {
		barWidth = visibleWidth(row.bar)
	}

	countWidth := layout.countWidth
	if countWidth == 0 {
		countWidth = visibleWidth(row.countText)
	}

	percentWidth := layout.percentWidth
	if percentWidth == 0 {
		percentWidth = visibleWidth(row.percentText)
	}

	barPadding := barWidth - visibleWidth(row.bar)
	if barPadding < 0 {
		barPadding = 0
	}
	paddedBar := row.bar + strings.Repeat(" ", barPadding)

	barStyle := histogramBarStyle(lineIdx, row, ctx)
	coloredBar := barStyle.Render(paddedBar)

	from := fmt.Sprintf("%-*s", fromWidth, row.from)
	to := fmt.Sprintf("%-*s", toWidth, row.to)
	count := fmt.Sprintf("%-*s", countWidth, row.countText)
	percent := fmt.Sprintf("%*s", percentWidth, row.percentText)

	var builder strings.Builder
	builder.WriteString(row.prefix)
	builder.WriteString(statsLabelStyle.Render(from))
	builder.WriteString(" – ")
	builder.WriteString(statsLabelStyle.Render(to))
	builder.WriteString(" | ")
	builder.WriteString(coloredBar)
	builder.WriteString(" (")
	builder.WriteString(statsHeaderValueStyle.Render(count))
	builder.WriteString(") ")
	builder.WriteString(statsValueStyle.Render(percent))

	return builder.String()
}

func histogramBarStyle(lineIdx int, row histogramLine, ctx histogramContext) lipgloss.Style {
	if row.count == 0 {
		return statsSubLabelStyle
	}
	if ctx.maxCount > 0 {
		share := float64(row.count) / float64(ctx.maxCount)
		if share < histogramFadeShare {
			if !ctx.hasP50P90 || !bucketTouchesOrExceeds(row, ctx.p90) {
				return statsSubLabelStyle
			}
		}
	}

	if ctx.hasP50P90 {
		if row.toOK && row.toDur <= ctx.p50 {
			return statsSuccessStyle
		}
		if bucketTouchesOrExceeds(row, ctx.p90) {
			return statsWarnStyle
		}
		if bucketTouchesOrExceeds(row, ctx.p50) {
			return statsCautionStyle
		}
		if mid, ok := bucketMidpoint(row); ok {
			switch {
			case mid <= ctx.p50:
				return statsSuccessStyle
			case mid < ctx.p90:
				return statsCautionStyle
			default:
				return statsWarnStyle
			}
		}
		return statsCautionStyle
	}

	total := len(ctx.order)
	if total <= 1 {
		return statsSuccessStyle
	}
	pos := ctx.positions[lineIdx]
	ratio := float64(pos) / float64(total-1)
	switch {
	case ratio >= 0.85:
		return statsWarnStyle
	case ratio >= 0.6:
		return statsCautionStyle
	default:
		return statsSuccessStyle
	}
}

func bucketMidpoint(row histogramLine) (time.Duration, bool) {
	switch {
	case row.fromOK && row.toOK:
		return row.fromDur + (row.toDur-row.fromDur)/2, true
	case row.toOK:
		return row.toDur, true
	case row.fromOK:
		return row.fromDur, true
	default:
		return 0, false
	}
}

func bucketTouchesOrExceeds(row histogramLine, threshold time.Duration) bool {
	if row.fromOK && row.fromDur >= threshold {
		return true
	}
	if row.toOK && row.toDur >= threshold {
		return true
	}
	return false
}

func parseDurationValue(val string) (time.Duration, bool) {
	d, err := time.ParseDuration(strings.TrimSpace(val))
	if err != nil {
		return 0, false
	}
	return d, true
}

func parseLatencyThresholds(lines []string) (time.Duration, time.Duration, bool) {
	for idx, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		if !strings.EqualFold(fields[0], "min") || !strings.EqualFold(fields[1], "p50") ||
			!strings.EqualFold(fields[2], "p90") {
			continue
		}
		for j := idx + 1; j < len(lines); j++ {
			valLine := strings.TrimSpace(lines[j])
			if valLine == "" {
				continue
			}

			vals := strings.Fields(valLine)
			if len(vals) < 3 {
				break
			}

			p50, ok50 := parseDurationValue(vals[1])
			p90, ok90 := parseDurationValue(vals[2])
			if ok50 && ok90 {
				return p50, p90, true
			}
			break
		}
	}
	return 0, 0, false
}

func latencyThresholds(
	stats *analysis.LatencyStats,
	lines []string,
) (time.Duration, time.Duration, bool) {
	if p50, p90, ok := percentileThresholds(stats); ok {
		return p50, p90, true
	}
	return parseLatencyThresholds(lines)
}

func percentileThresholds(stats *analysis.LatencyStats) (time.Duration, time.Duration, bool) {
	if stats == nil {
		return 0, 0, false
	}

	p50, ok50 := percentileFromStats(stats, 50)
	p90, ok90 := percentileFromStats(stats, 90)
	if ok50 && ok90 {
		return p50, p90, true
	}
	return 0, 0, false
}

func percentileFromStats(stats *analysis.LatencyStats, p int) (time.Duration, bool) {
	if stats == nil {
		return 0, false
	}
	if stats.Percentiles != nil {
		if v, ok := stats.Percentiles[p]; ok {
			return v, true
		}
	}
	if p == 50 && stats.Median > 0 {
		return stats.Median, true
	}
	return 0, false
}

func colorizeLegendHeader(line string) string {
	return statsSubLabelStyle.Render(line)
}

func colorizeLegendEntry(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return line
	}

	color := strings.ToLower(fields[0])
	rest := strings.Join(fields[1:], " ")
	style := statsSubLabelStyle
	switch color {
	case "green":
		style = statsSuccessStyle
	case "yellow":
		style = statsCautionStyle
	case "red":
		style = statsWarnStyle
	}

	coloredFirst := style.Render(fields[0])
	if rest == "" {
		return coloredFirst
	}
	return coloredFirst + " " + statsSubLabelStyle.Render(rest)
}

func colorizeWorkflowStats(report string) string {
	lines := strings.Split(report, "\n")
	out := make([]string, 0, len(lines))
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, line)
			continue
		}

		prefix := leadingIndent(line)
		if idx == 0 {
			out = append(out, prefix+statsTitleStyle.Render(trimmed))
			continue
		}

		if label, value, ok := splitLabelValue(trimmed); ok {
			lower := strings.ToLower(label)
			if lower == "workflow" || lower == "started" || lower == "steps" {
				out = append(
					out,
					prefix+renderLabelValue(label, value, statsLabelStyle, statsValueStyle),
				)
				continue
			}
		}

		if isWorkflowStepLine(trimmed) {
			colored := colorizeWorkflowStepLine(trimmed)
			out = append(out, prefix+colored)
			continue
		}

		if strings.HasPrefix(line, "    ") {
			out = append(out, prefix+statsMessageStyle.Render(trimmed))
			continue
		}
		out = append(out, prefix+trimmed)
	}
	return strings.Join(out, "\n")
}

func renderLabelValue(label, value string, labelStyle, valueStyle lipgloss.Style) string {
	rendered := labelStyle.Render(label + ":")
	if strings.TrimSpace(value) == "" {
		return rendered
	}
	return rendered + " " + valueStyle.Render(value)
}

func splitLabelValue(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx == -1 {
		return "", "", false
	}
	label := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])
	return label, value, true
}

func isWorkflowStepLine(line string) bool {
	if line == "" {
		return false
	}
	return strings.Contains(line, workflowStatusPass) ||
		strings.Contains(line, workflowStatusFail) ||
		strings.Contains(line, workflowStatusCanceled) ||
		strings.Contains(line, workflowStatusSkipped)
}

func colorizeWorkflowStepLine(line string) string {
	colored := highlightDurations(line)
	colored = strings.ReplaceAll(
		colored,
		workflowStatusPass,
		statsSuccessStyle.Render(workflowStatusPass),
	)
	colored = strings.ReplaceAll(
		colored,
		workflowStatusFail,
		statsWarnStyle.Render(workflowStatusFail),
	)
	colored = strings.ReplaceAll(
		colored,
		workflowStatusCanceled,
		statsCautionStyle.Render(workflowStatusCanceled),
	)
	colored = strings.ReplaceAll(
		colored,
		workflowStatusSkipped,
		statsCautionStyle.Render(workflowStatusSkipped),
	)
	colored = highlightParentheticals(colored)
	return colored
}

func highlightDurations(line string) string {
	var builder strings.Builder
	remaining := line
	for {
		start := strings.Index(remaining, "[")
		if start == -1 {
			builder.WriteString(remaining)
			break
		}

		end := strings.Index(remaining[start+1:], "]")
		if end == -1 {
			builder.WriteString(remaining)
			break
		}

		end += start + 1
		builder.WriteString(remaining[:start])
		content := remaining[start+1 : end]
		if content == "PASS" || content == "FAIL" || content == "CANCELED" || content == "SKIPPED" {
			builder.WriteString("[" + content + "]")
		} else {
			builder.WriteString(statsDurationStyle.Render("[" + content + "]"))
		}

		remaining = remaining[end+1:]
	}
	return builder.String()
}

func highlightParentheticals(line string) string {
	var builder strings.Builder
	remaining := line
	for {
		start := strings.Index(remaining, "(")
		if start == -1 {
			builder.WriteString(remaining)
			break
		}

		end := strings.Index(remaining[start+1:], ")")
		if end == -1 {
			builder.WriteString(remaining)
			break
		}

		end += start + 1
		builder.WriteString(remaining[:start])
		content := remaining[start : end+1]
		builder.WriteString(statsNeutralStyle.Render(content))
		remaining = remaining[end+1:]
	}
	return builder.String()
}
