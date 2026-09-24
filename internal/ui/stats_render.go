package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/termtext"
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
	statsHeaderValueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F0FF"))
	statsDurationStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#56C2F4")).Bold(true)
	statsSelectedStyle    = lipgloss.NewStyle().
				Background(lipgloss.Color("#343B59")).
				Foreground(lipgloss.Color("#E8E9F0"))
)

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
	rendered := labelStyle.Render(termtext.Row(label) + ":")
	if strings.TrimSpace(value) == "" {
		return rendered
	}
	return rendered + " " + valueStyle.Render(termtext.Row(value))
}

func splitLabelValue(line string) (string, string, bool) {
	before, after, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	label := strings.TrimSpace(before)
	value := strings.TrimSpace(after)
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
