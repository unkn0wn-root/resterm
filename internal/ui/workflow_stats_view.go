package ui

import (
	"fmt"
	"strings"
	"time"
)

type workflowStatsView struct {
	label       string
	name        string
	started     time.Time
	ended       time.Time
	totalSteps  int
	entries     []workflowStatsEntry
	selected    int
	expanded    map[int]bool
	renderCache map[int]workflowStatsRender
}

type workflowStatsEntry struct {
	index  int
	result workflowStepResult
}

type workflowStatsRender struct {
	content   string
	metrics   []workflowStatsMetric
	lineCount int
}

type workflowStatsMetric struct {
	index int
	start int
	end   int
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
		res := workflowStepResult{
			Step:     step,
			Canceled: true,
		}
		entries = append(entries, workflowStatsEntry{index: i, result: res})
	}
	return entries
}

func newWorkflowStatsView(state *workflowState) *workflowStatsView {
	if state == nil {
		return &workflowStatsView{selected: -1, expanded: make(map[int]bool)}
	}

	entries := buildWorkflowStatsEntries(state)

	selected := 0
	if len(entries) == 0 {
		selected = -1
	}

	return &workflowStatsView{
		label:       workflowRunLabel(state),
		name:        strings.TrimSpace(state.workflow.Name),
		started:     state.start,
		ended:       state.end,
		totalSteps:  len(state.steps),
		entries:     entries,
		selected:    selected,
		expanded:    make(map[int]bool),
		renderCache: make(map[int]workflowStatsRender),
	}
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
	v.invalidate()
	return true
}

func (v *workflowStatsView) toggle() bool {
	if !v.hasEntries() || v.selected < 0 || v.selected >= len(v.entries) {
		return false
	}
	if v.expanded == nil {
		v.expanded = make(map[int]bool)
	}
	curr := v.expanded[v.selected]
	v.expanded[v.selected] = !curr
	if !v.expanded[v.selected] {
		delete(v.expanded, v.selected)
	}
	v.invalidate()
	return true
}

func (v *workflowStatsView) invalidate() {
	if v.renderCache != nil {
		v.renderCache = make(map[int]workflowStatsRender)
	}
}

func (v *workflowStatsView) scrollExpanded(pane *responsePaneState, delta int) bool {
	if pane == nil || v == nil {
		return false
	}
	if v.selected < 0 || v.selected >= len(v.entries) {
		return false
	}
	if v.expanded == nil || !v.expanded[v.selected] {
		return false
	}

	before := pane.viewport.YOffset
	if delta > 0 {
		pane.viewport.ScrollDown(1)
	} else if delta < 0 {
		pane.viewport.ScrollUp(1)
	}
	return pane.viewport.YOffset != before
}

func (v *workflowStatsView) render(width int) workflowStatsRender {
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	if v.renderCache == nil {
		v.renderCache = make(map[int]workflowStatsRender)
	}
	if render, ok := v.renderCache[width]; ok {
		return render
	}

	lines := []string{}
	metrics := make([]workflowStatsMetric, 0, len(v.entries))

	header := v.workflowHeader()
	for _, line := range header {
		lines = append(lines, wrapStructuredLine(line, width)...)
	}

	for idx, entry := range v.entries {
		start := len(lines)
		title := v.renderEntryTitle(entry)
		lines = append(lines, wrapStructuredLine(title, width)...)

		if msg := strings.TrimSpace(entry.result.Message); msg != "" {
			msgLine := statsMessageStyle.Render("    " + msg)
			lines = append(lines, wrapStructuredLine(msgLine, width)...)
		}

		if v.expanded[idx] || !entry.hasResponse() {
			detailLines := entry.detailLines()
			for _, dl := range detailLines {
				lines = append(lines, wrapStructuredLine(dl, width)...)
			}
		}

		end := len(lines) - 1
		if end < start {
			end = start
		}
		metrics = append(metrics, workflowStatsMetric{index: idx, start: start, end: end})
	}

	content := strings.Join(lines, "\n")
	lineCount := len(lines)
	if content != "" {
		content += "\n"
	}

	render := workflowStatsRender{content: content, metrics: metrics, lineCount: lineCount}
	v.renderCache[width] = render
	return render
}

func (v *workflowStatsView) workflowHeader() []string {
	label := strings.TrimSpace(v.label)
	if label == "" {
		label = "Workflow"
	}
	name := strings.TrimSpace(v.name)
	if name == "" {
		name = label
	}
	workflow := renderLabelValue(label, name, statsLabelStyle, statsValueStyle)
	started := renderLabelValue(
		"Started",
		v.started.Format(time.RFC3339),
		statsLabelStyle,
		statsValueStyle,
	)
	lines := []string{workflow, started}
	if !v.ended.IsZero() {
		ended := renderLabelValue(
			"Ended",
			v.ended.Format(time.RFC3339),
			statsLabelStyle,
			statsValueStyle,
		)
		lines = append(lines, ended)
	}
	stepCount := fmt.Sprintf("%d", v.totalSteps)
	steps := renderLabelValue("Steps", stepCount, statsLabelStyle, statsValueStyle)
	lines = append(lines, steps, "")
	return lines
}

func (v *workflowStatsView) renderEntryTitle(entry workflowStatsEntry) string {
	base := workflowStepLine(entry.index, entry.result)
	colored := colorizeWorkflowStepLine(base)

	indicator := "[+]"
	if entry.hasResponse() {
		if v.expanded[entry.index] {
			indicator = "[-]"
		}
	} else {
		indicator = "[ ]"
	}

	line := fmt.Sprintf("%s %s", indicator, colored)
	if entry.index == v.selected {
		return statsSelectedStyle.Render(line)
	}
	return line
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

func (entry workflowStatsEntry) detailLines() []string {
	if entry.result.Canceled && !entry.hasResponse() {
		return nil
	}
	if entry.result.Skipped {
		reason := strings.TrimSpace(entry.result.Message)
		if reason == "" {
			reason = "Skipped"
		}
		return []string{statsMessageStyle.Render("    " + reason)}
	}
	if entry.hasHTTP() {
		views := buildHTTPResponseViews(
			entry.result.HTTP,
			entry.result.Tests,
			entry.result.ScriptErr,
		)
		return indentLines(views.pretty, "    ")
	}
	if entry.hasGRPC() {
		detail := buildWorkflowGRPCDetail(entry.result)
		return indentLines(detail, "    ")
	}
	placeholder := statsMessageStyle.Render("    <no response captured>")
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

func (v *workflowStatsView) alignSelection(
	pane *responsePaneState,
	render workflowStatsRender,
	forceTop bool,
) bool {
	if pane == nil || !v.hasEntries() || pane.viewport.Height <= 0 {
		return false
	}
	if v.selected < 0 || v.selected >= len(render.metrics) {
		return false
	}
	metric := render.metrics[v.selected]
	height := pane.viewport.Height
	offset := pane.viewport.YOffset
	total := render.lineCount
	if total < metric.end+1 {
		total = metric.end + 1
	}
	buf := height / 4
	if buf < 1 {
		buf = 1
	}
	if buf > 5 {
		buf = 5
	}
	top := offset + buf
	bottom := offset + height - 1 - buf
	target := offset
	if forceTop {
		target = metric.start - buf
	} else {
		switch {
		case metric.start < top:
			target = metric.start - buf
		case metric.start > bottom:
			target = metric.start - buf
		}
	}
	if target < 0 {
		target = 0
	}
	maxOff := v.clampOffset(render, height, total)
	if target > maxOff {
		target = maxOff
	}
	if target == offset {
		return false
	}
	pane.viewport.SetYOffset(target)
	return true
}

func (v *workflowStatsView) clampOffset(render workflowStatsRender, height int, target int) int {
	if height < 1 {
		height = 1
	}
	lineCount := render.lineCount
	if len(render.metrics) > 0 {
		maxMetric := render.metrics[len(render.metrics)-1].end + 1
		if maxMetric > lineCount {
			lineCount = maxMetric
		}
	}
	maxOffset := lineCount - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if target < 0 {
		return 0
	}
	if target > maxOffset {
		return maxOffset
	}
	return target
}

func (v *workflowStatsView) ensureVisible(pane *responsePaneState, render workflowStatsRender) {
	v.alignSelection(pane, render, false)
}

func (v *workflowStatsView) ensureVisibleImmediate(
	pane *responsePaneState,
	render workflowStatsRender,
) bool {
	if pane == nil || !v.hasEntries() || pane.viewport.Height <= 0 {
		return false
	}
	if v.selected < 0 || v.selected >= len(render.metrics) {
		return false
	}
	return v.alignSelection(pane, render, false)
}

func (v *workflowStatsView) selectVisibleStart(
	pane *responsePaneState,
	render workflowStatsRender,
	direction int,
) bool {
	if pane == nil || !v.hasEntries() || pane.viewport.Height <= 0 {
		return false
	}
	if len(render.metrics) == 0 {
		return false
	}
	height := pane.viewport.Height
	offset := pane.viewport.YOffset
	if height <= 0 {
		height = 1
	}
	top := offset
	bottom := offset + height - 1
	maxOffset := v.clampOffset(render, height, render.lineCount)
	buf := height / 5
	if buf < 1 {
		buf = 1
	}
	if buf > 5 {
		buf = 5
	}
	var currIdx int
	for i, metric := range render.metrics {
		if metric.index == v.selected {
			currIdx = i
			break
		}
	}
	candidate := -1
	if direction > 0 {
		if currIdx+1 < len(render.metrics) {
			next := render.metrics[currIdx+1]
			if next.start <= bottom-buf || offset >= maxOffset {
				candidate = next.index
			}
		}
	} else if direction < 0 {
		if currIdx-1 >= 0 {
			prev := render.metrics[currIdx-1]
			if prev.start >= top+buf || offset <= 0 {
				candidate = prev.index
			}
		}
	}

	if candidate == -1 || candidate == v.selected {
		return false
	}
	v.selected = candidate
	v.invalidate()
	return true
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
	if grpc := result.Step; grpc.Using != "" {
		method = grpc.Using
	}
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
			builder.WriteString(fmt.Sprintf("%s: %s\n", name, strings.Join(values, ", ")))
		}
	}
	if len(resp.Trailers) > 0 {
		builder.WriteString("Trailers:\n")
		for name, values := range resp.Trailers {
			builder.WriteString(fmt.Sprintf("%s: %s\n", name, strings.Join(values, ", ")))
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
