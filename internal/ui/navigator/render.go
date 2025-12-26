package navigator

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

// ListView renders the navigator list with an optional height constraint.
func ListView(m *Model[any], th theme.Theme, width int, height int, focus bool) string {
	if m == nil {
		return ""
	}
	if width < 1 {
		width = 1
	}

	m.SetViewportHeight(height)
	rows := m.VisibleRows()
	var out []string
	for i, row := range rows {
		selected := (m.offset + i) == m.sel
		out = append(out, renderRow(row, selected, th, width, focus, m.compact))
	}
	return strings.Join(out, "\n")
}

func renderRow(
	row Flat[any],
	selected bool,
	th theme.Theme,
	width int,
	focus bool,
	compact bool,
) string {
	n := row.Node
	if n == nil {
		return ""
	}

	pad := strings.Repeat("  ", row.Level)
	parts := []string{pad, rowIcon(n)}
	if n.Kind == KindWorkflow {
		parts = append(parts, renderWorkflowBadge(th))
	}
	if n.Method != "" {
		parts = append(parts, renderMethodBadge(n.Method, th))
	}

	title := n.Title
	if n.Kind == KindFile && n.Count > 0 {
		title = fmt.Sprintf("%s (%d)", title, n.Count)
	}

	titleStyle := th.NavigatorTitle
	descStyle := th.NavigatorSubtitle
	if selected {
		titleStyle = th.NavigatorTitleSelected
		descStyle = th.NavigatorSubtitleSelected
	}
	if !focus {
		titleStyle = titleStyle.Faint(true)
		descStyle = descStyle.Faint(true)
	}

	parts = append(parts, " ", titleStyle.Render(title))
	showTarget := n.Target != "" && !compact
	if n.Kind == KindRequest && n.HasName {
		showTarget = false
	}
	if n.Kind == KindRequest && showTarget {
		parts = append(parts, " ", descStyle.Render(trimPath(n.Target, width/2)))
	}
	if len(n.Badges) > 0 {
		parts = append(parts, " ", renderBadges(n.Badges, th))
	}

	line := strings.Join(parts, "")
	truncated := ansi.Truncate(line, width, "")
	indicator := ""
	if len(truncated) < len(line) {
		indicator = th.NavigatorSubtitle.Render(" +")
		avail := width - lipgloss.Width(indicator)
		if avail < 0 {
			avail = 0
		}
		truncated = ansi.Truncate(truncated, avail, "")
		truncated += indicator
	}
	return lipgloss.NewStyle().Width(width).Render(truncated)
}

func rowIcon(n *Node[any]) string {
	if n == nil {
		return " "
	}
	switch n.Kind {
	case KindWorkflow:
		return " "
	case KindDir:
		if len(n.Children) == 0 {
			return " "
		}
		return caret(n.Expanded)
	case KindFile:
		if filesvc.IsRTSFile(n.Payload.FilePath) {
			return "•"
		}
		return caret(n.Expanded)
	default:
		if len(n.Children) > 0 || n.Count > 0 {
			return caret(n.Expanded)
		}
		return " "
	}
}

func caret(expanded bool) string {
	if expanded {
		return "▾"
	}
	return "▸"
}

func renderMethodBadge(method string, th theme.Theme) string {
	label := strings.ToUpper(strings.TrimSpace(method))
	style := th.NavigatorBadge.Foreground(methodColor(th, label)).Bold(true)
	return style.Render(label)
}

func renderWorkflowBadge(th theme.Theme) string {
	style := th.NavigatorBadge.Foreground(th.MethodColors.POST).Bold(true)
	return style.Render("WF")
}

func renderBadges(badges []string, th theme.Theme) string {
	if len(badges) == 0 {
		return ""
	}

	badgeStyle := th.NavigatorBadge.Padding(0, 0)
	parts := make([]string, 0, len(badges))
	for _, b := range badges {
		label := strings.TrimSpace(b)
		if label == "" {
			continue
		}
		parts = append(parts, badgeStyle.Render(label))
	}

	sep := th.NavigatorSubtitle.Render(", ")
	return strings.Join(parts, sep)
}

func methodColor(th theme.Theme, method string) lipgloss.Color {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET":
		return th.MethodColors.GET
	case "POST":
		return th.MethodColors.POST
	case "PUT":
		return th.MethodColors.PUT
	case "PATCH":
		return th.MethodColors.PATCH
	case "DELETE":
		return th.MethodColors.DELETE
	case "HEAD":
		return th.MethodColors.HEAD
	case "OPTIONS":
		return th.MethodColors.OPTIONS
	case "GRPC":
		return th.MethodColors.GRPC
	case "WS", "WEBSOCKET":
		return th.MethodColors.WS
	default:
		return th.MethodColors.Default
	}
}

func trimPath(val string, limit int) string {
	if limit <= 0 || len(val) <= limit {
		return val
	}
	if limit < 4 {
		return val[:limit]
	}
	return val[:limit-3] + "..."
}
