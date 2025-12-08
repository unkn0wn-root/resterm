package ui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

type workflowState struct {
	doc          *restfile.Document
	options      httpclient.Options
	workflow     restfile.Workflow
	steps        []workflowStepRuntime
	index        int
	vars         map[string]string
	results      []workflowStepResult
	current      *restfile.Request
	start        time.Time
	end          time.Time
	stepStart    time.Time
	canceled     bool
	cancelReason string
}

type workflowStepRuntime struct {
	step    restfile.WorkflowStep
	request *restfile.Request
}

type workflowStepResult struct {
	Step      restfile.WorkflowStep
	Success   bool
	Canceled  bool
	Status    string
	Duration  time.Duration
	Message   string
	HTTP      *httpclient.Response
	GRPC      *grpcclient.Response
	Tests     []scripts.TestResult
	ScriptErr error
	Err       error
}

const (
	workflowStatusPass     = "[PASS]"
	workflowStatusFail     = "[FAIL]"
	workflowStatusCanceled = "[CANCELED]"
)

func (s *workflowState) matches(req *restfile.Request) bool {
	return s != nil && s.current != nil && req != nil && s.current == req
}

func (m *Model) startWorkflowRun(doc *restfile.Document, workflow restfile.Workflow, options httpclient.Options) tea.Cmd {
	if doc == nil {
		m.setStatusMessage(statusMsg{text: "No document loaded", level: statusWarn})
		return nil
	}
	if len(workflow.Steps) == 0 {
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("Workflow %s has no steps", workflow.Name), level: statusWarn})
		return nil
	}
	if m.workflowRun != nil {
		m.setStatusMessage(statusMsg{text: "Another workflow is already running", level: statusWarn})
		return nil
	}
	if key := workflowKey(&workflow); key != "" {
		m.activeWorkflowKey = key
	}

	stepRuntimes, err := m.prepareWorkflowSteps(doc, workflow)
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}

	state := &workflowState{
		doc:      doc,
		options:  options,
		workflow: workflow,
		steps:    stepRuntimes,
		vars:     make(map[string]string),
		start:    time.Now(),
	}
	for key, value := range workflow.Options {
		if strings.HasPrefix(key, "vars.") {
			state.vars[key] = value
		}
	}
	m.workflowRun = state
	m.statusPulseBase = ""
	m.statusPulseFrame = -1

	return m.executeWorkflowStep()
}

func (m *Model) prepareWorkflowSteps(doc *restfile.Document, workflow restfile.Workflow) ([]workflowStepRuntime, error) {
	if len(doc.Requests) == 0 {
		return nil, fmt.Errorf("workflow %s: no requests defined", workflow.Name)
	}
	lookup := make(map[string]*restfile.Request)
	for _, req := range doc.Requests {
		name := strings.TrimSpace(req.Metadata.Name)
		if name == "" {
			continue
		}
		lookup[strings.ToLower(name)] = req
	}
	steps := make([]workflowStepRuntime, 0, len(workflow.Steps))
	for idx, step := range workflow.Steps {
		key := strings.ToLower(strings.TrimSpace(step.Using))
		if key == "" {
			return nil, fmt.Errorf("workflow %s: step %d missing 'using' request", workflow.Name, idx+1)
		}
		req, ok := lookup[key]
		if !ok {
			return nil, fmt.Errorf("workflow %s: request %s not found", workflow.Name, step.Using)
		}
		steps = append(steps, workflowStepRuntime{step: step, request: req})
	}
	return steps, nil
}

func (m *Model) executeWorkflowStep() tea.Cmd {
	state := m.workflowRun
	if state == nil {
		return nil
	}
	if state.index >= len(state.steps) {
		return m.finalizeWorkflowRun(state)
	}
	runtime := state.steps[state.index]
	clone := cloneRequest(runtime.request)
	extraVars := make(map[string]string)
	for key, value := range state.vars {
		extraVars[key] = value
	}
	for key, value := range runtime.step.Vars {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "vars.") {
			trimmed = "vars." + trimmed
		}
		extraVars[trimmed] = value
		if strings.HasPrefix(trimmed, "vars.workflow.") {
			state.vars[trimmed] = value
		}
	}

	state.current = clone
	state.stepStart = time.Now()
	message := fmt.Sprintf("Workflow %s %d/%d: %s", state.workflow.Name, state.index+1, len(state.steps), runtime.step.Using)
	m.statusPulseBase = message
	m.statusPulseFrame = -1
	m.setStatusMessage(statusMsg{text: message, level: statusInfo})
	m.sending = true

	options := state.options
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}

	cmd := m.executeRequest(state.doc, clone, options, "", extraVars)
	if tick := m.startStatusPulse(); tick != nil {
		return tea.Batch(cmd, tick)
	}
	return cmd
}

func (m *Model) handleWorkflowResponse(msg responseMsg) tea.Cmd {
	state := m.workflowRun
	if state == nil {
		return nil
	}
	current := state.current
	state.current = nil
	m.statusPulseBase = ""
	m.statusPulseFrame = 0
	m.sending = false

	canceled := state.canceled || isCanceled(msg.err)

	var cmds []tea.Cmd
	if canceled {
		state.canceled = true
		m.lastError = nil
		msg.err = nil
		if strings.TrimSpace(state.cancelReason) == "" {
			state.cancelReason = "Workflow canceled"
		}
		if current != nil && state.index < len(state.steps) {
			state.index++
		}
	} else {
		if msg.err != nil {
			if cmd := m.consumeRequestError(msg.err); cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else if msg.response != nil {
			if cmd := m.consumeHTTPResponse(msg.response, msg.tests, msg.scriptErr, msg.environment); cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else if msg.grpc != nil {
			if cmd := m.consumeGRPCResponse(msg.grpc, msg.tests, msg.scriptErr, msg.executed, msg.environment); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	if canceled {
		if next := m.finalizeWorkflowRun(state); next != nil {
			cmds = append(cmds, next)
		}
		return batchCmds(cmds)
	}

	result := evaluateWorkflowStep(state, msg)
	state.results = append(state.results, result)
	shouldStop := !result.Success && result.Step.OnFailure != restfile.WorkflowOnFailureContinue
	state.index++

	var next tea.Cmd
	if shouldStop || state.index >= len(state.steps) {
		next = m.finalizeWorkflowRun(state)
	} else {
		next = m.executeWorkflowStep()
	}
	if next != nil {
		cmds = append(cmds, next)
	}
	return batchCmds(cmds)
}

func evaluateWorkflowStep(state *workflowState, msg responseMsg) workflowStepResult {
	step := state.steps[state.index].step
	status := ""
	duration := time.Since(state.stepStart)
	success := true
	message := ""
	httpResp := cloneHTTPResponse(msg.response)
	grpcResp := cloneGRPCResponse(msg.grpc)
	tests := append([]scripts.TestResult(nil), msg.tests...)

	if msg.response != nil {
		status = msg.response.Status
		if msg.response.Duration > 0 {
			duration = msg.response.Duration
		}
		if msg.response.StatusCode >= 400 {
			success = false
			message = fmt.Sprintf("unexpected status code %d", msg.response.StatusCode)
		}
	} else if msg.err != nil {
		success = false
		status = errdef.Message(msg.err)
		message = status
	} else if msg.grpc != nil {
		status = msg.grpc.StatusCode.String()
	} else {
		success = false
		message = "request failed"
	}

	if success && msg.scriptErr != nil {
		success = false
		message = msg.scriptErr.Error()
	}
	if success {
		for _, test := range msg.tests {
			if !test.Passed {
				success = false
				if strings.TrimSpace(test.Message) != "" {
					message = test.Message
				} else {
					message = fmt.Sprintf("test failed: %s", test.Name)
				}
				break
			}
		}
	}

	if exp, ok := step.Expect["status"]; ok {
		expected := strings.TrimSpace(exp)
		if strings.TrimSpace(status) == "" || !strings.EqualFold(expected, strings.TrimSpace(status)) {
			success = false
			if expected != "" {
				message = fmt.Sprintf("expected status %s", expected)
			}
		}
	}
	if exp, ok := step.Expect["statuscode"]; ok {
		expectedCode, err := strconv.Atoi(strings.TrimSpace(exp))
		if err != nil {
			message = fmt.Sprintf("invalid expected status code %q", exp)
			success = false
		} else {
			actual := 0
			if msg.response != nil {
				actual = msg.response.StatusCode
			}
			if actual != expectedCode {
				success = false
				message = fmt.Sprintf("expected status code %d", expectedCode)
			}
		}
	}

	return workflowStepResult{
		Step:      step,
		Success:   success,
		Status:    status,
		Duration:  duration,
		Message:   message,
		HTTP:      httpResp,
		GRPC:      grpcResp,
		Tests:     tests,
		ScriptErr: msg.scriptErr,
		Err:       msg.err,
	}
}

func (m *Model) finalizeWorkflowRun(state *workflowState) tea.Cmd {
	if state != nil {
		state.end = time.Now()
	}
	report := m.buildWorkflowReport(state)
	summary := workflowSummary(state)
	statsView := newWorkflowStatsView(state)
	m.workflowRun = nil
	m.sending = false
	m.statusPulseBase = ""
	m.statusPulseFrame = 0
	m.setStatusMessage(statusMsg{text: summary, level: workflowStatusLevel(state)})
	m.recordWorkflowHistory(state, summary, report)

	if m.responseLatest != nil {
		m.responseLatest.stats = report
		m.responseLatest.statsColored = ""
		m.responseLatest.statsColorize = true
		m.responseLatest.statsKind = statsReportKindWorkflow
		m.responseLatest.workflowStats = statsView
	} else {
		snapshot := &responseSnapshot{
			pretty:         report,
			raw:            report,
			headers:        report,
			requestHeaders: report,
			stats:          report,
			statsColorize:  true,
			statsKind:      statsReportKindWorkflow,
			statsColored:   "",
			workflowStats:  statsView,
			ready:          true,
		}
		m.responseLatest = snapshot
		m.responsePending = nil
	}

	var cmd tea.Cmd
	if m.responseLatest != nil && m.responseLatest.workflowStats != nil {
		m.invalidateWorkflowStatsCaches(m.responseLatest)
		cmd = m.activateWorkflowStatsView(m.responseLatest)
	}

	return cmd
}

func workflowSummary(state *workflowState) string {
	if state == nil {
		return "Workflow complete"
	}
	if state.canceled {
		name := strings.TrimSpace(state.workflow.Name)
		if name == "" {
			name = "workflow"
		}
		done := len(state.results)
		total := len(state.steps)
		step := done
		if done < total {
			step = done + 1
		}
		if step <= 0 {
			step = 1
		}
		if total == 0 {
			total = step
		}
		if step > total && total > 0 {
			step = total
		}
		return fmt.Sprintf("Workflow %s canceled at step %d/%d", name, step, total)
	}

	succeeded := 0
	for _, result := range state.results {
		if result.Success {
			succeeded++
		}
	}
	total := len(state.results)
	if total == 0 {
		total = len(state.steps)
	}
	failed := total - succeeded
	if failed == 0 {
		return fmt.Sprintf("Workflow %s completed: %d/%d steps passed", state.workflow.Name, succeeded, total)
	}
	last := state.results[len(state.results)-1]
	if last.Success {
		return fmt.Sprintf("Workflow %s finished with %d failure(s)", state.workflow.Name, failed)
	}
	reason := strings.TrimSpace(last.Message)
	if reason == "" {
		reason = "step failed"
	}
	return fmt.Sprintf("Workflow %s failed at step %s: %s", state.workflow.Name, displayStepName(last.Step), reason)
}

func workflowStatusLevel(state *workflowState) statusLevel {
	if state != nil && state.canceled {
		return statusWarn
	}
	for _, result := range state.results {
		if !result.Success {
			return statusWarn
		}
	}
	return statusSuccess
}

func (m *Model) buildWorkflowReport(state *workflowState) string {
	var builder strings.Builder
	if state == nil {
		return ""
	}
	name := state.workflow.Name
	if strings.TrimSpace(name) == "" {
		name = "Workflow"
	}
	builder.WriteString(fmt.Sprintf("Workflow: %s\n", name))
	builder.WriteString(fmt.Sprintf("Started: %s\n", state.start.Format(time.RFC3339)))
	if !state.end.IsZero() {
		builder.WriteString(fmt.Sprintf("Ended: %s\n", state.end.Format(time.RFC3339)))
	}
	builder.WriteString(fmt.Sprintf("Steps: %d\n\n", len(state.steps)))
	for _, entry := range buildWorkflowStatsEntries(state) {
		builder.WriteString(workflowStepLine(entry.index, entry.result))
		builder.WriteString("\n")
		if strings.TrimSpace(entry.result.Message) != "" {
			builder.WriteString(fmt.Sprintf("    %s\n", entry.result.Message))
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}

func displayStepName(step restfile.WorkflowStep) string {
	name := strings.TrimSpace(step.Name)
	if name != "" {
		return name
	}
	return strings.TrimSpace(step.Using)
}

func cloneGRPCResponse(resp *grpcclient.Response) *grpcclient.Response {
	if resp == nil {
		return nil
	}
	headers := make(map[string][]string, len(resp.Headers))
	for key, values := range resp.Headers {
		headers[key] = append([]string(nil), values...)
	}
	trailers := make(map[string][]string, len(resp.Trailers))
	for key, values := range resp.Trailers {
		trailers[key] = append([]string(nil), values...)
	}
	return &grpcclient.Response{
		Message:       resp.Message,
		Headers:       headers,
		Trailers:      trailers,
		StatusCode:    resp.StatusCode,
		StatusMessage: resp.StatusMessage,
		Duration:      resp.Duration,
	}
}

func (m *Model) syncWorkflowList(doc *restfile.Document) bool {
	items, listItems := buildWorkflowItems(doc)
	m.workflowItems = items
	visible := len(listItems) > 0
	if !visible {
		m.workflowList.SetItems(nil)
		m.workflowList.Select(-1)
		m.activeWorkflowKey = ""
		m.setHistoryWorkflow("")
		changed := m.setWorkflowShown(false)
		if m.focus == focusWorkflows {
			m.resetWorkflowFocus(doc)
		}
		return changed
	}
	m.workflowList.SetItems(listItems)
	if !m.selectWorkflowItemByKey(m.activeWorkflowKey) {
		m.workflowList.Select(0)
		if len(m.workflowItems) > 0 {
			m.activeWorkflowKey = workflowKey(m.workflowItems[0].workflow)
		}
	}
	changed := m.setWorkflowShown(true)
	return changed
}

func (m *Model) setWorkflowShown(visible bool) bool {
	if m.showWorkflow == visible {
		return false
	}
	m.showWorkflow = visible
	return true
}

func (m *Model) resetWorkflowFocus(doc *restfile.Document) {
	if doc != nil && len(doc.Requests) > 0 {
		m.setFocus(focusRequests)
		return
	}
	if len(m.fileList.Items()) > 0 {
		m.setFocus(focusFile)
		return
	}
	m.setFocus(focusEditor)
}

func (m *Model) selectWorkflowItemByKey(key string) bool {
	if key == "" {
		return false
	}
	for idx, item := range m.workflowItems {
		if workflowKey(item.workflow) == key {
			m.workflowList.Select(idx)
			return true
		}
	}
	return false
}

func (m *Model) runSelectedWorkflow() tea.Cmd {
	if m.doc == nil {
		m.setStatusMessage(statusMsg{text: "No document loaded", level: statusWarn})
		return nil
	}
	if m.workflowRun != nil {
		m.setStatusMessage(statusMsg{text: "Workflow already running", level: statusWarn})
		return nil
	}
	item, ok := m.workflowList.SelectedItem().(workflowListItem)
	if !ok || item.workflow == nil {
		m.setStatusMessage(statusMsg{text: "No workflow selected", level: statusWarn})
		return nil
	}
	workflowCopy := *item.workflow
	m.setHistoryWorkflow(workflowCopy.Name)
	if key := workflowKey(item.workflow); key != "" {
		m.activeWorkflowKey = key
	}
	return m.startWorkflowRun(m.doc, workflowCopy, m.cfg.HTTPOptions)
}

func (m *Model) recordWorkflowHistory(state *workflowState, summary, report string) {
	if m.historyStore == nil || state == nil {
		return
	}
	workflowName := history.NormalizeWorkflowName(state.workflow.Name)
	entry := history.Entry{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		ExecutedAt:  time.Now(),
		Environment: m.cfg.EnvironmentName,
		RequestName: workflowName,
		Method:      restfile.HistoryMethodWorkflow,
		URL:         workflowName,
		Status:      summary,
		StatusCode:  0,
		Duration:    time.Since(state.start),
		BodySnippet: report,
		RequestText: workflowDefinition(state),
		Description: strings.TrimSpace(state.workflow.Description),
		Tags:        normalizedTags(state.workflow.Tags),
	}
	if entry.RequestName == "" {
		entry.RequestName = "Workflow"
	}
	if err := m.historyStore.Append(entry); err != nil {
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn})
		return
	}
	m.historyWorkflowName = workflowName
	m.historySelectedID = entry.ID
	m.historyJumpToLatest = false
	m.syncHistory()
	m.historyList.Select(0)
}

func (m *Model) updateWorkflowHistoryFilter() {
	name := ""
	if item, ok := m.workflowList.SelectedItem().(workflowListItem); ok && item.workflow != nil {
		name = item.workflow.Name
	}
	m.setHistoryWorkflow(name)
}

func (m *Model) setHistoryWorkflow(name string) {
	trimmed := history.NormalizeWorkflowName(name)
	if m.historyWorkflowName == trimmed {
		return
	}
	m.historyWorkflowName = trimmed
	if m.ready {
		m.syncHistory()
	}
}

func workflowDefinition(state *workflowState) string {
	if state == nil {
		return ""
	}
	var builder strings.Builder
	name := strings.TrimSpace(state.workflow.Name)
	if name == "" {
		name = fmt.Sprintf("workflow-%d", state.start.Unix())
	}
	builder.WriteString("# @workflow ")
	builder.WriteString(name)
	if state.workflow.DefaultOnFailure == restfile.WorkflowOnFailureContinue {
		builder.WriteString(" on-failure=continue")
	}
	for key, value := range state.workflow.Options {
		if strings.HasPrefix(key, "vars.") {
			builder.WriteString(fmt.Sprintf(" %s=%s", key, value))
		}
	}
	builder.WriteString("\n")
	if desc := strings.TrimSpace(state.workflow.Description); desc != "" {
		for _, line := range strings.Split(desc, "\n") {
			builder.WriteString("# @description ")
			builder.WriteString(strings.TrimSpace(line))
			builder.WriteString("\n")
		}
	}
	if len(state.workflow.Tags) > 0 {
		builder.WriteString("# @tag ")
		builder.WriteString(strings.Join(state.workflow.Tags, " "))
		builder.WriteString("\n")
	}
	for _, step := range state.workflow.Steps {
		builder.WriteString("# @step ")
		if strings.TrimSpace(step.Name) != "" {
			builder.WriteString(strings.TrimSpace(step.Name))
			builder.WriteString(" ")
		}
		builder.WriteString("using=")
		builder.WriteString(strings.TrimSpace(step.Using))
		if step.OnFailure != state.workflow.DefaultOnFailure {
			builder.WriteString(" on-failure=")
			builder.WriteString(string(step.OnFailure))
		}
		for key, value := range step.Expect {
			builder.WriteString(" expect.")
			builder.WriteString(key)
			builder.WriteString("=")
			builder.WriteString(value)
		}
		for key, value := range step.Vars {
			builder.WriteString(" vars.")
			builder.WriteString(key)
			builder.WriteString("=")
			builder.WriteString(value)
		}
		for key, value := range step.Options {
			builder.WriteString(" ")
			builder.WriteString(key)
			builder.WriteString("=")
			builder.WriteString(value)
		}
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}
