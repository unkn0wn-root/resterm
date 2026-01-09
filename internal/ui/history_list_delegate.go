package ui

import (
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const historyLineEllipsis = "..."

type historyDelegate struct {
	list.DefaultDelegate
	th       theme.Theme
	selected map[string]struct{}
}

type historyLineFrame struct {
	padL int
	padR int

	borderL      string
	borderR      string
	borderLStyle lipgloss.Style
	borderRStyle lipgloss.Style

	width int
}

func (d historyDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	hi, ok := item.(historyItem)
	if !ok {
		return
	}
	width := m.Width()
	if width <= 0 {
		return
	}

	titleStyle, descStyle := historyItemStyles(d.Styles, m, index)
	titleSeg := historySegmentStyle(titleStyle)
	descSeg := historySegmentStyle(descStyle)

	titleFrame := newHistoryLineFrame(titleStyle, width)
	descFrame := newHistoryLineFrame(descStyle, width)

	marker := historySelectionMarker(d.selected, hi.entry.ID)
	title := renderHistoryTitleLine(hi, titleSeg, titleFrame, marker)
	_, _ = io.WriteString(w, title)

	if !d.ShowDescription {
		return
	}

	lines := historyDescriptionLines(hi.entry)
	maxLines := d.Height() - 1
	if maxLines < 0 {
		maxLines = 0
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	baseLine := historyBaseLine(hi.entry)
	for _, line := range lines {
		content := ""
		if line == baseLine {
			content = renderHistoryMethodLine(hi.entry, descSeg, d.th)
		} else {
			content = descSeg.Render(line)
		}
		content = historyTrimLine(content, descFrame.width)
		row := descFrame.render(descSeg, content)
		_, _ = io.WriteString(w, "\n"+row)
	}
}

func historyItemStyles(
	s list.DefaultItemStyles,
	m list.Model,
	index int,
) (lipgloss.Style, lipgloss.Style) {
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""
	selected := index == m.Index() && m.FilterState() != list.Filtering
	switch {
	case emptyFilter:
		return s.DimmedTitle, s.DimmedDesc
	case selected:
		return s.SelectedTitle, s.SelectedDesc
	default:
		return s.NormalTitle, s.NormalDesc
	}
}

func historySegmentStyle(base lipgloss.Style) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(base.GetForeground()).
		Background(base.GetBackground()).
		Bold(base.GetBold()).
		Italic(base.GetItalic()).
		Underline(base.GetUnderline()).
		Faint(base.GetFaint()).
		Strikethrough(base.GetStrikethrough()).
		Reverse(base.GetReverse())
}

func newHistoryLineFrame(style lipgloss.Style, width int) historyLineFrame {
	frame := historyLineFrame{
		padL: style.GetPaddingLeft(),
		padR: style.GetPaddingRight(),
	}
	frame.width = maxInt(
		width-frame.padL-frame.padR-style.GetBorderLeftSize()-style.GetBorderRightSize(),
		0,
	)
	if style.GetBorderLeft() {
		border := style.GetBorderStyle()
		frame.borderL = border.Left
		frame.borderLStyle = historyBorderStyle(
			style.GetBorderLeftForeground(),
			style.GetBorderLeftBackground(),
		)
	}
	if style.GetBorderRight() {
		border := style.GetBorderStyle()
		frame.borderR = border.Right
		frame.borderRStyle = historyBorderStyle(
			style.GetBorderRightForeground(),
			style.GetBorderRightBackground(),
		)
	}
	return frame
}

func historyBorderStyle(fg, bg lipgloss.TerminalColor) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(fg).Background(bg)
}

func (f historyLineFrame) render(segStyle lipgloss.Style, content string) string {
	prefix := ""
	if f.borderL != "" {
		prefix += f.borderLStyle.Render(f.borderL)
	}
	if f.padL > 0 {
		prefix += segStyle.Render(strings.Repeat(" ", f.padL))
	}
	suffix := ""
	if f.padR > 0 {
		suffix = segStyle.Render(strings.Repeat(" ", f.padR))
	}
	if f.borderR != "" {
		suffix += f.borderRStyle.Render(f.borderR)
	}
	return prefix + content + suffix
}

func renderHistoryTitleLine(
	item historyItem,
	base lipgloss.Style,
	frame historyLineFrame,
	marker string,
) string {
	parts := buildHistoryTitleParts(item)
	content := ""
	if parts.line != "" {
		content = base.Render(marker) + base.Render(parts.line)
	} else {
		entry := item.entry
		codeStyle := historyStatusStyle(base, entry.StatusCode, entry.Status)
		content = base.Render(marker+parts.prefix) +
			codeStyle.Render(parts.code) +
			base.Render(parts.suffix)
	}
	content = historyTrimLine(content, frame.width)
	return frame.render(base, content)
}

func historySelectionMarker(selected map[string]struct{}, id string) string {
	if len(selected) == 0 {
		return ""
	}
	if id == "" {
		return "[ ] "
	}
	if _, ok := selected[id]; ok {
		return "[x] "
	}
	return "[ ] "
}

func renderHistoryMethodLine(
	entry history.Entry,
	base lipgloss.Style,
	th theme.Theme,
) string {
	baseLine := historyBaseLine(entry)
	method := entry.Method
	if method == "" || !strings.HasPrefix(baseLine, method) {
		return base.Render(baseLine)
	}
	rest := strings.TrimPrefix(baseLine, method)
	methodStyle := base.Foreground(methodColor(th, method))
	return methodStyle.Render(method) + base.Render(rest)
}

func historyStatusStyle(base lipgloss.Style, code int, status string) lipgloss.Style {
	switch {
	case code >= 400:
		return base.Foreground(statsWarnStyle.GetForeground())
	case code >= 300:
		return base.Foreground(statsCautionStyle.GetForeground())
	case code > 0:
		return base.Foreground(statsSuccessStyle.GetForeground())
	case strings.EqualFold(strings.TrimSpace(status), "ok"):
		return base.Foreground(statsSuccessStyle.GetForeground())
	default:
		return base
	}
}

func historyTrimLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(line, width, historyLineEllipsis)
}
