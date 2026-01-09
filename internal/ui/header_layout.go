package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const headerGap = 1

func headerContentWidth(total int, style lipgloss.Style) int {
	if total <= 0 {
		return 0
	}
	frame := style.GetHorizontalFrameSize()
	width := total - frame
	if width < 1 {
		return 1
	}
	return width
}

func buildHeaderLine(
	left []string,
	sep string,
	right string,
	rightStyle lipgloss.Style,
	width int,
) string {
	if width <= 0 {
		return ""
	}
	if len(left) == 0 {
		right = strings.TrimSpace(right)
		if right == "" {
			return ""
		}
		right = truncateToWidth(right, maxInt(1, width-rightStyle.GetHorizontalFrameSize()))
		if strings.TrimSpace(right) == "" {
			return ""
		}
		return trimHeaderLine(rightStyle.Render(right), width)
	}
	sepW := lipgloss.Width(sep)
	lw := headerSegmentWidths(left)
	right = strings.TrimSpace(right)
	if right == "" {
		line, _ := fitHeaderSegments(left, lw, sep, sepW, width)
		return trimHeaderLine(line, width)
	}

	brandW := lw[0]
	maxRight := width - headerGap - brandW
	if maxRight < 1 {
		line, _ := fitHeaderSegments(left, lw, sep, sepW, width)
		return trimHeaderLine(line, width)
	}
	rightText := truncateToWidth(
		right,
		maxInt(1, maxRight-rightStyle.GetHorizontalFrameSize()),
	)
	if strings.TrimSpace(rightText) == "" {
		line, _ := fitHeaderSegments(left, lw, sep, sepW, width)
		return trimHeaderLine(line, width)
	}
	rightStyled := rightStyle.Render(rightText)
	rightW := lipgloss.Width(rightStyled)
	maxLeft := width - headerGap - rightW
	line, leftW := fitHeaderSegments(left, lw, sep, sepW, maxLeft)
	pad := width - leftW - rightW
	if pad < headerGap {
		pad = headerGap
	}
	line = lipgloss.JoinHorizontal(
		lipgloss.Center,
		line,
		strings.Repeat(" ", pad),
		rightStyled,
	)
	return trimHeaderLine(line, width)
}

func headerSegmentWidths(segs []string) []int {
	out := make([]int, len(segs))
	for i, seg := range segs {
		out[i] = lipgloss.Width(seg)
	}
	return out
}

func fitHeaderSegments(
	segs []string,
	widths []int,
	sep string,
	sepW int,
	max int,
) (string, int) {
	if len(segs) == 0 {
		return "", 0
	}
	if max <= 0 {
		return segs[0], widths[0]
	}
	total := widths[0]
	count := 1
	for i := 1; i < len(segs); i++ {
		next := total + sepW + widths[i]
		if next > max {
			break
		}
		total = next
		count = i + 1
	}
	if count == 1 {
		return segs[0], widths[0]
	}
	return joinHeaderSegments(segs[:count], sep), total
}

func joinHeaderSegments(segs []string, sep string) string {
	if len(segs) == 0 {
		return ""
	}
	if len(segs) == 1 {
		return segs[0]
	}
	parts := make([]string, 0, len(segs)*2-1)
	for i, seg := range segs {
		if i > 0 && sep != "" {
			parts = append(parts, sep)
		}
		parts = append(parts, seg)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func trimHeaderLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(line) <= width {
		return line
	}
	return ansi.Truncate(line, width, "")
}
