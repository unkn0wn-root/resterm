package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/ui/hint"
)

const metadataHintMaxWidth = 60
const metadataHintMaxRows = 15
const metadataHintPreviewMaxWidth = 52
const metadataHintPreviewMaxHeight = 12

type metadataHintPopupLayout struct {
	x     int
	y     int
	width int
	limit int
}

type hintBoxMetrics struct {
	frameW int
	frameH int
	hPad   int
}

func (m Model) editorHintBoxMetrics() hintBoxMetrics {
	s := m.theme.EditorHintBox
	return hintBoxMetrics{
		frameW: s.GetHorizontalPadding() + s.GetHorizontalBorderSize(),
		frameH: s.GetVerticalPadding() + s.GetVerticalBorderSize(),
		hPad:   s.GetHorizontalPadding(),
	}
}

func (m Model) renderMetadataHintPopup(content string) string {
	st := m.editor.metadataHints
	if !st.active || len(st.filtered) == 0 {
		return content
	}

	w := lipgloss.Width(content)
	h := lipgloss.Height(content)
	if w <= 0 || h <= 0 {
		return content
	}

	x, y := m.editor.ViewportCursor()
	layout, ok := m.metadataHintPopupLayout(w, h, x, y, len(st.filtered))
	if !ok {
		return content
	}

	items, selection, ok := st.display(layout.limit)
	if !ok || len(items) == 0 {
		return content
	}

	lines := m.buildMetadataHintPopup(
		items,
		selection,
		layout.width,
		st.popupLabelW,
		st.popupSummaryW,
	)
	if len(lines) == 0 {
		return content
	}

	boxW := 0
	for _, line := range lines {
		if ww := visibleWidth(line); ww > boxW {
			boxW = ww
		}
	}
	if boxW <= 0 {
		return content
	}

	maxX := maxInt(w-boxW, 0)
	if layout.x > maxX {
		layout.x = maxX
	}
	out := overlayHintPopup(content, lines, layout.x, layout.y, w, h)
	if !st.preview || selection < 0 || selection >= len(items) {
		return out
	}

	listBox := hintOverlayBox{x: layout.x, y: layout.y, w: boxW, h: len(lines)}
	preview, ok := m.metadataHintPreviewBox(items[selection], listBox, w, h)
	if !ok {
		return out
	}
	return overlayHintPopup(out, preview.lines, preview.x, preview.y, w, h)
}

func (m Model) metadataHintPopupLayout(
	contentW, contentH, cursorX, cursorY, count int,
) (metadataHintPopupLayout, bool) {
	if contentW <= 0 || contentH <= 0 || count <= 0 {
		return metadataHintPopupLayout{}, false
	}

	box := m.editorHintBoxMetrics()
	maxW := minInt(contentW, metadataHintMaxWidth)
	if maxW <= box.frameW {
		return metadataHintPopupLayout{}, false
	}

	below := contentH - cursorY - 1
	above := cursorY
	belowItems := maxInt(below-box.frameH, 0)
	aboveItems := maxInt(above-box.frameH, 0)
	if belowItems == 0 && aboveItems == 0 {
		return metadataHintPopupLayout{}, false
	}

	targetItems := minInt(count, metadataHintMaxRows)
	limit := 0
	y := cursorY + 1
	switch {
	case belowItems >= targetItems:
		limit = targetItems
	case belowItems == 0 || aboveItems > belowItems:
		limit = minInt(targetItems, aboveItems)
		y = cursorY - (limit + box.frameH)
	default:
		limit = minInt(targetItems, belowItems)
	}
	if limit < 1 {
		return metadataHintPopupLayout{}, false
	}

	x := cursorX
	if x < 0 {
		x = 0
	}
	if x > contentW-maxW {
		x = maxInt(contentW-maxW, 0)
	}

	return metadataHintPopupLayout{
		x:     x,
		y:     maxInt(y, 0),
		width: maxW,
		limit: limit,
	}, true
}

func (m Model) buildMetadataHintPopup(
	items []hint.Hint,
	selection int,
	maxOuterW int,
	prefLabelW int,
	prefSummaryW int,
) []string {
	if len(items) == 0 || maxOuterW <= 0 {
		return nil
	}

	mx := m.editorHintBoxMetrics()
	if maxOuterW <= mx.frameW {
		return nil
	}

	maxTextW := maxOuterW - mx.frameW
	if maxTextW < 1 {
		return nil
	}

	labelW, summaryW := metadataHintPopupColumns(prefLabelW, prefSummaryW, maxTextW)
	lines := m.metadataHintPopupLines(items, selection, labelW, summaryW)
	textW := labelW
	if summaryW > 0 {
		textW += 1 + summaryW
	}

	out := m.theme.EditorHintBox.
		Width(textW + mx.hPad).
		Render(strings.Join(lines, "\n"))
	return strings.Split(out, "\n")
}

func (m Model) metadataHintPopupLines(
	items []hint.Hint,
	selection int,
	labelW, summaryW int,
) []string {
	lines := make([]string, len(items))
	for i, item := range items {
		labelStyle := m.theme.EditorHintItem
		if i == selection {
			labelStyle = m.theme.EditorHintSelected
		}

		labelText := ansi.Truncate(item.Label, labelW, "")
		labelText = padVisibleRight(labelText, labelW)
		label := labelStyle.Render(labelText)
		if summaryW > 0 && item.Summary != "" {
			summaryText := ansi.Truncate(item.Summary, summaryW, "")
			summary := m.theme.EditorHintAnnotation.Render(summaryText)
			lines[i] = lipgloss.JoinHorizontal(lipgloss.Top, label, " ", summary)
			continue
		}
		lines[i] = label
	}
	return lines
}

func metadataHintPopupColumns(prefLabelW, prefSummaryW, maxTextW int) (int, int) {
	if maxTextW < 1 {
		return 0, 0
	}
	labelW := prefLabelW
	if labelW < 1 {
		labelW = 1
	}
	if labelW > maxTextW {
		labelW = maxTextW
	}
	if prefSummaryW < 1 || labelW >= maxTextW {
		return labelW, 0
	}

	avail := maxTextW - labelW - 1
	if avail < 1 {
		return labelW, 0
	}
	summaryW := prefSummaryW
	if summaryW > avail {
		summaryW = avail
	}
	return labelW, summaryW
}

func metadataHintPopupWidth(lines []string) int {
	w := 0
	for _, line := range lines {
		if ww := visibleWidth(line); ww > w {
			w = ww
		}
	}
	return w
}

func overlayHintPopup(
	base string,
	block []string,
	x, y, width, height int,
) string {
	if len(block) == 0 || width <= 0 || height <= 0 {
		return base
	}

	rows := strings.Split(base, "\n")
	if len(rows) == 0 {
		return base
	}

	for i, line := range block {
		row := y + i
		if row < 0 || row >= len(rows) || row >= height {
			continue
		}
		if x >= width {
			continue
		}

		baseLine := rows[row]
		if ww := visibleWidth(baseLine); ww < width {
			baseLine += strings.Repeat(" ", width-ww)
		}

		overlay := line
		if ww := visibleWidth(overlay); ww > width-x {
			overlay = ansi.Cut(overlay, 0, width-x)
		}

		ow := visibleWidth(overlay)
		prefix := ansi.Cut(baseLine, 0, x)
		suffix := ""
		if x+ow < width {
			suffix = ansi.Cut(baseLine, x+ow, width)
		}

		// Replace the covered span so the popup behaves like a floating block.
		merged := prefix + overlay + suffix
		if ww := visibleWidth(merged); ww < width {
			merged += strings.Repeat(" ", width-ww)
		}
		rows[row] = merged
	}

	return strings.Join(rows, "\n")
}

func padVisibleRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := visibleWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

type hintOverlayBox struct {
	x     int
	y     int
	w     int
	h     int
	lines []string
}

func (m Model) metadataHintPreviewBox(
	item hint.Hint,
	list hintOverlayBox,
	contentW, contentH int,
) (hintOverlayBox, bool) {
	gap := 1
	cands := []struct {
		x    int
		y    int
		maxW int
		maxH int
		side bool
	}{
		{
			x:    list.x + list.w + gap,
			y:    list.y,
			maxW: contentW - (list.x + list.w + gap),
			maxH: contentH,
			side: true,
		},
		{
			x:    0,
			y:    list.y,
			maxW: list.x - gap,
			maxH: contentH,
			side: true,
		},
		{
			x:    list.x,
			y:    list.y + list.h + gap,
			maxW: contentW,
			maxH: contentH - (list.y + list.h + gap),
		},
		{
			x:    list.x,
			y:    0,
			maxW: contentW,
			maxH: list.y - gap,
		},
	}

	for i, cand := range cands {
		if cand.maxW < 18 || cand.maxH < 5 {
			continue
		}
		maxW := minInt(cand.maxW, metadataHintPreviewMaxWidth)
		maxH := minInt(cand.maxH, metadataHintPreviewMaxHeight)
		lines := m.buildMetadataHintPreview(item, maxW, maxH)
		if len(lines) == 0 {
			continue
		}

		box := hintOverlayBox{
			x:     cand.x,
			y:     cand.y,
			w:     metadataHintPopupWidth(lines),
			h:     len(lines),
			lines: lines,
		}
		if box.w <= 0 || box.h <= 0 {
			continue
		}
		if cand.side {
			box.y = clamp(box.y, 0, maxInt(contentH-box.h, 0))
			if i == 1 {
				box.x = list.x - gap - box.w
			}
		} else {
			box.x = clamp(box.x, 0, maxInt(contentW-box.w, 0))
			if i == 3 {
				box.y = list.y - gap - box.h
			}
		}
		if box.x < 0 || box.y < 0 || box.x >= contentW || box.y >= contentH {
			continue
		}
		return box, true
	}
	return hintOverlayBox{}, false
}

func (m Model) buildMetadataHintPreview(
	item hint.Hint,
	maxOuterW, maxOuterH int,
) []string {
	if maxOuterW <= 0 || maxOuterH <= 0 {
		return nil
	}

	mx := m.editorHintBoxMetrics()
	textW := maxOuterW - mx.frameW
	textH := maxOuterH - mx.frameH
	if textW < 12 || textH < 3 {
		return nil
	}

	titleStyle := m.theme.EditorHintSelected
	bodyStyle := m.theme.EditorHintItem
	metaStyle := m.theme.EditorHintAnnotation.Bold(true)
	helpStyle := m.theme.EditorHintAnnotation

	var lines []string
	addLine := func(line string) bool {
		if len(lines) >= textH {
			return false
		}
		lines = append(lines, line)
		return true
	}
	addBlank := func() bool {
		if len(lines) == 0 || lines[len(lines)-1] == "" {
			return true
		}
		return addLine("")
	}
	addWrapped := func(style lipgloss.Style, text string) bool {
		if strings.TrimSpace(text) == "" {
			return true
		}
		for _, line := range strings.Split(wrapToWidth(text, textW), "\n") {
			line = ansi.Truncate(line, textW, "")
			if !addLine(style.Render(line)) {
				return false
			}
		}
		return true
	}

	title := ansi.Truncate(item.Label, textW, "")
	if !addLine(titleStyle.Render(title)) {
		return nil
	}
	if !addWrapped(bodyStyle, item.Summary) {
		goto render
	}

	if len(item.Aliases) > 0 {
		if addBlank() && addLine(metaStyle.Render("Aliases")) {
			if !addWrapped(helpStyle, strings.Join(item.Aliases, ", ")) {
				goto render
			}
		}
	}

	if item.Insert != "" {
		if addBlank() && addLine(metaStyle.Render("Insert")) {
			if !addWrapped(bodyStyle, item.Insert) {
				goto render
			}
		}
	}

	if addBlank() {
		_ = addWrapped(helpStyle, "Enter insert | Left/Esc close")
	}

render:
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, textW, "")
	}
	width := metadataHintPopupWidth(lines)
	if width < 1 {
		return nil
	}
	boxLines := m.theme.EditorHintBox.
		Width(width + mx.hPad).
		Render(strings.Join(lines, "\n"))
	return strings.Split(boxLines, "\n")
}
