package ui

import (
	"fmt"
	"strings"
	"time"
)

type workflowStatsView struct {
	name        string
	started     time.Time
	ended       time.Time
	totalSteps  int
	entries     []workflowStatsEntry
	selected    int
	expanded    map[int]bool
	renderCache map[int]workflowStatsRender
	alignTopSelected  bool
	alignTopRemaining int
}

type workflowStatsEntry struct {
	index  int
	result workflowStepResult
}

type workflowStatsRender struct {
	content string
	metrics []workflowStatsMetric
}

type workflowStatsMetric struct {
	index int
	start int
	end   int
}

func newWorkflowStatsView(state *workflowState) *workflowStatsView {
	if state == nil {
		return &workflowStatsView{selected: -1, expanded: make(map[int]bool)}
	}

	entries := make([]workflowStatsEntry, len(state.results))
	for i, res := range state.results {
		entries[i] = workflowStatsEntry{index: i, result: res}
	}

	selected := 0
	if len(entries) == 0 {
		selected = -1
	}

	return &workflowStatsView{
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
	if content != "" {
		content += "\n"
	}

	render := workflowStatsRender{content: content, metrics: metrics}
	v.renderCache[width] = render
	return render
}

func (v *workflowStatsView) workflowHeader() []string {
	name := v.name
	if name == "" {
		name = "Workflow"
	}
	workflow := renderLabelValue("Workflow", name, statsLabelStyle, statsValueStyle)
	started := renderLabelValue("Started", v.started.Format(time.RFC3339), statsLabelStyle, statsValueStyle)
	lines := []string{workflow, started}
	if !v.ended.IsZero() {
		ended := renderLabelValue("Ended", v.ended.Format(time.RFC3339), statsLabelStyle, statsValueStyle)
		lines = append(lines, ended)
	}
	stepCount := fmt.Sprintf("%d", v.totalSteps)
	steps := renderLabelValue("Steps", stepCount, statsLabelStyle, statsValueStyle)
	lines = append(lines, steps, "")
	return lines
}

func (v *workflowStatsView) renderEntryTitle(entry workflowStatsEntry) string {
	status := "PASS"
	if !entry.result.Success {
		status = "FAIL"
	}
	base := fmt.Sprintf("%d. %s [%s]", entry.index+1, displayStepName(entry.result.Step), status)
	if strings.TrimSpace(entry.result.Status) != "" {
		base += fmt.Sprintf(" (%s)", entry.result.Status)
	}
	if entry.result.Duration > 0 {
		base += fmt.Sprintf(" [%s]", entry.result.Duration.Truncate(time.Millisecond))
	}
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

func (entry workflowStatsEntry) detailLines() []string {
	if entry.hasHTTP() {
		pretty, _, _ := buildHTTPResponseViews(entry.result.HTTP, entry.result.Tests, entry.result.ScriptErr)
		return indentLines(pretty, "    ")
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

func (v *workflowStatsView) ensureVisible(pane *responsePaneState, render workflowStatsRender) {
	if pane == nil || !v.hasEntries() || pane.viewport.Height <= 0 {
		return
	}
	if v.selected < 0 || v.selected >= len(render.metrics) {
		return
	}
	metric := render.metrics[v.selected]
	height := pane.viewport.Height
	offset := pane.viewport.YOffset

	if metric.start < offset {
		pane.viewport.SetYOffset(metric.start)
		return
	}
	bottom := offset + height - 1
	if metric.end > bottom {
		pane.viewport.SetYOffset(metric.end - height + 1)
		if pane.viewport.YOffset < 0 {
			pane.viewport.SetYOffset(0)
		}
	}
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
	statusLine := fmt.Sprintf("gRPC %s - %s", strings.TrimPrefix(method, "/"), resp.StatusCode.String())
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
