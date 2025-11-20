package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	statsNeutralStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))
	statsMessageStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Faint(true)
	statsHeaderValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#D2D4F5"))
	statsDurationStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#56C2F4")).Bold(true)
	statsSelectedStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#343B59")).Foreground(lipgloss.Color("#E8E9F0"))
)

func colorizeStatsReport(report string, kind statsReportKind) string {
	if strings.TrimSpace(report) == "" {
		return report
	}
	switch kind {
	case statsReportKindProfile:
		return colorizeProfileStats(report)
	case statsReportKindWorkflow:
		return colorizeWorkflowStats(report)
	default:
		return report
	}
}

func colorizeProfileStats(report string) string {
	lines := strings.Split(report, "\n")
	out := make([]string, 0, len(lines))
	inFailureBlock := false
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, line)
			inFailureBlock = false
			continue
		}
		prefix := leadingIndent(line)
		lower := strings.ToLower(trimmed)
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
			out = append(out, prefix+style.Render(trimmed))
		default:
			if label, value, ok := splitLabelValue(trimmed); ok {
				out = append(out, prefix+colorizeProfileLabelValue(label, value))
				inFailureBlock = false
				continue
			}
			if inFailureBlock && strings.HasPrefix(trimmed, "-") {
				out = append(out, prefix+statsWarnStyle.Render(trimmed))
				continue
			}
			if looksLikeHistogramRow(trimmed) {
				out = append(out, prefix+statsSubLabelStyle.Render(trimmed))
				continue
			}
			if isLatencyHeaderLine(trimmed) {
				out = append(out, prefix+statsSubLabelStyle.Render(trimmed))
				continue
			}
			if isLatencyValuesLine(trimmed) {
				out = append(out, prefix+statsValueStyle.Render(trimmed))
				continue
			}
			out = append(out, prefix+trimmed)
			inFailureBlock = false
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

func colorizeProfileLabelValue(label, value string) string {
	labelStyle := statsLabelStyle
	valueStyle := statsValueStyle
	switch strings.ToLower(label) {
	case "runs":
		valueStyle = statsHeaderValueStyle
	case "success":
		valueStyle = statsSuccessStyle
		if strings.HasPrefix(strings.TrimSpace(strings.ToLower(value)), "0%") || strings.Contains(strings.ToLower(value), "n/a") {
			valueStyle = statsSubLabelStyle
		}
	case "elapsed":
		valueStyle = statsHeaderValueStyle
	case "note":
		labelStyle = statsSubLabelStyle
		valueStyle = statsWarnStyle
	}
	return renderLabelValue(label, value, labelStyle, valueStyle)
}

var latencyHeaderFields = map[string]struct{}{
	"min": {},
	"p50": {},
	"p90": {},
	"p95": {},
	"p99": {},
	"max": {},
}

var durationValueRegex = regexp.MustCompile(`\d+(?:\.\d+)?(?:ns|µs|us|ms|s|m|h)`)

func isLatencyHeaderLine(line string) bool {
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

func isLatencyValuesLine(line string) bool {
	if line == "" || strings.Contains(line, ":") {
		return false
	}
	return durationValueRegex.MatchString(line)
}

func looksLikeHistogramRow(line string) bool {
	if line == "" {
		return false
	}
	return strings.Contains(line, "|") && strings.Contains(line, "(") && strings.Contains(line, ")")
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
				out = append(out, prefix+renderLabelValue(label, value, statsLabelStyle, statsValueStyle))
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
	if strings.Contains(line, "[PASS]") || strings.Contains(line, "[FAIL]") {
		return true
	}
	return false
}

func colorizeWorkflowStepLine(line string) string {
	colored := highlightDurations(line)
	colored = strings.ReplaceAll(colored, "[PASS]", statsSuccessStyle.Render("[PASS]"))
	colored = strings.ReplaceAll(colored, "[FAIL]", statsWarnStyle.Render("[FAIL]"))
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
		if content == "PASS" || content == "FAIL" {
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
