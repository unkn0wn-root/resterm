package navigator

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

// View renders the navigator list and detail well without height constraints.
func View(m *Model[any], th theme.Theme, width int, focus bool) (string, string) {
	return ListView(m, th, width, 0, focus), DetailView(m, th, width)
}

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

// DetailView renders the detail section for the selected node.
func DetailView(m *Model[any], th theme.Theme, width int) string {
	if m == nil {
		return ""
	}
	return renderDetail(m.Selected(), th, width)
}

func renderRow(row Flat[any], selected bool, th theme.Theme, width int, focus bool, compact bool) string {
	n := row.Node
	if n == nil {
		return ""
	}
	pad := strings.Repeat("  ", row.Level)
	icon := " "
	if n.Kind != KindWorkflow && (len(n.Children) > 0 || n.Kind == KindFile || n.Count > 0) {
		if n.Expanded {
			icon = "▾"
		} else {
			icon = "▸"
		}
	}
	parts := []string{pad, icon}
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
	if len(n.Tags) > 0 && !compact {
		parts = append(parts, " ", renderTags(n.Tags, th))
	}
	line := strings.Join(parts, "")
	truncated := ansi.Truncate(line, width, "")
	indicator := ""
	if len(truncated) < len(line) {
		indicator = th.NavigatorDetailDim.Render(" +")
		avail := width - lipgloss.Width(indicator)
		if avail < 0 {
			avail = 0
		}
		truncated = ansi.Truncate(truncated, avail, "")
		truncated += indicator
	}
	return lipgloss.NewStyle().Width(width).Render(truncated)
}

func renderDetail(n *Node[any], th theme.Theme, width int) string {
	if n == nil {
		return ""
	}
	softWrap := func(s string, style lipgloss.Style) []string {
		if s == "" {
			return nil
		}
		raw := style.Render(s)
		wrapped := ansi.Wrap(raw, width, "")
		return strings.Split(wrapped, "\n")
	}

	var lines []string

	header := n.Title
	if n.Method != "" {
		header = fmt.Sprintf("%s %s", strings.ToUpper(n.Method), header)
	}
	lines = append(lines, ansi.Truncate(th.NavigatorDetailTitle.Render(header), width, ""))

	if n.Target != "" {
		lines = append(lines, softWrap(n.Target, th.NavigatorDetailValue)...)
	}
	if n.Desc != "" {
		lines = append(lines, softWrap(n.Desc, th.NavigatorDetailDim)...)
	}
	if len(n.Badges) > 0 {
		lines = append(lines, renderBadges(n.Badges, th))
	}
	if len(n.Tags) > 0 {
		lines = append(lines, renderTags(n.Tags, th))
	}

	return lipgloss.NewStyle().Width(width).Render(strings.Join(lines, "\n"))
}

func renderMethodBadge(method string, th theme.Theme) string {
	label := strings.ToUpper(strings.TrimSpace(method))
	style := th.NavigatorBadge.Background(methodColor(th, label)).Foreground(lipgloss.Color("#0f111a")).Bold(true)
	return style.Render(label)
}

func renderWorkflowBadge(th theme.Theme) string {
	style := th.NavigatorBadge.Background(th.MethodColors.POST).Foreground(lipgloss.Color("#0f111a")).Bold(true)
	return style.Render("WF")
}

func renderTags(tags []string, th theme.Theme) string {
	if len(tags) == 0 {
		return ""
	}
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			clean = append(clean, "#"+t)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return th.NavigatorTag.Render(strings.Join(clean, " "))
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
	sep := th.NavigatorDetailDim.Render(", ")
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
