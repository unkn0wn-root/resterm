package navigator

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	iconNone        = " "
	iconSelected    = ">"
	iconCaretClosed = "▸"
	iconCaretOpen   = "▾"
	iconDirClosed   = "📁"
	iconDirOpen     = "📂"
	iconRTS         = "λ"
	iconEnv         = "⚙"
	iconGraphQL     = "◇"
	iconJSON        = "▣"
	iconJavaScript  = "JS"
)

// ListView renders the navigator list with an optional height constraint.
func ListView(
	m *Model[any],
	th theme.Theme,
	width int,
	height int,
	focus bool,
	appearance theme.Appearance,
) string {
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
		out = append(out, renderRow(row, selected, th, width, focus, m.compact, appearance))
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
	appearance theme.Appearance,
) string {
	n := row.Node
	if n == nil {
		return ""
	}

	titleStyle := th.NavigatorTitle
	descStyle := th.NavigatorSubtitle
	if selected {
		titleStyle = th.NavigatorTitleSelected
		descStyle = th.NavigatorSubtitleSelected
	}
	if !focus && appearance != theme.AppearanceLight {
		titleStyle = titleStyle.Faint(true)
		descStyle = descStyle.Faint(true)
	}

	pad := strings.Repeat("  ", row.Level)
	icon := rowIcon(n, selected)
	if selected {
		icon = titleStyle.Render(icon)
	}

	parts := []string{pad, icon}
	if n.Kind == KindWorkflow {
		parts = append(parts, renderWorkflowBadge(th, selected))
	}
	if n.Method != "" {
		parts = append(parts, renderMethodBadge(n.Method, th, selected))
	}

	title := n.Title
	if n.Kind == KindFile && n.Count > 0 {
		title = fmt.Sprintf("%s (%d)", title, n.Count)
	}

	parts = append(parts, selectedGap(th, selected), titleStyle.Render(title))
	showTarget := n.Target != "" && !compact
	if n.Kind == KindRequest && n.HasName {
		showTarget = false
	}
	if n.Kind == KindRequest && showTarget {
		parts = append(parts, selectedGap(th, selected), descStyle.Render(trimPath(n.Target, width/2)))
	}
	if len(n.Badges) > 0 {
		parts = append(parts, selectedGap(th, selected), renderBadges(n.Badges, th, selected))
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

func rowIcon(n *Node[any], selected bool) string {
	if n == nil {
		return iconNone
	}
	switch n.Kind {
	case KindRequest:
		if selected {
			return iconSelected
		}
		if len(n.Children) > 0 || n.Count > 0 {
			return caret(n.Expanded)
		}
		return iconNone
	case KindWorkflow:
		if selected {
			return iconSelected
		}
		return iconNone
	case KindDir:
		return dirIcon(n.Expanded)
	case KindFile:
		switch fileKind(n) {
		case filesvc.FileKindScript:
			return iconRTS
		case filesvc.FileKindEnv:
			return iconEnv
		case filesvc.FileKindGraphQL:
			return iconGraphQL
		case filesvc.FileKindJSON:
			return iconJSON
		case filesvc.FileKindJavaScript:
			return iconJavaScript
		}
		return caret(n.Expanded)
	default:
		if len(n.Children) > 0 || n.Count > 0 {
			return caret(n.Expanded)
		}
		return iconNone
	}
}

func fileKind(n *Node[any]) filesvc.FileKind {
	if entry, ok := n.Payload.Data.(filesvc.FileEntry); ok {
		return entry.Kind
	}
	if kind, ok := filesvc.ClassifyWorkspacePath(n.Payload.FilePath); ok {
		return kind
	}
	return filesvc.FileKindRequest
}

func caret(expanded bool) string {
	if expanded {
		return iconCaretOpen
	}
	return iconCaretClosed
}

func dirIcon(expanded bool) string {
	if expanded {
		return iconDirOpen
	}
	return iconDirClosed
}

func renderMethodBadge(method string, th theme.Theme, selected bool) string {
	label := strings.ToUpper(strings.TrimSpace(method))
	style := th.NavigatorBadge.Foreground(methodColor(th, label)).Bold(true)
	if selected {
		style = withSelectedBackground(style, th)
	}
	return style.Render(label)
}

func renderWorkflowBadge(th theme.Theme, selected bool) string {
	style := th.NavigatorBadge.Foreground(th.MethodColors.POST).Bold(true)
	if selected {
		style = withSelectedBackground(style, th)
	}
	return style.Render("WF")
}

func renderBadges(badges []string, th theme.Theme, selected bool) string {
	if len(badges) == 0 {
		return ""
	}

	badgeStyle := th.NavigatorBadge.Padding(0, 0)
	if selected {
		badgeStyle = withSelectedBackground(badgeStyle, th)
	}
	parts := make([]string, 0, len(badges))
	for _, b := range badges {
		label := strings.TrimSpace(b)
		if label == "" {
			continue
		}
		parts = append(parts, badgeStyle.Render(label))
	}

	sepStyle := th.NavigatorSubtitle
	if selected {
		sepStyle = withSelectedBackground(sepStyle, th)
	}
	sep := sepStyle.Render(", ")
	return strings.Join(parts, sep)
}

func selectedGap(th theme.Theme, selected bool) string {
	if !selected {
		return " "
	}
	return withSelectedBackground(lipgloss.NewStyle(), th).Render(" ")
}

func withSelectedBackground(style lipgloss.Style, th theme.Theme) lipgloss.Style {
	if bg := th.NavigatorTitleSelected.GetBackground(); theme.ColorDefined(bg) {
		return style.Background(bg)
	}
	return style
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
