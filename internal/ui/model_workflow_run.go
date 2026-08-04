package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/restwriter"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type workflowState struct {
	id             string
	workflow       restfile.Workflow
	steps          []workflowStepRuntime
	index          int
	results        []workflowStepResult
	current        *restfile.Request
	loop           *workflowLoopState
	currentBranch  string
	origin         workflowOrigin
	env            vars.Environment
	start          time.Time
	end            time.Time
	stepStart      time.Time
	canceled       bool
	cancelReason   string
	latGen         int
	pendingExplain *xplain.Report
	src            *restfile.Request
	// These belong to the document the run was planned from, not to any one step,
	// so they are kept here instead of gathered from step reports.
	warnings []string
}

type workflowStepRuntime struct {
	step    restfile.WorkflowStep
	request *restfile.Request
}

type workflowLoopState struct {
	index int
	total int
}

type workflowOrigin int

const (
	workflowOriginWorkflow workflowOrigin = iota
	workflowOriginForEach
)

func (state *workflowState) runLabel() string {
	if state.origin == workflowOriginForEach {
		return "For-each"
	}
	return "Workflow"
}

func (state *workflowState) runDisplayName() string {
	label := state.runLabel()
	name := state.runSubject()
	if name == "" {
		return label
	}
	return fmt.Sprintf("%s %s", label, name)
}

func (state *workflowState) runSubject() string {
	if state.origin == workflowOriginForEach {
		if req := state.sourceRequest(); req != nil {
			return requestBaseTitle(req)
		}
	}
	return strings.TrimSpace(state.workflow.Name)
}

func (state *workflowState) sourceRequest() *restfile.Request {
	if state.origin != workflowOriginForEach || len(state.steps) == 0 {
		return nil
	}
	return state.steps[0].request
}

func workflowOriginForMode(mode core.Mode) workflowOrigin {
	if mode == core.ModeForEach {
		return workflowOriginForEach
	}
	return workflowOriginWorkflow
}

func workflowStateFromPlan(
	pl *core.WorkflowPlan,
) *workflowState {
	if pl == nil {
		return nil
	}
	steps := make([]workflowStepRuntime, 0, len(pl.Steps))
	for _, item := range pl.Steps {
		steps = append(steps, workflowStepRuntime{
			step:    item.Step,
			request: item.Req,
		})
	}
	st := &workflowState{
		id:       strings.TrimSpace(pl.Run.ID),
		workflow: pl.Workflow,
		steps:    steps,
		origin:   workflowOriginForMode(pl.Run.Mode),
		env:      pl.Run.Env,
		start:    time.Now(),
		warnings: parser.WarningTexts(pl.Doc),
	}
	return st
}

func (m *Model) startWorkflowRun(
	doc *restfile.Document,
	workflow restfile.Workflow,
	options httpx.Options,
) tea.Cmd {
	if cmd := m.runBlocked(); cmd != nil {
		return cmd
	}
	if doc == nil {
		m.setStatusMessage(statusMsg{text: "No document loaded", level: statusWarn})
		return nil
	}
	if err := docErr(doc); err != nil {
		return batchCommands(m.restorePane(paneRegionResponse), m.failErr(err))
	}
	if len(workflow.Steps) == 0 {
		m.setStatusMessage(
			statusMsg{
				text:  fmt.Sprintf("Workflow %s has no steps", workflow.Name),
				level: statusWarn,
			},
		)
		return nil
	}
	if m.workflowRun != nil {
		m.setStatusMessage(
			statusMsg{text: "Another workflow is already running", level: statusWarn},
		)
		return nil
	}
	if key := workflowKey(&workflow); key != "" {
		m.workflowSelectionKey = key
	}
	pl, err := core.PrepareWorkflow(doc, workflow, core.RunMeta{
		ID:  fmt.Sprintf("%d", time.Now().UnixNano()),
		Env: m.ws.active,
	})
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}
	return m.startWorkflowCoreRun(pl, options)
}

func (m *Model) startForEachRun(
	doc *restfile.Document,
	req *restfile.Request,
	options httpx.Options,
) tea.Cmd {
	if cmd := m.runBlocked(); cmd != nil {
		return cmd
	}
	if doc == nil || req == nil {
		m.setStatusMessage(statusMsg{text: "No request loaded", level: statusWarn})
		return nil
	}
	if err := docErr(doc); err != nil {
		return batchCommands(m.restorePane(paneRegionResponse), m.failErr(err))
	}
	if m.workflowRun != nil {
		m.setStatusMessage(statusMsg{text: "Another run is already active", level: statusWarn})
		return nil
	}
	pl, err := core.PrepareForEach(doc, req, core.RunMeta{
		ID:  fmt.Sprintf("%d", time.Now().UnixNano()),
		Env: m.ws.active,
	})
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}
	return m.startWorkflowCoreRun(pl, options)
}

func (m *Model) startWorkflowCoreRun(pl *core.WorkflowPlan, opts httpx.Options) tea.Cmd {
	st := workflowStateFromPlan(pl)
	if st == nil {
		return nil
	}
	rq := m.runRequestSvc(opts)
	if rq == nil {
		return nil
	}
	st.latGen = m.latencySeries.generation()
	m.workflowRun = st
	m.statusPulseBase = ""
	m.statusPulseFrame = -1
	ch := m.runMsgChan
	return m.startRunWorker(st.id, func(ctx context.Context) error {
		return core.RunPlan(ctx, rq, runSink(ch), pl)
	})
}

func (m *Model) handleRunEvt(msg runEvtMsg) tea.Cmd {
	if msg.evt == nil {
		return nil
	}
	switch core.MetaOf(msg.evt).Run.Mode {
	case core.ModeWorkflow, core.ModeForEach:
		return m.handleWorkflowRunEvt(msg.evt)
	case core.ModeCompare:
		return m.handleCompareRunEvt(msg.evt)
	case core.ModeProfile:
		return m.handleProfileRunEvt(msg.evt)
	default:
		return nil
	}
}

func (m *Model) handleRunWorkerDone(msg runWorkerDoneMsg) tea.Cmd {
	var id string
	var canceled bool
	switch {
	case m.profileRun != nil:
		id, canceled = m.profileRun.id, m.profileRun.canceled
	case m.workflowRun != nil:
		id, canceled = m.workflowRun.id, m.workflowRun.canceled
	case m.compareRun != nil:
		id, canceled = m.compareRun.id, m.compareRun.canceled
	}
	if runIDMismatch(id, msg.runID) {
		return nil
	}
	m.sendCancel = nil
	if msg.err != nil && !canceled {
		return m.handleRunErr(msg.err)
	}
	return nil
}

// An empty ID is not enough to reject an event. Only discard it when both IDs
// are known and differ.
func runIDMismatch(id, evtID string) bool {
	return id != "" && evtID != "" && id != evtID
}

func (m *Model) handleWorkflowRunEvt(evt core.Evt) tea.Cmd {
	st := m.workflowRun
	if st == nil || evt == nil {
		return nil
	}
	meta := core.MetaOf(evt)
	if runIDMismatch(st.id, meta.Run.ID) {
		return nil
	}
	switch v := evt.(type) {
	case core.RunStart:
		st.start = v.Meta.At
	case core.WfStepStart:
		m.handleWorkflowStepStart(st, v)
	case core.ReqStart:
		return m.handleWorkflowReqStart(st, v)
	case core.ReqDone:
		return m.handleWorkflowReqDone(st, v)
	case core.WfStepDone:
		m.handleWorkflowStepDone(st, v)
	case core.RunDone:
		return m.handleWorkflowRunDone(st, v)
	}
	return nil
}

func (m *Model) handleWorkflowStepStart(st *workflowState, evt core.WfStepStart) {
	if st == nil {
		return
	}
	st.index = evt.Step.Index
	st.stepStart = evt.Meta.At
	st.current = nil
	st.currentBranch = evt.Step.Branch
	st.pendingExplain = nil
	st.src = nil
	if evt.Step.Iter > 0 && evt.Step.Total > 0 {
		st.loop = &workflowLoopState{
			index: evt.Step.Iter - 1,
			total: evt.Step.Total,
		}
		return
	}
	st.loop = nil
}

func (m *Model) handleWorkflowReqStart(st *workflowState, evt core.ReqStart) tea.Cmd {
	if st == nil {
		return nil
	}
	st.current = evt.Request.Clone()
	st.src = st.current
	title := st.runDisplayName()
	msg := fmt.Sprintf("%s %d/%d: %s", title, st.index+1, len(st.steps), evt.Req.Label)
	m.statusPulseBase = msg
	m.setStatusMessage(statusMsg{text: msg, level: statusInfo})
	spin := m.startSending()
	pulse := m.startStatusPulse()
	return batchCmds([]tea.Cmd{spin, pulse})
}

func (m *Model) handleWorkflowReqDone(st *workflowState, evt core.ReqDone) tea.Cmd {
	if st == nil {
		return nil
	}
	st.current = nil
	msg := m.responseMsgFromRunState(evt.Result, st.origin == workflowOriginForEach)
	msg.latGen = st.latGen
	st.pendingExplain = msg.explain
	m.recordResponseLatency(msg)
	if isCanceled(evt.Result.Err) {
		m.lastError = nil
		return nil
	}
	return batchCmds(m.wfConsume(st, msg))
}

func (m *Model) handleWorkflowStepDone(st *workflowState, evt core.WfStepDone) {
	if st == nil {
		return
	}
	step, _ := st.runtimeAt(evt.Step.Index)
	res := workflowResultFromRun(
		step,
		evt.Step,
		evt.Result,
		st.stepDuration(evt.Meta.At),
	)
	res.Explain = st.pendingExplain
	st.pendingExplain = nil
	res.Src = st.src.Clone()
	st.src = nil
	st.currentBranch = ""
	if evt.Step.Iter <= 0 || evt.Step.Iter >= evt.Step.Total {
		st.loop = nil
	}
	if res.Canceled {
		st.canceled = true
		return
	}
	st.results = append(st.results, res)
}

func (m *Model) handleWorkflowRunDone(st *workflowState, evt core.RunDone) tea.Cmd {
	if st == nil {
		return nil
	}
	st.end = evt.Meta.At
	st.current = nil
	st.currentBranch = ""
	st.loop = nil
	if evt.Canceled {
		st.canceled = true
	}
	m.sendCancel = nil
	m.stopSending()
	return m.finalizeWorkflowRun(st)
}

func (st *workflowState) runtimeAt(i int) (restfile.WorkflowStep, *restfile.Request) {
	if st == nil || i < 0 || i >= len(st.steps) {
		return restfile.WorkflowStep{}, nil
	}
	return st.steps[i].step, st.steps[i].request
}

func (st *workflowState) stepDuration(at time.Time) time.Duration {
	if st == nil || st.stepStart.IsZero() || at.IsZero() || at.Before(st.stepStart) {
		return 0
	}
	return at.Sub(st.stepStart)
}

func workflowResultFromRun(
	step restfile.WorkflowStep,
	meta core.StepMeta,
	res engine.RequestResult,
	dur time.Duration,
) workflowStepResult {
	out := workflowStepResult{
		Step:       step,
		Duration:   dur,
		Iteration:  meta.Iter,
		Total:      meta.Total,
		Branch:     meta.Branch,
		Req:        res.Executed.Clone(),
		HTTP:       cloneHTTPResponse(res.Response),
		GRPC:       res.GRPC.Clone(),
		Stream:     cloneStreamInfo(res.Stream),
		Transcript: append([]byte(nil), res.Transcript...),
		Tests:      append([]scripts.TestResult(nil), res.Tests...),
		ScriptErr:  res.ScriptErr,
		Err:        res.Err,
	}
	if res.Skipped {
		out.Skipped = true
		out.Message = strings.TrimSpace(res.SkipReason)
		return out
	}
	if res.Err != nil && isCanceled(res.Err) {
		out.Canceled = true
		out.Err = nil
		return out
	}

	hasExp := step.Expect.HasStatus()
	hasResp := res.Response != nil || res.GRPC != nil || res.Stream != nil ||
		len(res.Transcript) > 0
	hasProto := res.Response != nil || res.GRPC != nil
	ok := true
	switch {
	case res.Response != nil:
		out.Status = res.Response.Status
		if res.Response.Duration > 0 {
			out.Duration = res.Response.Duration
		}
		if res.Response.StatusCode >= 400 && res.Err == nil && !hasExp {
			ok = false
			out.Message = fmt.Sprintf("unexpected status code %d", res.Response.StatusCode)
		}
	case res.GRPC != nil:
		out.Status = res.GRPC.StatusCode.String()
		if res.GRPC.Duration > 0 {
			out.Duration = res.GRPC.Duration
		}
	case res.Stream != nil || len(res.Transcript) > 0:
		out.Status = strings.TrimSpace(streamSummaryText(res.Stream))
		if out.Status == "" {
			out.Status = "stream completed"
		}
	default:
		if res.Err == nil {
			ok = false
			out.Message = "request failed"
		}
	}

	if res.Err != nil {
		ok = false
		out.Status = res.Err.Error()
		if out.Status == "" {
			out.Status = "request failed"
		}
		out.Message = out.Status
	}
	if ok && res.ScriptErr != nil {
		ok = false
		out.Message = res.ScriptErr.Error()
	}
	if ok {
		for _, test := range res.Tests {
			if !test.Passed {
				ok = false
				if strings.TrimSpace(test.Message) != "" {
					out.Message = test.Message
				} else {
					out.Message = fmt.Sprintf("test failed: %s", test.Name)
				}
				break
			}
		}
	}
	if hasProto && res.Err == nil {
		if step.Expect.Status != "" {
			want := strings.TrimSpace(step.Expect.Status)
			got := strings.TrimSpace(out.Status)
			if got == "" || !strings.EqualFold(want, got) {
				ok = false
				out.Message = fmt.Sprintf("expected status %s", want)
			}
		}
		if step.Expect.StatusCode != nil {
			want := *step.Expect.StatusCode
			got := 0
			switch {
			case res.Response != nil:
				got = res.Response.StatusCode
			case res.GRPC != nil:
				got = int(res.GRPC.StatusCode)
			}
			if got != want {
				ok = false
				out.Message = fmt.Sprintf("expected status code %d", want)
			}
		}
	}
	if !hasResp && res.Err == nil {
		ok = false
	}
	out.Success = ok
	return out
}

func (m *Model) wfConsume(st *workflowState, msg responseMsg) []tea.Cmd {
	var cmds []tea.Cmd
	switch {
	case msg.skipped:
		m.lastError = nil
		if cmd := m.consumeSkippedRequest(msg.skipReason, msg.explain); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case msg.err != nil:
		if cmd := m.consumeRequestError(msg.err, msg.explain); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case msg.response != nil:
		if cmd := m.consumeHTTPResponse(
			msg.response,
			msg.tests,
			msg.scriptErr,
			msg.environment,
			msg.explain,
		); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case msg.grpc != nil:
		if cmd := m.consumeGRPCResponse(
			msg.grpc,
			msg.tests,
			msg.scriptErr,
			msg.executed,
			msg.environment,
			msg.explain,
		); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case msg.stream != nil || len(msg.transcript) > 0:
		m.applyRunSnapshot(newStreamSnapshot(msg.stream, msg.transcript, msg.environment), nil, nil)
	}

	if st != nil && st.origin == workflowOriginForEach {
		if msg.historyDone {
			prev := workflowHistoryCount(m.historyStore())
			m.syncRecordedHistory()
			if workflowHistoryCount(m.historyStore()) > prev {
				return cmds
			}
		}
		switch {
		case msg.skipped:
			m.recordSkippedHistory(
				msg.executed,
				msg.requestText,
				msg.environment,
				msg.skipReason,
				msg.selection,
				msg.runtimeSecrets...,
			)
		case msg.response != nil:
			m.recordHTTPHistory(
				msg.response,
				msg.executed,
				msg.requestText,
				msg.environment,
				msg.selection,
				msg.runtimeSecrets...,
			)
		case msg.grpc != nil:
			m.recordGRPCHistory(
				msg.grpc,
				msg.executed,
				msg.requestText,
				msg.environment,
				msg.selection,
				msg.runtimeSecrets...,
			)
		}
	}
	return cmds
}

func workflowHistoryCount(hs history.Store) int {
	if hs == nil {
		return 0
	}
	es, err := hs.Entries()
	if err != nil {
		return 0
	}
	return len(es)
}

func (m *Model) finalizeWorkflowRun(state *workflowState) tea.Cmd {
	if state != nil {
		state.end = time.Now()
	}
	report := m.buildWorkflowReport(state)
	summary := state.summary()
	statsView := newWorkflowStatsView(state)
	explain := state.explainReport()
	m.workflowRun = nil
	m.stopSending()
	m.stopStatusPulseIfIdle()
	m.setStatusMessage(statusMsg{text: summary, level: state.statusLevel()})
	if state == nil || state.origin != workflowOriginForEach {
		m.recordWorkflowHistory(state, summary, report)
	}

	if m.responseLatest != nil {
		m.responseLatest.explain = explainState{report: explain}
		m.responseLatest.stats = report
		m.responseLatest.statsColored = ""
		m.responseLatest.statsColorize = true
		m.responseLatest.statsKind = statsReportKindWorkflow
		m.responseLatest.workflowStats = statsView
	} else {
		m.responseLatest = &responseSnapshot{
			explain:        explainState{report: explain},
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
		m.responsePending = nil
	}

	var cmd tea.Cmd
	if m.responseLatest != nil && m.responseLatest.workflowStats != nil {
		m.invalidateWorkflowStatsCaches(m.responseLatest)
		cmd = m.activateWorkflowStatsView(m.responseLatest)
	}
	return cmd
}

func (state *workflowState) summary() string {
	if state == nil {
		return "Workflow complete"
	}
	title := state.runDisplayName()
	if state.canceled {
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
		return fmt.Sprintf("%s canceled at step %d/%d", title, step, total)
	}

	succeeded := 0
	skipped := 0
	failed := 0
	for _, result := range state.results {
		if result.Skipped {
			skipped++
			continue
		}
		if result.Success {
			succeeded++
			continue
		}
		failed++
	}
	total := len(state.results)
	if total == 0 {
		total = len(state.steps)
	}
	if failed == 0 {
		if skipped > 0 {
			return fmt.Sprintf("%s completed: %d passed, %d skipped", title, succeeded, skipped)
		}
		return fmt.Sprintf("%s completed: %d/%d steps passed", title, succeeded, total)
	}

	lastFailure := -1
	for idx := len(state.results) - 1; idx >= 0; idx-- {
		if !state.results[idx].Skipped && !state.results[idx].Success {
			lastFailure = idx
			break
		}
	}
	if lastFailure == -1 {
		return fmt.Sprintf("%s finished with %d failure(s)", title, failed)
	}
	if lastFailure < len(state.results)-1 {
		return fmt.Sprintf("%s finished with %d failure(s)", title, failed)
	}
	last := state.results[lastFailure]
	reason := strings.TrimSpace(last.Message)
	if reason == "" {
		reason = "step failed"
	}
	return fmt.Sprintf(
		"%s failed at step %s: %s",
		title,
		core.StepLabel(last.Step, last.Branch, last.Iteration, last.Total),
		reason,
	)
}

func (state *workflowState) statusLevel() statusLevel {
	if state != nil && state.canceled {
		return statusWarn
	}
	for _, result := range state.results {
		if !result.Skipped && !result.Success {
			return statusWarn
		}
	}
	return statusSuccess
}

func (m *Model) buildWorkflowReport(state *workflowState) string {
	if state == nil {
		return ""
	}
	var b strings.Builder
	label := state.runLabel()
	name := state.runSubject()
	if name == "" {
		name = label
	}
	fmt.Fprintf(&b, "%s: %s\n", label, name)
	fmt.Fprintf(&b, "Started: %s\n", state.start.Format(time.RFC3339))
	if !state.end.IsZero() {
		fmt.Fprintf(&b, "Ended: %s\n", state.end.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "Steps: %d\n\n", len(state.steps))
	for _, entry := range buildWorkflowStatsEntries(state) {
		b.WriteString(workflowStepLine(entry.index, entry.result))
		b.WriteString("\n")
		if strings.TrimSpace(entry.result.Message) != "" {
			fmt.Fprintf(&b, "    %s\n", entry.result.Message)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) recordWorkflowHistory(state *workflowState, summary, report string) {
	hs := m.historyStore()
	if hs == nil || state == nil {
		return
	}
	workflowName := history.NormalizeWorkflowName(state.workflow.Name)
	entry := history.Entry{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		ExecutedAt:  time.Now(),
		Environment: state.env.Label(),
		EnvironmentSelection: history.EnvironmentSelection(
			state.env.Selection().Groups(),
		),
		RequestName: workflowName,
		FilePath:    m.historyFilePath(),
		Method:      restfile.HistoryMethodWorkflow,
		URL:         workflowName,
		Status:      summary,
		Duration:    time.Since(state.start),
		BodySnippet: report,
		RequestText: state.definition(),
		Description: state.workflow.Description,
		Tags:        normalizedTags(state.workflow.Tags),
	}
	if entry.RequestName == "" {
		entry.RequestName = "Workflow"
	}
	if err := hs.Append(entry); err != nil {
		m.setStatusMessage(
			statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn},
		)
		return
	}
	m.historySelectedID = entry.ID
	m.setHistoryWorkflow(workflowName)
}

func (state *workflowState) definition() string {
	if state == nil {
		return ""
	}
	fallback := fmt.Sprintf("workflow-%d", state.start.Unix())
	return restwriter.RenderWorkflow(state.workflow, fallback)
}
