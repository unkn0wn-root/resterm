package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
)

const (
	workflowStatsSplitMinWidth = 96
	workflowStatsSplitMinH     = 12
	workflowStatsGap           = " │ "
)

type workflowStatsView struct {
	label        string
	name         string
	started      time.Time
	ended        time.Time
	totalSteps   int
	entries      []workflowStatsEntry
	selected     int
	detailOffset int
	detailFocus  bool
}

type workflowStatsEntry struct {
	index  int
	result workflowStepResult
}

type workflowStatsRender struct {
	content   string
	lineCount int
}

func buildWorkflowStatsEntries(state *workflowState) []workflowStatsEntry {
	if state == nil {
		return nil
	}
	total := len(state.steps)
	if total == 0 {
		total = len(state.results)
	}
	entries := make([]workflowStatsEntry, 0, total)
	for i, res := range state.results {
		entries = append(entries, workflowStatsEntry{index: i, result: res})
	}
	if !state.canceled || len(entries) >= total || len(state.steps) == 0 {
		return entries
	}

	for i := len(entries); i < total && i < len(state.steps); i++ {
		step := state.steps[i].step
		entries = append(entries, workflowStatsEntry{
			index: i,
			result: workflowStepResult{
				Step:     step,
				Canceled: true,
			},
		})
	}
	return entries
}

func newWorkflowStatsView(state *workflowState) *workflowStatsView {
	if state == nil {
		return &workflowStatsView{selected: -1}
	}

	entries := buildWorkflowStatsEntries(state)
	selected := workflowDefaultSelection(entries)

	return &workflowStatsView{
		label:      state.runLabel(),
		name:       state.runSubject(),
		started:    state.start,
		ended:      state.end,
		totalSteps: len(state.steps),
		entries:    entries,
		selected:   selected,
	}
}

func workflowDefaultSelection(entries []workflowStatsEntry) int {
	if len(entries) == 0 {
		return -1
	}
	for i, entry := range entries {
		if workflowEntryNeedsAttention(entry.result) {
			return i
		}
	}
	return 0
}

func workflowEntryNeedsAttention(res workflowStepResult) bool {
	if res.Canceled {
		return true
	}
	if res.Skipped {
		return false
	}
	return !res.Success
}

func (v *workflowStatsView) hasEntries() bool {
	return len(v.entries) > 0
}

func (v *workflowStatsView) move(delta int) bool {
	if !v.hasEntries() {
		return false
	}
	next := max(v.selected+delta, 0)
	if next >= len(v.entries) {
		next = len(v.entries) - 1
	}
	if next == v.selected {
		return false
	}
	v.selected = next
	v.detailOffset = 0
	return true
}

func (v *workflowStatsView) selectEdge(top bool) bool {
	if !v.hasEntries() {
		return false
	}
	next := 0
	if !top {
		next = len(v.entries) - 1
	}
	if next == v.selected {
		return false
	}
	v.selected = next
	v.detailOffset = 0
	return true
}

func (v *workflowStatsView) toggle() bool {
	if !v.hasEntries() {
		return false
	}
	v.detailFocus = !v.detailFocus
	return true
}

func (v *workflowStatsView) blurDetail() bool {
	if v == nil || !v.detailFocus {
		return false
	}
	v.detailFocus = false
	return true
}

func (v *workflowStatsView) scrollDetail(width, height, delta int, r responseRenderer) bool {
	if v == nil || delta == 0 || !v.hasEntries() {
		return false
	}
	layout := v.layout(width, height, r.stats)
	detailWidth := layout.detailWidth
	detailHeight := layout.detailHeight
	if detailWidth < 1 || detailHeight < 1 {
		return false
	}
	_, body := v.detailParts(detailWidth, r)
	bodyHeight := v.detailBodyHeight(detailWidth, detailHeight, len(body), r.stats)
	if bodyHeight < 1 {
		return false
	}
	maxOffset := max(len(body)-bodyHeight, 0)
	next := min(max(v.detailOffset+delta, 0), maxOffset)
	if next == v.detailOffset {
		return false
	}
	v.detailOffset = next
	return true
}

func (v *workflowStatsView) scrollDetailEdge(width, height int, top bool, r responseRenderer) bool {
	if v == nil || !v.hasEntries() {
		return false
	}
	layout := v.layout(width, height, r.stats)
	if layout.detailWidth < 1 || layout.detailHeight < 1 {
		return false
	}
	_, body := v.detailParts(layout.detailWidth, r)
	bodyHeight := v.detailBodyHeight(layout.detailWidth, layout.detailHeight, len(body), r.stats)
	maxOffset := max(len(body)-bodyHeight, 0)
	next := maxOffset
	if top {
		next = 0
	}
	if next == v.detailOffset {
		return false
	}
	v.detailOffset = next
	return true
}

func (v *workflowStatsView) render(width, height int, r responseRenderer) workflowStatsRender {
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	if height <= 0 {
		height = 24
	}

	lines := v.renderScreen(width, height, r)
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return workflowStatsRender{
		content:   content,
		lineCount: len(lines),
	}
}

func (v *workflowStatsView) renderScreen(width, height int, r responseRenderer) []string {
	lines := v.summaryLines(width, r.stats)
	if !v.hasEntries() {
		lines = append(
			lines,
			workflowFitLine(r.stats.Message.Render("No workflow steps captured"), width),
		)
		return workflowFitLines(lines, width, height)
	}

	layout := v.layout(width, height, r.stats)
	lines = append(lines, workflowDivider(width, r.stats))
	if layout.sideBySide {
		left := v.renderStepList(layout.listWidth, layout.listHeight, r.stats)
		right := v.renderDetail(layout.detailWidth, layout.detailHeight, r)
		lines = append(
			lines,
			workflowJoinColumns(left, right, layout.listWidth, layout.detailWidth, r.stats)...)
		return workflowFitLines(lines, width, height)
	}

	lines = append(lines, v.renderStepList(layout.listWidth, layout.listHeight, r.stats)...)
	lines = append(lines, workflowDivider(width, r.stats))
	lines = append(lines, v.renderDetail(layout.detailWidth, layout.detailHeight, r)...)
	return workflowFitLines(lines, width, height)
}

type workflowStatsLayout struct {
	sideBySide   bool
	listWidth    int
	detailWidth  int
	listHeight   int
	detailHeight int
}

func (v *workflowStatsView) layout(width, height int, pal statsPalette) workflowStatsLayout {
	summaryHeight := len(v.summaryLines(width, pal)) + 1
	available := max(height-summaryHeight, 1)

	if width >= workflowStatsSplitMinWidth && height >= workflowStatsSplitMinH {
		gapWidth := visibleWidth(workflowStatsGap)
		listWidth := min(max(width*38/100, 32), 48)
		detailWidth := width - listWidth - gapWidth
		if detailWidth < 32 {
			detailWidth = 32
			listWidth = width - detailWidth - gapWidth
			if listWidth < 20 {
				listWidth = max(width/2-gapWidth, 1)
				detailWidth = max(width-listWidth-gapWidth, 1)
			}
		}
		return workflowStatsLayout{
			sideBySide:   true,
			listWidth:    listWidth,
			detailWidth:  detailWidth,
			listHeight:   available,
			detailHeight: available,
		}
	}

	listHeight := max(height/3, 4)
	if v != nil && len(v.entries)+1 < listHeight {
		listHeight = len(v.entries) + 1
	}
	maxList := max(available-4, 1)
	if listHeight > maxList {
		listHeight = maxList
	}
	detailHeight := max(available-listHeight-1, 1)
	return workflowStatsLayout{
		listWidth:    width,
		detailWidth:  width,
		listHeight:   listHeight,
		detailHeight: detailHeight,
	}
}

func (v *workflowStatsView) summaryLines(width int, pal statsPalette) []string {
	label := strings.TrimSpace(v.label)
	if label == "" {
		label = "Workflow"
	}
	name := strings.TrimSpace(v.name)
	if name == "" {
		name = label
	}

	status := v.overallStatus()
	title := pal.Title.Render(label) + " " +
		pal.Value.Render(workflowPlainTruncate(name, max(width-24, 8))) + " " +
		workflowStatusBadge(status, pal)

	counts := v.counts()
	total := v.totalSteps
	if total == 0 {
		total = len(v.entries)
	}
	countLine := fmt.Sprintf(
		"Steps %d  Pass %d  Fail %d  Skipped %d  Canceled %d",
		total,
		counts.pass,
		counts.fail,
		counts.skipped,
		counts.canceled,
	)

	elapsed := "-"
	if !v.started.IsZero() && !v.ended.IsZero() && !v.ended.Before(v.started) {
		elapsed = workflowFormatDuration(v.ended.Sub(v.started))
		if elapsed == "" {
			elapsed = "0s"
		}
	}
	timeLine := fmt.Sprintf(
		"Elapsed %s  Started %s  Ended %s",
		elapsed,
		workflowFormatTime(v.started),
		workflowFormatTime(v.ended),
	)

	return workflowFitLines([]string{
		title,
		pal.SubLabel.Render(countLine),
		pal.SubLabel.Render(timeLine),
	}, width, 3)
}

type workflowStatsCounts struct {
	pass     int
	fail     int
	skipped  int
	canceled int
}

func (v *workflowStatsView) counts() workflowStatsCounts {
	var counts workflowStatsCounts
	for _, entry := range v.entries {
		switch {
		case entry.result.Canceled:
			counts.canceled++
		case entry.result.Skipped:
			counts.skipped++
		case entry.result.Success:
			counts.pass++
		default:
			counts.fail++
		}
	}
	return counts
}

func (v *workflowStatsView) overallStatus() string {
	counts := v.counts()
	switch {
	case counts.pass+counts.fail+counts.skipped+counts.canceled == 0:
		return "UNKNOWN"
	case counts.fail > 0:
		return "FAIL"
	case counts.canceled > 0:
		return "CANCELED"
	case counts.skipped > 0 && counts.pass == 0:
		return "SKIPPED"
	default:
		return "PASS"
	}
}

func (v *workflowStatsView) renderStepList(width, height int, pal statsPalette) []string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		return nil
	}
	lines := []string{workflowStatsSectionHeading("Steps", width, !v.detailFocus, pal)}
	rowsHeight := height - 1
	if rowsHeight < 1 {
		return workflowFitLines(lines, width, height)
	}
	start := workflowWindowStart(len(v.entries), v.selected, rowsHeight)
	end := min(start+rowsHeight, len(v.entries))
	for i := start; i < end; i++ {
		lines = append(lines, v.renderStepRow(i, width, pal))
	}
	return workflowFitLines(lines, width, height)
}

func workflowWindowStart(total, selected, height int) int {
	if total <= 0 || height <= 0 {
		return 0
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}
	start := max(selected-height/2, 0)
	if start+height > total {
		start = total - height
	}
	if start < 0 {
		start = 0
	}
	return start
}

func (v *workflowStatsView) renderStepRow(idx, width int, pal statsPalette) string {
	entry := v.entries[idx]
	selected := idx == v.selected
	marker := " "
	if selected {
		marker = ">"
	}
	prefix := fmt.Sprintf("%s %2d ", marker, entry.index+1)
	status := workflowStatusBadge(workflowStatusText(entry.result), pal)
	meta := workflowEntryMeta(entry.result)
	metaWidth := visibleWidth(meta)
	if metaWidth > 0 {
		metaWidth++
	}
	nameWidth := width - visibleWidth(prefix) - visibleWidth(status) - 2 - metaWidth
	if nameWidth < 6 {
		nameWidth = 6
		if metaWidth > 0 && width < 40 {
			meta = ""
			nameWidth = width - visibleWidth(prefix) - visibleWidth(status) - 2
		}
	}
	if nameWidth < 1 {
		nameWidth = 1
	}
	name := workflowPlainTruncate(
		core.StepLabel(
			entry.result.Step,
			entry.result.Branch,
			entry.result.Iteration,
			entry.result.Total,
		),
		nameWidth,
	)
	line := prefix + status + " " + name
	if meta != "" {
		line += " " + pal.SubLabel.Render(
			workflowPlainTruncate(meta, max(width-visibleWidth(line)-1, 1)),
		)
	}
	line = workflowFitLine(line, width)
	if selected {
		line = pal.Selected.Render(line)
	}
	return line
}

func workflowEntryMeta(res workflowStepResult) string {
	parts := compactStrings(workflowResultStatus(res), workflowFormatDuration(res.Duration))
	return strings.Join(parts, " ")
}

func (v *workflowStatsView) renderDetail(width, height int, r responseRenderer) []string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		return nil
	}
	if !v.hasEntries() || v.selected < 0 || v.selected >= len(v.entries) {
		return workflowFitLines([]string{
			workflowDetailHeading("Selected Step", width, v.detailFocus, r.stats),
			r.stats.Message.Render("No step selected"),
		}, width, height)
	}

	header, body := v.detailParts(width, r)
	bodyHeight := v.detailBodyHeight(width, height, len(body), r.stats)
	v.clampDetailOffset(len(body), bodyHeight)
	rangeLine := workflowDetailRangeLine(v.detailOffset, bodyHeight, len(body))

	lines := append([]string{}, header...)
	if rangeLine != "" && len(lines) < height {
		lines = append(lines, workflowFitLine(r.stats.SubLabel.Render(rangeLine), width))
	}
	if bodyHeight > 0 {
		end := min(v.detailOffset+bodyHeight, len(body))
		if v.detailOffset < len(body) {
			lines = append(lines, body[v.detailOffset:end]...)
		}
	}
	return workflowFitLines(lines, width, height)
}

func (v *workflowStatsView) detailParts(width int, r responseRenderer) ([]string, []string) {
	entry := v.entries[v.selected]
	header := v.detailHeader(entry, width, r.stats)
	body := workflowWrapLines(entry.detailBodyLines("", r), width)
	if len(body) == 0 {
		body = []string{r.stats.Message.Render("<empty>")}
	}
	return header, body
}

func (v *workflowStatsView) detailHeader(
	entry workflowStatsEntry,
	width int,
	pal statsPalette,
) []string {
	title := fmt.Sprintf(
		"%d. %s",
		entry.index+1,
		core.StepLabel(
			entry.result.Step,
			entry.result.Branch,
			entry.result.Iteration,
			entry.result.Total,
		),
	)
	lines := []string{
		workflowDetailHeading("Selected Step", width, v.detailFocus, pal),
		workflowFitLine(pal.Value.Render(workflowPlainTruncate(title, width)), width),
	}

	fields := compactStrings(
		workflowStatusBadge(workflowStatusText(entry.result), pal),
		workflowResultStatus(entry.result),
		workflowFormatDuration(entry.result.Duration),
	)
	if len(fields) > 0 {
		lines = append(lines, workflowFitLine(strings.Join(fields, " "), width))
	}
	if target := workflowStepTarget(entry.result); target != "" {
		lines = append(
			lines,
			workflowFitLine(pal.SubLabel.Render(workflowPlainTruncate(target, width)), width),
		)
	}
	if msg := strings.TrimSpace(entry.result.Message); msg != "" {
		for _, line := range wrapStructuredLine(pal.Message.Render(msg), width) {
			lines = append(lines, workflowFitLine(line, width))
		}
	}
	return lines
}

func workflowDetailHeading(label string, width int, focused bool, pal statsPalette) string {
	return workflowStatsSectionHeading(label, width, focused, pal)
}

func workflowStatsSectionHeading(
	label string,
	width int,
	focused bool,
	pal statsPalette,
) string {
	style := pal.SubLabel
	if focused {
		style = pal.Heading
	}
	return workflowFitLine(style.Render(headerSwitchText(label, focused)), width)
}

func (v *workflowStatsView) detailBodyHeight(width, height, bodyLines int, pal statsPalette) int {
	if !v.hasEntries() || v.selected < 0 || v.selected >= len(v.entries) {
		return 0
	}
	header := v.detailHeader(v.entries[v.selected], width, pal)
	bodyHeight := max(height-len(header)-1, 0)
	if bodyLines == 0 {
		return 0
	}
	return bodyHeight
}

func (v *workflowStatsView) clampDetailOffset(bodyLines, bodyHeight int) {
	maxOffset := max(bodyLines-bodyHeight, 0)
	if v.detailOffset > maxOffset {
		v.detailOffset = maxOffset
	}
	if v.detailOffset < 0 {
		v.detailOffset = 0
	}
}

func workflowDetailRangeLine(offset, height, total int) string {
	if total <= 0 {
		return ""
	}
	if height <= 0 {
		return fmt.Sprintf("Detail 0/%d", total)
	}
	start := offset + 1
	end := min(offset+height, total)
	return fmt.Sprintf("Detail %d-%d/%d", start, end, total)
}

func workflowStepTarget(res workflowStepResult) string {
	req := res.Req
	if req == nil {
		req = res.Src
	}
	if req == nil {
		return strings.TrimSpace(res.Step.Using)
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	target := strings.TrimSpace(requestTarget(req))
	return strings.TrimSpace(strings.Join(compactStrings(method, target), " "))
}

func workflowStepLine(idx int, res workflowStepResult) string {
	label := workflowStatusLabel(res)
	line := fmt.Sprintf(
		"%d. %s %s",
		idx+1,
		core.StepLabel(res.Step, res.Branch, res.Iteration, res.Total),
		label,
	)
	if strings.TrimSpace(res.Status) != "" {
		line += fmt.Sprintf(" (%s)", res.Status)
	}
	if res.Duration > 0 {
		line += fmt.Sprintf(" [%s]", res.Duration.Truncate(time.Millisecond))
	}
	return line
}

func workflowStatusLabel(res workflowStepResult) string {
	switch {
	case res.Canceled:
		return workflowStatusCanceled
	case res.Skipped:
		return workflowStatusSkipped
	case res.Success:
		return workflowStatusPass
	default:
		return workflowStatusFail
	}
}

func workflowStatusText(res workflowStepResult) string {
	switch {
	case res.Canceled:
		return "CANCELED"
	case res.Skipped:
		return "SKIPPED"
	case res.Success:
		return "PASS"
	default:
		return "FAIL"
	}
}

func workflowStatusBadge(status string, pal statsPalette) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case "PASS":
		return pal.Success.Render(padStyled(status, 8))
	case "FAIL":
		return pal.Warn.Render(padStyled(status, 8))
	case "CANCELED":
		return pal.Caution.Render(padStyled(status, 8))
	case "SKIPPED":
		return pal.Caution.Render(padStyled(status, 8))
	default:
		if status == "" {
			status = "UNKNOWN"
		}
		return pal.SubLabel.Render(padStyled(workflowPlainTruncate(status, 8), 8))
	}
}

func workflowResultStatus(res workflowStepResult) string {
	if status := strings.TrimSpace(res.Status); status != "" {
		return status
	}
	if res.HTTP != nil {
		if status := strings.TrimSpace(res.HTTP.Status); status != "" {
			return status
		}
		if res.HTTP.StatusCode > 0 {
			return fmt.Sprintf("%d", res.HTTP.StatusCode)
		}
	}
	if res.GRPC != nil {
		return res.GRPC.StatusCode.String()
	}
	return ""
}

func (entry workflowStatsEntry) detailBodyLines(indent string, r responseRenderer) []string {
	if entry.result.Canceled && !entry.hasResponse() {
		return []string{r.stats.Message.Render(indent + "Canceled before response capture")}
	}
	if entry.result.Skipped {
		reason := strings.TrimSpace(entry.result.Message)
		if reason == "" {
			reason = "Skipped"
		}
		return []string{r.stats.Message.Render(indent + reason)}
	}
	if entry.hasHTTP() {
		views := r.buildHTTPResponseViews(
			entry.result.HTTP,
			entry.result.Tests,
			entry.result.ScriptErr,
		)
		return indentLines(views.pretty, indent, r.stats)
	}
	if entry.hasGRPC() {
		detail := buildWorkflowGRPCDetail(entry.result, r)
		return indentLines(detail, indent, r.stats)
	}
	placeholder := r.stats.Message.Render(indent + "<no response captured>")
	return []string{placeholder}
}

func (entry workflowStatsEntry) hasResponse() bool {
	return entry.hasHTTP() || entry.hasGRPC()
}

func (entry workflowStatsEntry) hasHTTP() bool {
	return entry.result.HTTP != nil
}

func (entry workflowStatsEntry) hasGRPC() bool {
	return entry.result.GRPC != nil
}

func indentLines(content string, indent string, pal statsPalette) []string {
	if strings.TrimSpace(content) == "" {
		return []string{pal.Message.Render(indent + "<empty>")}
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		out = append(out, indent+trimmed)
	}
	return out
}

func buildWorkflowGRPCDetail(result workflowStepResult, renderer responseRenderer) string {
	resp := result.GRPC
	if resp == nil {
		return ""
	}
	method := strings.TrimSpace(result.Step.Using)

	builder := strings.Builder{}
	builder.WriteString(renderer.renderGRPCStatusLine(resp, method))
	builder.WriteString("\n")

	writeMeta := func(title string, meta map[string][]string) {
		hdrs := grpcx.EncodeMetadataHeader(meta)
		if len(hdrs) == 0 {
			return
		}
		builder.WriteString(title)
		builder.WriteString(":\n")
		builder.WriteString(bodyfmt.FormatHeaders(hdrs))
		builder.WriteString("\n")
	}
	writeMeta("Headers", resp.Headers)
	writeMeta("Trailers", resp.Trailers)

	contentType := "application/json"
	bodyRaw := prettifyBody([]byte(resp.Message), contentType)
	body := trimResponseBody(bodyRaw)
	if isBodyEmpty(body) {
		body = "<empty>"
	}
	builder.WriteString(body)
	return strings.TrimRight(builder.String(), "\n")
}

func workflowWrapLines(lines []string, width int) []string {
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped := wrapStructuredLine(line, width)
		if len(wrapped) == 0 {
			out = append(out, "")
			continue
		}
		for _, seg := range wrapped {
			out = append(out, workflowFitLine(seg, width))
		}
	}
	return out
}

func workflowJoinColumns(
	left, right []string,
	leftWidth, rightWidth int,
	pal statsPalette,
) []string {
	height := max(len(left), len(right))
	out := make([]string, 0, height)
	for i := range height {
		l := strings.Repeat(" ", leftWidth)
		r := strings.Repeat(" ", rightWidth)
		if i < len(left) {
			l = workflowFitLine(left[i], leftWidth)
		}
		if i < len(right) {
			r = workflowFitLine(right[i], rightWidth)
		}
		out = append(out, l+pal.SubLabel.Render(workflowStatsGap)+r)
	}
	return out
}

func workflowDivider(width int, pal statsPalette) string {
	if width < 1 {
		width = 1
	}
	return pal.SubLabel.Render(strings.Repeat("─", width))
}

func workflowFitLines(lines []string, width, height int) []string {
	if height < 0 {
		height = 0
	}
	out := make([]string, 0, max(len(lines), height))
	for _, line := range lines {
		out = append(out, workflowFitLine(line, width))
	}
	if height > 0 && len(out) > height {
		out = out[:height]
	}
	for height > 0 && len(out) < height {
		out = append(out, strings.Repeat(" ", max(width, 0)))
	}
	return out
}

func workflowFitLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	if visibleWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	return padStyled(line, width)
}

func workflowPlainTruncate(text string, width int) string {
	text = strings.TrimSpace(text)
	if width <= 0 {
		return ""
	}
	if visibleWidth(text) <= width {
		return text
	}
	if width <= 3 {
		return ansi.Truncate(text, width, "")
	}
	return ansi.Truncate(text, width-3, "") + "..."
}

func workflowFormatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	if d >= time.Millisecond {
		return d.Truncate(time.Millisecond).String()
	}
	return d.String()
}

func workflowFormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format(time.RFC3339)
}
