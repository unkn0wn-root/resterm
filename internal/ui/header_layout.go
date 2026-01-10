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

func styleRight(text string, st lipgloss.Style, max int) (string, int) {
	if max <= 0 {
		return "", 0
	}
	t := strings.TrimSpace(text)
	if t == "" {
		return "", 0
	}
	t = truncateToWidth(t, maxInt(1, max-st.GetHorizontalFrameSize()))
	if strings.TrimSpace(t) == "" {
		return "", 0
	}
	s := st.Render(t)
	return s, lipgloss.Width(s)
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
		rs, _ := styleRight(right, rightStyle, width)
		if rs == "" {
			return ""
		}
		return trimHeaderLine(rs, width)
	}
	sepW := lipgloss.Width(sep)
	sw := headerSegmentWidths(left)
	leftLine := func() string {
		return fitHeaderLine(left, sw, sep, sepW, width, width)
	}
	mr := width - headerGap - sw[0]
	if mr < 1 {
		return leftLine()
	}
	rs, rw := styleRight(right, rightStyle, mr)
	if rs == "" {
		return leftLine()
	}
	ml := width - headerGap - rw
	line, lw := fitHeaderSegments(left, sw, sep, sepW, ml)
	pad := width - lw - rw
	if pad < headerGap {
		pad = headerGap
	}
	line = lipgloss.JoinHorizontal(
		lipgloss.Center,
		line,
		strings.Repeat(" ", pad),
		rs,
	)
	return trimHeaderLine(line, width)
}

func fitHeaderLine(segs []string, sw []int, sep string, sepW int, max int, width int) string {
	line, _ := fitHeaderSegments(segs, sw, sep, sepW, max)
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
