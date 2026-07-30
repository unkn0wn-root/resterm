package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const headerGap = 1

// Header segments are dropped lowest priority first, so a narrow terminal keeps
// the environment and the active request rather than the decoration around them.
const (
	headerPriorityRequests = iota
	headerPriorityWorkspace
	headerPriorityBrand
	headerPriorityTests
	headerPriorityActive
	headerPriorityEnv
)

// headerValueTight is the width a value shrinks to once its full rendering no
// longer fits. Below that a value stops being readable, so the segment is
// dropped instead.
const headerValueTight = 12

const headerValueFloor = 8

// headerSegment is one rendered header cell. text holds the same cell from
// widest to narrowest, and lossy is the first rendering that drops characters:
// everything before it is an alternative wording that says the same thing in
// less room. The fitter shrinks cells one step at a time, spending every lossless
// rendering before it truncates anything and truncating before it drops a cell.
type headerSegment struct {
	text     []string
	lossy    int
	priority int
}

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

// headerValueCap stops one long value from monopolising the header: no segment
// may claim more than a quarter of it.
func headerValueCap(width int) int {
	return max(width/4, headerValueFloor)
}

// headerCellVariants renders a cell's values widest first, dropping any that
// would not actually save room. Every value is capped to limit, and a tighter
// rendering of the shortest one is appended so a long value has somewhere to
// shrink to before whole segments start disappearing. lossy reports where
// truncation starts, which is what lets the fitter prefer a caller's own shorter
// wording over cutting characters off anything.
func headerCellVariants(values []string, limit int) (text []string, lossy int) {
	text = make([]string, 0, len(values)+1)
	lossy = -1
	add := func(value, rendered string) {
		if len(text) > 0 && lipgloss.Width(rendered) >= lipgloss.Width(text[len(text)-1]) {
			return
		}
		if lossy < 0 && rendered != value {
			lossy = len(text)
		}
		text = append(text, rendered)
	}

	for _, value := range values {
		add(value, truncateToWidth(value, limit))
	}
	last := values[len(values)-1]
	add(last, truncateToWidth(last, min(limit, headerValueTight)))

	if lossy < 0 {
		lossy = len(text)
	}
	return text, lossy
}

func styleRight(text string, st lipgloss.Style, maxWidth int) (string, int) {
	if maxWidth <= 0 || text == "" {
		return "", 0
	}
	w := maxWidth - st.GetHorizontalFrameSize()
	if w <= 0 {
		return "", 0
	}
	text = ansi.Truncate(text, w, "…")
	s := st.Render(text)
	return s, lipgloss.Width(s)
}

func buildHeaderLine(
	left []headerSegment,
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
	leftOnly := func() string {
		line, _ := fitHeaderSegments(left, sep, sepW, width)
		return trimHeaderLine(line, width)
	}

	mr := width - headerGap - minHeaderWidth(left)
	if mr < 1 {
		return leftOnly()
	}
	rs, rw := styleRight(right, rightStyle, mr)
	if rs == "" {
		return leftOnly()
	}

	line, lw := fitHeaderSegments(left, sep, sepW, width-headerGap-rw)
	pad := max(width-lw-rw, headerGap)
	line = lipgloss.JoinHorizontal(
		lipgloss.Center,
		line,
		strings.Repeat(" ", pad),
		rs,
	)
	return trimHeaderLine(line, width)
}

// fitHeaderSegments packs segs into limit. It shrinks one cell at a time until
// nothing can shrink further, and only then drops the least important cells, so
// a wordy segment never costs a neighbour and never shortens one that had room
// to spare. Survivors keep their original left-to-right order, and the most
// important one always survives.
func fitHeaderSegments(segs []headerSegment, sep string, sepW, limit int) (string, int) {
	if len(segs) == 0 {
		return "", 0
	}

	widths := make([][]int, len(segs))
	for i, seg := range segs {
		widths[i] = headerSegmentWidths(seg.text)
	}

	keep := make([]int, len(segs))
	for i := range keep {
		keep[i] = i
	}
	levels := make([]int, len(segs))

	for {
		if w := headerLineWidth(widths, keep, levels, sepW); w <= limit {
			return joinHeaderSegments(segs, keep, levels, sep), w
		}
		next, ok := shrinkHeaderSegment(segs, keep, levels, widths)
		if !ok {
			break
		}
		levels[next]++
	}

	for len(keep) > 1 {
		keep = dropHeaderSegment(segs, keep)
		if w := headerLineWidth(widths, keep, levels, sepW); w <= limit {
			return joinHeaderSegments(segs, keep, levels, sep), w
		}
	}
	return joinHeaderSegments(segs, keep, levels, sep), headerLineWidth(widths, keep, levels, sepW)
}

// shrinkHeaderSegment picks the next cell to shorten: a lossless rendering ahead
// of any truncation, and the largest width saving among equals, so each step
// gives up as little as it can.
func shrinkHeaderSegment(segs []headerSegment, keep, levels []int, widths [][]int) (int, bool) {
	best, bestCost, bestSave := -1, 0, 0
	for _, i := range keep {
		next := levels[i] + 1
		if next >= len(segs[i].text) {
			continue
		}
		cost := 0
		if next >= segs[i].lossy {
			cost = 1
		}
		save := widths[i][levels[i]] - widths[i][next]
		if best < 0 || cost < bestCost || (cost == bestCost && save > bestSave) {
			best, bestCost, bestSave = i, cost, save
		}
	}
	return best, best >= 0
}

// minHeaderWidth is the narrowest the left side can get: the segment
// fitHeaderSegments always keeps, at its shortest rendering.
func minHeaderWidth(segs []headerSegment) int {
	width, best := 0, -1
	for _, seg := range segs {
		if seg.priority > best && len(seg.text) > 0 {
			best = seg.priority
			width = lipgloss.Width(seg.text[len(seg.text)-1])
		}
	}
	return width
}

// dropHeaderSegment removes the least important kept segment, taking the
// rightmost when priorities tie.
func dropHeaderSegment(segs []headerSegment, keep []int) []int {
	drop := 0
	for n, i := range keep {
		if segs[i].priority <= segs[keep[drop]].priority {
			drop = n
		}
	}
	return slices.Delete(keep, drop, drop+1)
}

func headerSegmentWidths(segs []string) []int {
	out := make([]int, len(segs))
	for i, seg := range segs {
		out[i] = lipgloss.Width(seg)
	}
	return out
}

func headerLineWidth(widths [][]int, keep, levels []int, sepW int) int {
	total := 0
	for n, i := range keep {
		if n > 0 {
			total += sepW
		}
		total += widths[i][levels[i]]
	}
	return total
}

func joinHeaderSegments(segs []headerSegment, keep, levels []int, sep string) string {
	parts := make([]string, 0, 2*len(keep))
	for n, i := range keep {
		if n > 0 && sep != "" {
			parts = append(parts, sep)
		}
		parts = append(parts, segs[i].text[levels[i]])
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
