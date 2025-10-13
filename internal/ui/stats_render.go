package ui

import (
	"strconv"
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
	statsTitleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
	statsHeadingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Bold(true)
	statsHeadingWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("#F25F5C")).Bold(true)
	statsLabelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB"))
	statsSubLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Faint(true)
	statsValueStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#E8E9F0")).Bold(true)
	statsSuccessStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#44C25B")).Bold(true)
	statsWarnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#F25F5C")).Bold(true)
	statsNeutralStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))
	statsMessageStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB")).Faint(true)
	statsDurationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#56C2F4")).Bold(true)
	statsSelectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("#343B59")).Foreground(lipgloss.Color("#E8E9F0"))
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
		switch {
		case idx == 0:
			out = append(out, prefix+statsTitleStyle.Render(trimmed))
		case trimmed == "No successful measurements.":
			out = append(out, prefix+statsWarnStyle.Render(trimmed))
		case strings.EqualFold(trimmed, "Failures:"):
			out = append(out, prefix+statsHeadingWarn.Render(trimmed))
			inFailureBlock = true
		default:
			label, value, ok := splitLabelValue(trimmed)
			if ok {
				lower := strings.ToLower(label)
				switch lower {
				case "measured runs":
					style := statsSuccessStyle
					if !isNonZero(value) {
						style = statsSubLabelStyle
					}
					out = append(out, prefix+renderLabelValue(label, value, statsLabelStyle, style))
				case "warmup runs":
					out = append(out, prefix+renderLabelValue(label, value, statsLabelStyle, statsValueStyle))
				case "failures":
					style := statsSuccessStyle
					if isNonZero(value) {
						style = statsWarnStyle
					}
					out = append(out, prefix+renderLabelValue(label, value, statsLabelStyle, style))
				case "latency summary", "percentiles", "histogram":
					out = append(out, prefix+statsHeadingStyle.Render(label+":"))
					if lower == "histogram" {
						inFailureBlock = false
					}
				default:
					out = append(out, prefix+renderLabelValue(label, value, statsSubLabelStyle, statsValueStyle))
				}
				if lower != "failures" {
					inFailureBlock = false
				}
				continue
			}
			if inFailureBlock && strings.HasPrefix(trimmed, "-") {
				out = append(out, prefix+statsWarnStyle.Render(trimmed))
				continue
			}
			if strings.Contains(trimmed, "|") && strings.Contains(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
				out = append(out, prefix+statsSubLabelStyle.Render(trimmed))
				continue
			}
			out = append(out, prefix+trimmed)
			inFailureBlock = false
		}
	}
	return strings.Join(out, "\n")
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

func isNonZero(value string) bool {
	if value == "" {
		return false
	}
	if i, err := strconv.Atoi(strings.Fields(value)[0]); err == nil {
		return i != 0
	}
	return strings.TrimSpace(value) != "0"
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
