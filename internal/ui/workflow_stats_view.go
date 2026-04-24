package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

const (
	workflowStatsSplitMinWidth = 96
	workflowStatsSplitMinH     = 12
	workflowStatsGap           = " | "
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
		label:      workflowRunLabel(state),
		name:       workflowRunSubject(state),
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
	next := v.selected + delta
	if next < 0 {
		next = 0
	}
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

func (v *workflowStatsView) scrollDetail(width, height, delta int) bool {
	if v == nil || delta == 0 || !v.hasEntries() {
		return false
	}
	layout := v.layout(width, height)
	detailWidth := layout.detailWidth
	detailHeight := layout.detailHeight
	if detailWidth < 1 || detailHeight < 1 {
		return false
	}
	_, body := v.detailParts(detailWidth)
	bodyHeight := v.detailBodyHeight(detailWidth, detailHeight, len(body))
	if bodyHeight < 1 {
		return false
	}
	maxOffset := len(body) - bodyHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	next := v.detailOffset + delta
	if next < 0 {
		next = 0
	}
	if next > maxOffset {
		next = maxOffset
	}
	if next == v.detailOffset {
		return false
	}
	v.detailOffset = next
	return true
}

func (v *workflowStatsView) scrollDetailEdge(width, height int, top bool) bool {
	if v == nil || !v.hasEntries() {
		return false
	}
	layout := v.layout(width, height)
	if layout.detailWidth < 1 || layout.detailHeight < 1 {
		return false
	}
	_, body := v.detailParts(layout.detailWidth)
	bodyHeight := v.detailBodyHeight(layout.detailWidth, layout.detailHeight, len(body))
	maxOffset := len(body) - bodyHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
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

func (v *workflowStatsView) render(width, height int) workflowStatsRender {
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	if height <= 0 {
		height = 24
	}

	lines := v.renderScreen(width, height)
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return workflowStatsRender{
		content:   content,
		lineCount: len(lines),
	}
}

func (v *workflowStatsView) renderScreen(width, height int) []string {
	lines := v.summaryLines(width)
	if !v.hasEntries() {
		lines = append(lines, workflowFitLine(statsMessageStyle.Render("No workflow steps captured"), width))
		return workflowFitLines(lines, width, height)
	}

	layout := v.layout(width, height)
	lines = append(lines, workflowDivider(width))
	if layout.sideBySide {
		left := v.renderStepList(layout.listWidth, layout.listHeight)
		right := v.renderDetail(layout.detailWidth, layout.detailHeight)
		lines = append(lines, workflowJoinColumns(left, right, layout.listWidth, layout.detailWidth)...)
		return workflowFitLines(lines, width, height)
	}

	lines = append(lines, v.renderStepList(layout.listWidth, layout.listHeight)...)
	lines = append(lines, workflowDivider(width))
	lines = append(lines, v.renderDetail(layout.detailWidth, layout.detailHeight)...)
	return workflowFitLines(lines, width, height)
}

type workflowStatsLayout struct {
	sideBySide   bool
	listWidth    int
	detailWidth  int
	listHeight   int
	detailHeight int
}

func (v *workflowStatsView) layout(width, height int) workflowStatsLayout {
	summaryHeight := len(v.summaryLines(width)) + 1
	available := height - summaryHeight
	if available < 1 {
		available = 1
	}

	if width >= workflowStatsSplitMinWidth && height >= workflowStatsSplitMinH {
		gapWidth := visibleWidth(workflowStatsGap)
		listWidth := width * 38 / 100
		if listWidth < 32 {
			listWidth = 32
		}
		if listWidth > 48 {
			listWidth = 48
		}
		detailWidth := width - listWidth - gapWidth
		if detailWidth < 32 {
			detailWidth = 32
			listWidth = width - detailWidth - gapWidth
			if listWidth < 20 {
				listWidth = maxInt(width/2-gapWidth, 1)
				detailWidth = maxInt(width-listWidth-gapWidth, 1)
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

	listHeight := height / 3
	if listHeight < 4 {
		listHeight = 4
	}
	if v != nil && len(v.entries)+1 < listHeight {
		listHeight = len(v.entries) + 1
	}
	maxList := available - 4
	if maxList < 1 {
		maxList = 1
	}
	if listHeight > maxList {
		listHeight = maxList
	}
	detailHeight := available - listHeight - 1
	if detailHeight < 1 {
		detailHeight = 1
	}
	return workflowStatsLayout{
		listWidth:    width,
		detailWidth:  width,
		listHeight:   listHeight,
		detailHeight: detailHeight,
	}
}

func (v *workflowStatsView) summaryLines(width int) []string {
	label := strings.TrimSpace(v.label)
	if label == "" {
		label = "Workflow"
	}
	name := strings.TrimSpace(v.name)
	if name == "" {
		name = label
	}

	status := v.overallStatus()
	title := statsTitleStyle.Render(label) + " " +
		statsValueStyle.Render(workflowPlainTruncate(name, maxInt(width-24, 8))) + " " +
		workflowStatusBadge(status)

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
		statsSubLabelStyle.Render(countLine),
		statsSubLabelStyle.Render(timeLine),
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

func (v *workflowStatsView) renderStepList(width, height int) []string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		return nil
	}
	heading := statsHeadingStyle.Render("Steps")
	if !v.detailFocus {
		heading = statsSelectedStyle.Render(workflowFitLine(heading, width))
	}
	lines := []string{workflowFitLine(heading, width)}
	rowsHeight := height - 1
	if rowsHeight < 1 {
		return workflowFitLines(lines, width, height)
	}
	start := workflowWindowStart(len(v.entries), v.selected, rowsHeight)
	end := start + rowsHeight
	if end > len(v.entries) {
		end = len(v.entries)
	}
	for i := start; i < end; i++ {
		lines = append(lines, v.renderStepRow(i, width))
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
	start := selected - height/2
	if start < 0 {
		start = 0
	}
	if start+height > total {
		start = total - height
	}
	if start < 0 {
		start = 0
	}
	return start
}

func (v *workflowStatsView) renderStepRow(idx, width int) string {
	entry := v.entries[idx]
	selected := idx == v.selected
	marker := " "
	if selected {
		marker = ">"
	}
	prefix := fmt.Sprintf("%s %2d ", marker, entry.index+1)
	status := workflowStatusBadge(workflowStatusText(entry.result))
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
		workflowStepLabel(
			entry.result.Step,
			entry.result.Branch,
			entry.result.Iteration,
			entry.result.Total,
		),
		nameWidth,
	)
	line := prefix + status + " " + name
	if meta != "" {
		line += " " + statsSubLabelStyle.Render(workflowPlainTruncate(meta, maxInt(width-visibleWidth(line)-1, 1)))
	}
	line = workflowFitLine(line, width)
	if selected {
		line = statsSelectedStyle.Render(line)
	}
	return line
}

func workflowEntryMeta(res workflowStepResult) string {
	parts := compactStrings(workflowResultStatus(res), workflowFormatDuration(res.Duration))
	return strings.Join(parts, " ")
}

func (v *workflowStatsView) renderDetail(width, height int) []string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		return nil
	}
	if !v.hasEntries() || v.selected < 0 || v.selected >= len(v.entries) {
		return workflowFitLines([]string{
			statsHeadingStyle.Render("Selected Step"),
			statsMessageStyle.Render("No step selected"),
		}, width, height)
	}

	header, body := v.detailParts(width)
	bodyHeight := v.detailBodyHeight(width, height, len(body))
	v.clampDetailOffset(len(body), bodyHeight)
	rangeLine := workflowDetailRangeLine(v.detailOffset, bodyHeight, len(body))

	lines := append([]string{}, header...)
	if rangeLine != "" && len(lines) < height {
		lines = append(lines, workflowFitLine(statsSubLabelStyle.Render(rangeLine), width))
	}
	if bodyHeight > 0 {
		end := v.detailOffset + bodyHeight
		if end > len(body) {
			end = len(body)
		}
		if v.detailOffset < len(body) {
			lines = append(lines, body[v.detailOffset:end]...)
		}
	}
	return workflowFitLines(lines, width, height)
}

func (v *workflowStatsView) detailParts(width int) ([]string, []string) {
	entry := v.entries[v.selected]
	header := v.detailHeader(entry, width)
	body := workflowWrapLines(entry.detailBodyLines(""), width)
	if len(body) == 0 {
		body = []string{statsMessageStyle.Render("<empty>")}
	}
	return header, body
}

func (v *workflowStatsView) detailHeader(entry workflowStatsEntry, width int) []string {
	title := fmt.Sprintf(
		"%d. %s",
		entry.index+1,
		workflowStepLabel(
			entry.result.Step,
			entry.result.Branch,
			entry.result.Iteration,
			entry.result.Total,
		),
	)
	lines := []string{
		workflowDetailHeading("Selected Step", width, v.detailFocus),
		workflowFitLine(statsValueStyle.Render(workflowPlainTruncate(title, width)), width),
	}

	fields := compactStrings(
		workflowStatusBadge(workflowStatusText(entry.result)),
		workflowResultStatus(entry.result),
		workflowFormatDuration(entry.result.Duration),
	)
	if len(fields) > 0 {
		lines = append(lines, workflowFitLine(strings.Join(fields, "  "), width))
	}
	if target := workflowStepTarget(entry.result); target != "" {
		lines = append(lines, workflowFitLine(statsSubLabelStyle.Render(workflowPlainTruncate(target, width)), width))
	}
	if msg := strings.TrimSpace(entry.result.Message); msg != "" {
		for _, line := range wrapStructuredLine(statsMessageStyle.Render(msg), width) {
			lines = append(lines, workflowFitLine(line, width))
		}
	}
	return lines
}

func workflowDetailHeading(label string, width int, focused bool) string {
	line := workflowFitLine(statsHeadingStyle.Render(label), width)
	if focused {
		return statsSelectedStyle.Render(line)
	}
	return line
}

func (v *workflowStatsView) detailBodyHeight(width, height, bodyLines int) int {
	if !v.hasEntries() || v.selected < 0 || v.selected >= len(v.entries) {
		return 0
	}
	header := v.detailHeader(v.entries[v.selected], width)
	bodyHeight := height - len(header) - 1
	if bodyHeight < 0 {
		bodyHeight = 0
	}
	if bodyLines == 0 {
		return 0
	}
	return bodyHeight
}

func (v *workflowStatsView) clampDetailOffset(bodyLines, bodyHeight int) {
	maxOffset := bodyLines - bodyHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
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
	end := offset + height
	if end > total {
		end = total
	}
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
		workflowStepLabel(res.Step, res.Branch, res.Iteration, res.Total),
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

func workflowStatusBadge(status string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case "PASS":
		return statsSuccessStyle.Render(padStyled(status, 8))
	case "FAIL":
		return statsWarnStyle.Render(padStyled(status, 8))
	case "CANCELED":
		return statsCautionStyle.Render(padStyled(status, 8))
	case "SKIPPED":
		return statsCautionStyle.Render(padStyled(status, 8))
	default:
		if status == "" {
			status = "UNKNOWN"
		}
		return statsSubLabelStyle.Render(padStyled(workflowPlainTruncate(status, 8), 8))
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

func (entry workflowStatsEntry) detailBodyLines(indent string) []string {
	if entry.result.Canceled && !entry.hasResponse() {
		return []string{statsMessageStyle.Render(indent + "Canceled before response capture")}
	}
	if entry.result.Skipped {
		reason := strings.TrimSpace(entry.result.Message)
		if reason == "" {
			reason = "Skipped"
		}
		return []string{statsMessageStyle.Render(indent + reason)}
	}
	if entry.hasHTTP() {
		views := buildHTTPResponseViews(
			entry.result.HTTP,
			entry.result.Tests,
			entry.result.ScriptErr,
		)
		return indentLines(views.pretty, indent)
	}
	if entry.hasGRPC() {
		detail := buildWorkflowGRPCDetail(entry.result)
		return indentLines(detail, indent)
	}
	placeholder := statsMessageStyle.Render(indent + "<no response captured>")
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

func indentLines(content string, indent string) []string {
	if strings.TrimSpace(content) == "" {
		return []string{statsMessageStyle.Render(indent + "<empty>")}
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		out = append(out, indent+trimmed)
	}
	return out
}

func buildWorkflowGRPCDetail(result workflowStepResult) string {
	resp := result.GRPC
	if resp == nil {
		return ""
	}
	method := strings.TrimSpace(result.Step.Using)
	statusLine := fmt.Sprintf(
		"gRPC %s - %s",
		strings.TrimPrefix(method, "/"),
		resp.StatusCode.String(),
	)
	if resp.StatusMessage != "" {
		statusLine += " (" + resp.StatusMessage + ")"
	}

	builder := strings.Builder{}
	builder.WriteString(statusLine)
	builder.WriteString("\n")

	if len(resp.Headers) > 0 {
		builder.WriteString("Headers:\n")
		for name, values := range resp.Headers {
			fmt.Fprintf(&builder, "%s: %s\n", name, strings.Join(values, ", "))
		}
	}
	if len(resp.Trailers) > 0 {
		builder.WriteString("Trailers:\n")
		for name, values := range resp.Trailers {
			fmt.Fprintf(&builder, "%s: %s\n", name, strings.Join(values, ", "))
		}
	}

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

func workflowJoinColumns(left, right []string, leftWidth, rightWidth int) []string {
	height := maxInt(len(left), len(right))
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		l := strings.Repeat(" ", leftWidth)
		r := strings.Repeat(" ", rightWidth)
		if i < len(left) {
			l = workflowFitLine(left[i], leftWidth)
		}
		if i < len(right) {
			r = workflowFitLine(right[i], rightWidth)
		}
		out = append(out, l+statsSubLabelStyle.Render(workflowStatsGap)+r)
	}
	return out
}

func workflowDivider(width int) string {
	if width < 1 {
		width = 1
	}
	return statsSubLabelStyle.Render(strings.Repeat("-", width))
}

func workflowFitLines(lines []string, width, height int) []string {
	if height < 0 {
		height = 0
	}
	out := make([]string, 0, maxInt(len(lines), height))
	for _, line := range lines {
		out = append(out, workflowFitLine(line, width))
	}
	if height > 0 && len(out) > height {
		out = out[:height]
	}
	for height > 0 && len(out) < height {
		out = append(out, strings.Repeat(" ", maxInt(width, 0)))
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
