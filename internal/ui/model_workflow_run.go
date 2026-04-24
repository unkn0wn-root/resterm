package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type workflowState struct {
	id               string
	core             bool
	doc              *restfile.Document
	options          httpclient.Options
	workflow         restfile.Workflow
	steps            []workflowStepRuntime
	index            int
	vars             map[string]string
	results          []workflowStepResult
	current          *restfile.Request
	requests         map[string]*restfile.Request
	loop             *workflowLoopState
	currentBranch    string
	origin           workflowOrigin
	loopVarsWorkflow bool
	start            time.Time
	end              time.Time
	stepStart        time.Time
	canceled         bool
	cancelReason     string
	pendingExplain   *xplain.Report
	src              *restfile.Request
}

type workflowStepRuntime struct {
	step    restfile.WorkflowStep
	request *restfile.Request
}

type workflowLoopState struct {
	step      restfile.WorkflowStep
	request   *restfile.Request
	items     []rts.Value
	index     int
	varName   string
	reqVarKey string
	wfVarKey  string
	line      int
}

type workflowOrigin int

const (
	workflowOriginWorkflow workflowOrigin = iota
	workflowOriginForEach
)

func workflowIterationInfo(state *workflowState) (int, int) {
	if state == nil || state.loop == nil {
		return 0, 0
	}
	total := len(state.loop.items)
	if total == 0 {
		return 0, 0
	}
	return state.loop.index + 1, total
}

func workflowRunLabel(state *workflowState) string {
	if state != nil && state.origin == workflowOriginForEach {
		return "For-each"
	}
	return "Workflow"
}

func workflowRunDisplayName(state *workflowState) string {
	label := workflowRunLabel(state)
	if state == nil {
		return label
	}
	name := workflowRunSubject(state)
	if name == "" {
		return label
	}
	return fmt.Sprintf("%s %s", label, name)
}

func workflowRunSubject(state *workflowState) string {
	if state == nil {
		return ""
	}
	if state.origin == workflowOriginForEach {
		if req := workflowRunSourceRequest(state); req != nil {
			return requestBaseTitle(req)
		}
	}
	return strings.TrimSpace(state.workflow.Name)
}

func workflowRunSourceRequest(state *workflowState) *restfile.Request {
	if state == nil || state.origin != workflowOriginForEach || len(state.steps) == 0 {
		return nil
	}
	return state.steps[0].request
}

func makeWorkflowResult(
	state *workflowState,
	step restfile.WorkflowStep,
	success bool,
	skipped bool,
	message string,
	err error,
) workflowStepResult {
	res := workflowStepResult{
		Step:    step,
		Success: success,
		Skipped: skipped,
		Message: message,
		Err:     err,
	}
	wfMeta(state, &res)
	return res
}

func wfMeta(st *workflowState, res *workflowStepResult) {
	if st == nil || res == nil {
		return
	}
	if iter, total := workflowIterationInfo(st); total > 0 {
		res.Iteration = iter
		res.Total = total
	}
	if st.currentBranch != "" {
		res.Branch = st.currentBranch
	}
}

func workflowOriginForMode(mode core.Mode) workflowOrigin {
	if mode == core.ModeForEach {
		return workflowOriginForEach
	}
	return workflowOriginWorkflow
}

func workflowStateFromPlan(
	pl *core.WorkflowPlan,
	opts httpclient.Options,
	useCore bool,
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
		id:               strings.TrimSpace(pl.Run.ID),
		core:             useCore,
		doc:              pl.Doc,
		options:          opts,
		workflow:         pl.Workflow,
		steps:            steps,
		vars:             cloneStringMap(pl.Vars),
		requests:         pl.Reqs,
		origin:           workflowOriginForMode(pl.Run.Mode),
		loopVarsWorkflow: pl.WfVars,
		start:            time.Now(),
	}
	if st.vars == nil {
		st.vars = make(map[string]string)
	}
	return st
}

func workflowPlanNeedsUIDrivenRun(pl *core.WorkflowPlan) bool {
	if pl == nil {
		return false
	}
	for _, item := range pl.Steps {
		if requestNeedsUIDrivenRun(item.Req) {
			return true
		}
	}
	for _, req := range pl.Reqs {
		if requestNeedsUIDrivenRun(req) {
			return true
		}
	}
	return false
}

func requestNeedsUIDrivenRun(req *restfile.Request) bool {
	return req != nil && req.WebSocket != nil && len(req.WebSocket.Steps) == 0
}

func workflowForEachSpec(step restfile.WorkflowStep, req *restfile.Request) (*forEachSpec, error) {
	var spec *forEachSpec
	if step.Kind == restfile.WorkflowStepKindForEach {
		if step.ForEach == nil {
			return nil, fmt.Errorf("@for-each spec missing")
		}
		spec = &forEachSpec{Expr: step.ForEach.Expr, Var: step.ForEach.Var, Line: step.ForEach.Line}
	}
	if req != nil && req.Metadata.ForEach != nil {
		if spec != nil {
			return nil, fmt.Errorf("cannot combine workflow @for-each with request @for-each")
		}
		spec = &forEachSpec{
			Expr: req.Metadata.ForEach.Expression,
			Var:  req.Metadata.ForEach.Var,
			Line: req.Metadata.ForEach.Line,
		}
	}
	return spec, nil
}

func workflowStepVars(step restfile.WorkflowStep) map[string]string {
	if len(step.Vars) == 0 {
		return nil
	}
	out := make(map[string]string, len(step.Vars))
	for key, value := range step.Vars {
		if key == "" {
			continue
		}
		if !strings.HasPrefix(key, "vars.") {
			key = "vars." + key
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func workflowApplyVars(st *workflowState, vals map[string]string) {
	if st == nil || len(vals) == 0 {
		return
	}
	if st.vars == nil {
		st.vars = make(map[string]string)
	}
	for key, value := range vals {
		if strings.HasPrefix(key, "vars.workflow.") {
			st.vars[key] = value
		}
	}
}

func workflowStepExtras(
	st *workflowState,
	stepVars map[string]string,
	extra map[string]string,
) map[string]string {
	size := len(stepVars) + len(extra)
	if st != nil {
		size += len(st.vars)
	}
	out := make(map[string]string, size)
	if st != nil {
		for key, value := range st.vars {
			out[key] = value
		}
	}
	for key, value := range stepVars {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func (m *Model) wfVars(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	extra map[string]string,
) map[string]string {
	base := m.collectVariables(doc, req, env)
	if len(extra) == 0 {
		return base
	}
	return mergeVariableMaps(base, extra)
}

func workflowLoopKeys(st *workflowState, name string) (string, string) {
	if name == "" {
		return "", ""
	}
	reqKey := "vars.request." + name
	wfKey := ""
	if st != nil && st.loopVarsWorkflow {
		wfKey = "vars.workflow." + name
	}
	return reqKey, wfKey
}

func (m *Model) wfErr(
	st *workflowState,
	step restfile.WorkflowStep,
	tag string,
	err error,
) tea.Cmd {
	wrapped := errdef.Wrap(errdef.CodeScript, err, "%s", tag)
	m.lastError = wrapped
	cmd := m.consumeRequestError(wrapped, nil)
	next := m.advanceWorkflow(
		st,
		makeWorkflowResult(st, step, false, false, wrapped.Error(), wrapped),
	)
	return batchCmds([]tea.Cmd{cmd, next})
}

func (m *Model) wfSkip(st *workflowState, step restfile.WorkflowStep, reason string) tea.Cmd {
	cmd := m.consumeSkippedRequest(reason, nil)
	next := m.advanceWorkflow(st, makeWorkflowResult(st, step, false, true, reason, nil))
	return batchCmds([]tea.Cmd{cmd, next})
}

func (m *Model) wfRunReq(
	st *workflowState,
	step restfile.WorkflowStep,
	req *restfile.Request,
	opts httpclient.Options,
	env string,
	ctx context.Context,
	xv map[string]string,
) tea.Cmd {
	spec, err := workflowForEachSpec(step, req)
	if err != nil {
		return m.wfErr(st, step, "@for-each", err)
	}
	if spec == nil {
		return m.executeWorkflowRequest(st, step, req, opts, xv, nil)
	}

	v := m.wfVars(st.doc, req, env, xv)
	items, err := m.evalForEachItems(
		ctx,
		st.doc,
		req,
		env,
		opts.BaseDir,
		*spec,
		v,
		nil,
	)
	if err != nil {
		return m.wfErr(st, step, "@for-each", err)
	}
	if len(items) == 0 {
		return m.wfSkip(st, step, "for-each produced no items")
	}

	loopStep := step
	resetSpec := loopStep.Kind != restfile.WorkflowStepKindForEach
	if resetSpec {
		loopStep.Kind = restfile.WorkflowStepKindForEach
	}
	if resetSpec || loopStep.ForEach == nil {
		loopStep.ForEach = &restfile.WorkflowForEach{
			Expr: spec.Expr,
			Var:  spec.Var,
			Line: spec.Line,
		}
	}
	name := spec.Var
	reqKey, wfKey := workflowLoopKeys(st, name)
	st.loop = &workflowLoopState{
		step:      loopStep,
		request:   req,
		items:     items,
		index:     0,
		varName:   name,
		reqVarKey: reqKey,
		wfVarKey:  wfKey,
		line:      spec.Line,
	}
	return m.executeWorkflowLoopIteration(st, opts)
}

func (s *workflowState) matches(req *restfile.Request) bool {
	return s != nil && s.current != nil && req != nil && s.current == req
}

func (m *Model) startWorkflowRun(
	doc *restfile.Document,
	workflow restfile.Workflow,
	options httpclient.Options,
) tea.Cmd {
	if doc == nil {
		m.setStatusMessage(statusMsg{text: "No document loaded", level: statusWarn})
		return nil
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
		m.activeWorkflowKey = key
	}
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}
	env := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	pl, err := core.PrepareWorkflow(doc, workflow, core.RunMeta{
		ID:  fmt.Sprintf("%d", time.Now().UnixNano()),
		Env: env,
	})
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}
	if workflowPlanNeedsUIDrivenRun(pl) {
		return m.startWorkflowUIDrivenState(workflowStateFromPlan(pl, options, false))
	}
	return m.startWorkflowCoreRun(pl, options)
}

func (m *Model) startForEachRun(
	doc *restfile.Document,
	req *restfile.Request,
	options httpclient.Options,
) tea.Cmd {
	if doc == nil || req == nil {
		m.setStatusMessage(statusMsg{text: "No request loaded", level: statusWarn})
		return nil
	}
	if m.workflowRun != nil {
		m.setStatusMessage(statusMsg{text: "Another run is already active", level: statusWarn})
		return nil
	}
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}
	env := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	pl, err := core.PrepareForEach(doc, req, core.RunMeta{
		ID:  fmt.Sprintf("%d", time.Now().UnixNano()),
		Env: env,
	})
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}
	if workflowPlanNeedsUIDrivenRun(pl) {
		return m.startWorkflowUIDrivenState(workflowStateFromPlan(pl, options, false))
	}
	return m.startWorkflowCoreRun(pl, options)
}

func (m *Model) startWorkflowUIDrivenState(st *workflowState) tea.Cmd {
	if st == nil {
		return nil
	}
	m.workflowRun = st
	m.statusPulseBase = ""
	m.statusPulseFrame = -1
	return m.executeWorkflowStep()
}

func (m *Model) startWorkflowCoreRun(pl *core.WorkflowPlan, opts httpclient.Options) tea.Cmd {
	st := workflowStateFromPlan(pl, opts, true)
	if st == nil {
		return nil
	}
	rq := m.requestSvc(opts)
	if rq == nil {
		return nil
	}
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
	if st := m.profileRun; st != nil && st.core {
		if st.id != "" && msg.runID != "" && st.id != msg.runID {
			return nil
		}
		m.sendCancel = nil
		if msg.err != nil && !st.canceled {
			return m.handleRunErr(msg.err)
		}
		return nil
	}
	if st := m.workflowRun; st != nil && st.core {
		if st.id != "" && msg.runID != "" && st.id != msg.runID {
			return nil
		}
		m.sendCancel = nil
		if msg.err != nil && !st.canceled {
			return m.handleRunErr(msg.err)
		}
		return nil
	}
	if st := m.compareRun; st != nil && st.core {
		if st.id != "" && msg.runID != "" && st.id != msg.runID {
			return nil
		}
		m.sendCancel = nil
		if msg.err != nil && !st.canceled {
			return m.handleRunErr(msg.err)
		}
		return nil
	}
	m.sendCancel = nil
	if msg.err != nil {
		return m.handleRunErr(msg.err)
	}
	return nil
}

func (m *Model) handleWorkflowRunEvt(evt core.Evt) tea.Cmd {
	st := m.workflowRun
	if st == nil || !st.core || evt == nil {
		return nil
	}
	meta := core.MetaOf(evt)
	if st.id != "" && meta.Run.ID != "" && st.id != meta.Run.ID {
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
		step, req := workflowRuntimeAt(st, evt.Step.Index)
		if evt.Request != nil {
			req = evt.Request
		}
		st.loop = &workflowLoopState{
			step:    step,
			request: req,
			items:   make([]rts.Value, evt.Step.Total),
			index:   evt.Step.Iter - 1,
		}
		return
	}
	st.loop = nil
}

func (m *Model) handleWorkflowReqStart(st *workflowState, evt core.ReqStart) tea.Cmd {
	if st == nil {
		return nil
	}
	st.current = cloneRequest(evt.Request)
	st.src = st.current
	title := workflowRunDisplayName(st)
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
	step, _ := workflowRuntimeAt(st, evt.Step.Index)
	res := workflowResultFromRun(
		step,
		evt.Step,
		evt.Result,
		workflowStepDuration(st, evt.Meta.At),
	)
	res.Explain = st.pendingExplain
	st.pendingExplain = nil
	res.Src = cloneRequest(st.src)
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

func workflowRuntimeAt(st *workflowState, i int) (restfile.WorkflowStep, *restfile.Request) {
	if st == nil || i < 0 || i >= len(st.steps) {
		return restfile.WorkflowStep{}, nil
	}
	return st.steps[i].step, st.steps[i].request
}

func workflowStepDuration(st *workflowState, at time.Time) time.Duration {
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
		Req:        cloneRequest(res.Executed),
		HTTP:       cloneHTTPResponse(res.Response),
		GRPC:       cloneGRPCResponse(res.GRPC),
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

	hasExp := hasStatusExp(step.Expect)
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
		if exp, okExp := step.Expect["status"]; okExp {
			want := strings.TrimSpace(exp)
			got := strings.TrimSpace(out.Status)
			if want == "" {
				ok = false
				out.Message = "invalid expected status"
			} else if got == "" || !strings.EqualFold(want, got) {
				ok = false
				out.Message = fmt.Sprintf("expected status %s", want)
			}
		}
		if exp, okExp := step.Expect["statuscode"]; okExp {
			want, err := strconv.Atoi(strings.TrimSpace(exp))
			if err != nil {
				ok = false
				out.Message = fmt.Sprintf("invalid expected status code %q", exp)
			} else {
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
	}
	if !hasResp && res.Err == nil {
		ok = false
	}
	out.Success = ok
	return out
}

func (m *Model) executeWorkflowStep() tea.Cmd {
	state := m.workflowRun
	if state == nil {
		return nil
	}
	if state.index >= len(state.steps) {
		return m.finalizeWorkflowRun(state)
	}
	options := state.options
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}
	if state.loop != nil {
		return m.executeWorkflowLoopIteration(state, options)
	}

	runtime := state.steps[state.index]
	step := runtime.step
	if step.Kind == "" {
		step.Kind = restfile.WorkflowStepKindRequest
	}
	switch step.Kind {
	case restfile.WorkflowStepKindIf:
		return m.executeWorkflowIfStep(state, step, options)
	case restfile.WorkflowStepKindSwitch:
		return m.executeWorkflowSwitchStep(state, step, options)
	case restfile.WorkflowStepKindRequest, restfile.WorkflowStepKindForEach:
		return m.executeWorkflowRequestStep(state, runtime, options)
	default:
		err := fmt.Errorf("unknown workflow step kind %q", step.Kind)
		return m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, err.Error(), err),
		)
	}
}

func (m *Model) advanceWorkflow(state *workflowState, result workflowStepResult) tea.Cmd {
	if state == nil {
		return nil
	}
	state.results = append(state.results, result)
	state.currentBranch = ""
	shouldStop := !result.Skipped && !result.Success &&
		result.Step.OnFailure != restfile.WorkflowOnFailureContinue
	state.index++
	if shouldStop || state.index >= len(state.steps) {
		return m.finalizeWorkflowRun(state)
	}
	return m.executeWorkflowStep()
}

func (m *Model) executeWorkflowRequest(
	st *workflowState,
	step restfile.WorkflowStep,
	req *restfile.Request,
	opts httpclient.Options,
	xv map[string]string,
	vals map[string]rts.Value,
) tea.Cmd {
	if st == nil || req == nil {
		return nil
	}
	clone := cloneRequest(req)
	st.current = clone
	st.stepStart = time.Now()

	title := workflowRunDisplayName(st)
	iter, total := workflowIterationInfo(st)
	label := workflowStepLabel(step, st.currentBranch, iter, total)
	message := fmt.Sprintf("%s %d/%d: %s", title, st.index+1, len(st.steps), label)
	m.statusPulseBase = message
	m.setStatusMessage(statusMsg{text: message, level: statusInfo})
	spin := m.startSending()

	cmd := m.executeRequest(st.doc, clone, opts, "", vals, xv)
	pulse := m.startStatusPulse()
	return batchCmds([]tea.Cmd{cmd, pulse, spin})
}

func (m *Model) executeWorkflowRequestStep(
	st *workflowState,
	rt workflowStepRuntime,
	opts httpclient.Options,
) tea.Cmd {
	step := rt.step
	req := rt.request
	if req == nil {
		err := fmt.Errorf("workflow step missing request")
		return m.advanceWorkflow(
			st,
			makeWorkflowResult(st, step, false, false, err.Error(), err),
		)
	}
	st.currentBranch = ""
	stepVars := workflowStepVars(step)
	workflowApplyVars(st, stepVars)
	xv := workflowStepExtras(st, stepVars, nil)
	env := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	ctx := context.Background()
	v := m.wfVars(st.doc, req, env, xv)

	if step.When != nil {
		shouldRun, reason, err := m.evalCondition(
			ctx,
			st.doc,
			req,
			env,
			opts.BaseDir,
			step.When,
			v,
			nil,
		)
		if err != nil {
			return m.wfErr(st, step, "@when", err)
		}
		if !shouldRun {
			return m.wfSkip(st, step, reason)
		}
	}

	return m.wfRunReq(st, step, req, opts, env, ctx, xv)
}

func (m *Model) executeWorkflowIfStep(
	st *workflowState,
	step restfile.WorkflowStep,
	opts httpclient.Options,
) tea.Cmd {
	if step.If == nil {
		err := fmt.Errorf("workflow @if missing definition")
		return m.advanceWorkflow(
			st,
			makeWorkflowResult(st, step, false, false, err.Error(), err),
		)
	}
	st.currentBranch = ""
	stepVars := workflowStepVars(step)
	workflowApplyVars(st, stepVars)
	xv := workflowStepExtras(st, stepVars, nil)
	env := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	ctx := context.Background()
	v := m.wfVars(st.doc, nil, env, xv)

	evalBranch := func(cond string, line int, tag string) (bool, error) {
		if cond == "" {
			return false, fmt.Errorf("%s expression missing", tag)
		}
		pos := m.rtsPosForLine(st.doc, nil, line)
		val, err := m.rtsEvalValue(
			ctx,
			st.doc,
			nil,
			env,
			opts.BaseDir,
			cond,
			tag+" "+cond,
			pos,
			v,
			nil,
		)
		if err != nil {
			return false, err
		}
		return val.IsTruthy(), nil
	}

	var branch *restfile.WorkflowIfBranch
	ok, err := evalBranch(step.If.Then.Cond, step.If.Then.Line, "@if")
	if err != nil {
		return m.wfErr(st, step, "@if", err)
	}
	if ok {
		branch = &step.If.Then
	} else {
		for i := range step.If.Elifs {
			el := &step.If.Elifs[i]
			ok, err = evalBranch(el.Cond, el.Line, "@elif")
			if err != nil {
				return m.wfErr(st, step, "@elif", err)
			}
			if ok {
				branch = el
				break
			}
		}
	}
	if branch == nil && step.If.Else != nil {
		branch = step.If.Else
	}
	if branch == nil {
		return m.wfSkip(st, step, "no @if branch matched")
	}
	if branch.Fail != "" {
		return m.advanceWorkflow(
			st,
			makeWorkflowResult(st, step, false, false, branch.Fail, fmt.Errorf("%s", branch.Fail)),
		)
	}
	run := branch.Run
	if run == "" {
		return m.wfSkip(st, step, "no @if run target")
	}
	req := st.requests[strings.ToLower(run)]
	if req == nil {
		err := fmt.Errorf("request %s not found", run)
		return m.advanceWorkflow(
			st,
			makeWorkflowResult(st, step, false, false, err.Error(), err),
		)
	}
	st.currentBranch = run
	return m.wfRunReq(st, step, req, opts, env, ctx, xv)
}

func (m *Model) executeWorkflowSwitchStep(
	st *workflowState,
	step restfile.WorkflowStep,
	opts httpclient.Options,
) tea.Cmd {
	if step.Switch == nil {
		err := fmt.Errorf("workflow @switch missing definition")
		return m.advanceWorkflow(
			st,
			makeWorkflowResult(st, step, false, false, err.Error(), err),
		)
	}
	st.currentBranch = ""
	stepVars := workflowStepVars(step)
	workflowApplyVars(st, stepVars)
	xv := workflowStepExtras(st, stepVars, nil)
	env := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	ctx := context.Background()
	v := m.wfVars(st.doc, nil, env, xv)

	expr := step.Switch.Expr
	if expr == "" {
		err := fmt.Errorf("@switch expression missing")
		return m.advanceWorkflow(
			st,
			makeWorkflowResult(st, step, false, false, err.Error(), err),
		)
	}
	switchPos := m.rtsPosForLine(st.doc, nil, step.Switch.Line)
	switchVal, err := m.rtsEvalValue(
		ctx,
		st.doc,
		nil,
		env,
		opts.BaseDir,
		expr,
		"@switch "+expr,
		switchPos,
		v,
		nil,
	)
	if err != nil {
		return m.wfErr(st, step, "@switch", err)
	}

	var selected *restfile.WorkflowSwitchCase
	for i := range step.Switch.Cases {
		c := &step.Switch.Cases[i]
		if c.Expr == "" {
			continue
		}
		casePos := m.rtsPosForLine(st.doc, nil, c.Line)
		caseVal, err := m.rtsEvalValue(
			ctx,
			st.doc,
			nil,
			env,
			opts.BaseDir,
			c.Expr,
			"@case "+c.Expr,
			casePos,
			v,
			nil,
		)
		if err != nil {
			return m.wfErr(st, step, "@case", err)
		}
		if rts.ValueEqual(switchVal, caseVal) {
			selected = c
			break
		}
	}
	if selected == nil {
		selected = step.Switch.Default
	}
	if selected == nil {
		return m.wfSkip(st, step, "no @switch case matched")
	}
	if selected.Fail != "" {
		return m.advanceWorkflow(
			st,
			makeWorkflowResult(
				st,
				step,
				false,
				false,
				selected.Fail,
				fmt.Errorf("%s", selected.Fail),
			),
		)
	}
	run := selected.Run
	if run == "" {
		return m.wfSkip(st, step, "no @switch run target")
	}
	req := st.requests[strings.ToLower(run)]
	if req == nil {
		err := fmt.Errorf("request %s not found", run)
		return m.advanceWorkflow(
			st,
			makeWorkflowResult(st, step, false, false, err.Error(), err),
		)
	}
	st.currentBranch = run
	return m.wfRunReq(st, step, req, opts, env, ctx, xv)
}

func (m *Model) executeWorkflowLoopIteration(
	st *workflowState,
	opts httpclient.Options,
) tea.Cmd {
	loop := st.loop
	if loop == nil {
		return m.executeWorkflowStep()
	}
	env := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	ctx := context.Background()
	var cmds []tea.Cmd

	for loop.index < len(loop.items) {
		item := loop.items[loop.index]
		stepVars := workflowStepVars(loop.step)
		workflowApplyVars(st, stepVars)
		xv := workflowStepExtras(st, stepVars, nil)
		pos := m.rtsPosForLine(st.doc, loop.request, loop.line)
		itemStr, err := m.rtsValueString(ctx, pos, item)
		if err != nil {
			wrapped := errdef.Wrap(errdef.CodeScript, err, "@for-each")
			m.lastError = wrapped
			if cmd := m.consumeRequestError(wrapped, nil); cmd != nil {
				cmds = append(cmds, cmd)
			}
			st.results = append(
				st.results,
				makeWorkflowResult(st, loop.step, false, false, wrapped.Error(), wrapped),
			)
			if loop.step.OnFailure != restfile.WorkflowOnFailureContinue {
				st.loop = nil
				st.currentBranch = ""
				return batchCmds(append(cmds, m.finalizeWorkflowRun(st)))
			}
			loop.index++
			continue
		}
		if loop.wfVarKey != "" {
			st.vars[loop.wfVarKey] = itemStr
			xv[loop.wfVarKey] = itemStr
		}
		if loop.reqVarKey != "" {
			xv[loop.reqVarKey] = itemStr
		}
		vals := map[string]rts.Value{loop.varName: item}
		v := m.wfVars(st.doc, loop.request, env, xv)

		if loop.step.When != nil {
			shouldRun, reason, err := m.evalCondition(
				ctx,
				st.doc,
				loop.request,
				env,
				opts.BaseDir,
				loop.step.When,
				v,
				vals,
			)
			if err != nil {
				wrapped := errdef.Wrap(errdef.CodeScript, err, "@when")
				m.lastError = wrapped
				if cmd := m.consumeRequestError(wrapped, nil); cmd != nil {
					cmds = append(cmds, cmd)
				}
				st.results = append(
					st.results,
					makeWorkflowResult(st, loop.step, false, false, wrapped.Error(), wrapped),
				)
				if loop.step.OnFailure != restfile.WorkflowOnFailureContinue {
					st.loop = nil
					st.currentBranch = ""
					return batchCmds(append(cmds, m.finalizeWorkflowRun(st)))
				}
				loop.index++
				continue
			}
			if !shouldRun {
				if cmd := m.consumeSkippedRequest(reason, nil); cmd != nil {
					cmds = append(cmds, cmd)
				}
				st.results = append(
					st.results,
					makeWorkflowResult(st, loop.step, false, true, reason, nil),
				)
				loop.index++
				continue
			}
		}

		cmd := m.executeWorkflowRequest(st, loop.step, loop.request, opts, xv, vals)
		return batchCmds(append(cmds, cmd))
	}

	st.loop = nil
	st.currentBranch = ""
	st.index++
	return batchCmds(append(cmds, m.executeWorkflowStep()))
}

func (m *Model) handleWorkflowUIDrivenResponse(msg responseMsg) tea.Cmd {
	st := m.workflowRun
	if st == nil {
		return nil
	}
	cur := st.current
	st.current = nil

	canceled := st.canceled || isCanceled(msg.err)
	inLoop := st.loop != nil

	if canceled {
		st.canceled = true
		m.lastError = nil
		msg.err = nil
		if strings.TrimSpace(st.cancelReason) == "" {
			st.cancelReason = "Workflow canceled"
		}
		if cur != nil && st.index < len(st.steps) {
			st.index++
		}
	}

	var cmds []tea.Cmd
	if !canceled {
		cmds = append(cmds, m.wfConsume(st, msg)...)
	}

	if canceled {
		if next := m.finalizeWorkflowRun(st); next != nil {
			cmds = append(cmds, next)
		}
		return batchCmds(cmds)
	}

	result := evaluateWorkflowStep(st, msg)
	result.Explain = msg.explain
	result.Src = cloneRequest(cur)
	st.results = append(st.results, result)
	if next := m.wfAdvanceResp(st, result, inLoop); next != nil {
		cmds = append(cmds, next)
	}
	return batchCmds(cmds)
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
				msg.runtimeSecrets...,
			)
		case msg.response != nil:
			m.recordHTTPHistory(
				msg.response,
				msg.executed,
				msg.requestText,
				msg.environment,
				msg.runtimeSecrets...,
			)
		case msg.grpc != nil:
			m.recordGRPCHistory(
				msg.grpc,
				msg.executed,
				msg.requestText,
				msg.environment,
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

func (m *Model) wfAdvanceResp(
	st *workflowState,
	result workflowStepResult,
	inLoop bool,
) tea.Cmd {
	shouldStop := !result.Skipped && !result.Success &&
		result.Step.OnFailure != restfile.WorkflowOnFailureContinue

	if shouldStop {
		st.loop = nil
		st.currentBranch = ""
		return m.finalizeWorkflowRun(st)
	}
	if inLoop && st.loop != nil {
		st.loop.index++
		if st.loop.index >= len(st.loop.items) {
			st.loop = nil
			st.currentBranch = ""
			st.index++
			return m.executeWorkflowStep()
		}
		return m.executeWorkflowStep()
	}
	st.currentBranch = ""
	st.index++
	if st.index >= len(st.steps) {
		return m.finalizeWorkflowRun(st)
	}
	return m.executeWorkflowStep()
}

func hasStatusExp(exp map[string]string) bool {
	if len(exp) == 0 {
		return false
	}
	if _, ok := exp["status"]; ok {
		return true
	}
	if _, ok := exp["statuscode"]; ok {
		return true
	}
	return false
}

func evaluateWorkflowStep(st *workflowState, rm responseMsg) workflowStepResult {
	if st == nil || st.index < 0 || st.index >= len(st.steps) {
		return workflowStepResult{
			Success: false,
			Skipped: rm.skipped,
			Message: "workflow state missing",
			Err:     errdef.New(errdef.CodeUI, "workflow state missing"),
		}
	}

	step := st.steps[st.index].step
	if rm.skipped {
		res := workflowStepResult{
			Step:      step,
			Success:   false,
			Skipped:   true,
			Message:   strings.TrimSpace(rm.skipReason),
			Duration:  0,
			Tests:     nil,
			ScriptErr: nil,
			Err:       nil,
		}
		wfMeta(st, &res)
		return res
	}

	var (
		status, msg, emsg string
		ok                = true
		dur               = time.Since(st.stepStart)
		http              = cloneHTTPResponse(rm.response)
		grpc              = cloneGRPCResponse(rm.grpc)
		req               = cloneRequest(rm.executed)
		stream            = cloneStreamInfo(rm.stream)
		transcript        = append([]byte(nil), rm.transcript...)
		tests             = append([]scripts.TestResult(nil), rm.tests...)
		hasExp            = hasStatusExp(step.Expect)
		hasResp           = rm.response != nil || rm.grpc != nil || rm.stream != nil ||
			len(rm.transcript) > 0
		hasProtoResp = rm.response != nil || rm.grpc != nil
		hasErr       = rm.err != nil
	)
	if hasErr {
		emsg = rm.err.Error()
		if emsg == "" {
			emsg = "request failed"
		}
	}

	switch {
	case rm.response != nil:
		status = rm.response.Status
		if rm.response.Duration > 0 {
			dur = rm.response.Duration
		}
		if rm.response.StatusCode >= 400 && !hasErr && !hasExp {
			ok = false
			msg = fmt.Sprintf("unexpected status code %d", rm.response.StatusCode)
		}
	case rm.grpc != nil:
		status = rm.grpc.StatusCode.String()
	case rm.stream != nil || len(rm.transcript) > 0:
		status = strings.TrimSpace(streamSummaryText(rm.stream))
		if status == "" {
			status = "stream completed"
		}
	default:
		if !hasErr {
			ok = false
			msg = "request failed"
		}
	}

	if hasErr {
		ok = false
		status = emsg
		msg = emsg
	}
	if ok && rm.scriptErr != nil {
		ok = false
		msg = rm.scriptErr.Error()
	}
	if ok {
		for _, test := range rm.tests {
			if !test.Passed {
				ok = false
				if strings.TrimSpace(test.Message) != "" {
					msg = test.Message
				} else {
					msg = fmt.Sprintf("test failed: %s", test.Name)
				}
				break
			}
		}
	}

	if hasProtoResp && !hasErr {
		if exp, okExp := step.Expect["status"]; okExp {
			expected := strings.TrimSpace(exp)
			trimmedStatus := strings.TrimSpace(status)
			if expected == "" {
				ok = false
				msg = "invalid expected status"
			} else if trimmedStatus == "" || !strings.EqualFold(expected, trimmedStatus) {
				ok = false
				msg = fmt.Sprintf("expected status %s", expected)
			}
		}
		if exp, okExp := step.Expect["statuscode"]; okExp {
			expectedCode, err := strconv.Atoi(strings.TrimSpace(exp))
			if err != nil {
				msg = fmt.Sprintf("invalid expected status code %q", exp)
				ok = false
			} else {
				actual := 0
				if rm.response != nil {
					actual = rm.response.StatusCode
				}
				if actual != expectedCode {
					ok = false
					msg = fmt.Sprintf("expected status code %d", expectedCode)
				}
			}
		}
	}

	res := workflowStepResult{
		Step:       step,
		Success:    ok,
		Status:     status,
		Duration:   dur,
		Message:    msg,
		Req:        req,
		HTTP:       http,
		GRPC:       grpc,
		Stream:     stream,
		Transcript: transcript,
		Tests:      tests,
		ScriptErr:  rm.scriptErr,
		Err:        rm.err,
	}
	if !hasResp && !hasErr {
		res.Success = false
	}
	wfMeta(st, &res)
	return res
}

func (m *Model) finalizeWorkflowRun(state *workflowState) tea.Cmd {
	if state != nil {
		state.end = time.Now()
	}
	report := m.buildWorkflowReport(state)
	summary := workflowSummary(state)
	statsView := newWorkflowStatsView(state)
	explain := workflowExplainReport(state)
	m.workflowRun = nil
	m.stopSending()
	m.stopStatusPulseIfIdle()
	m.setStatusMessage(statusMsg{text: summary, level: workflowStatusLevel(state)})
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

func workflowSummary(state *workflowState) string {
	if state == nil {
		return "Workflow complete"
	}
	title := workflowRunDisplayName(state)
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
		workflowStepLabel(last.Step, last.Branch, last.Iteration, last.Total),
		reason,
	)
}

func workflowStatusLevel(state *workflowState) statusLevel {
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

func workflowExplainReport(state *workflowState) *xplain.Report {
	if state == nil {
		return nil
	}
	title := strings.TrimSpace(workflowRunDisplayName(state))
	if title == "" {
		title = "Workflow"
	}
	entries := buildWorkflowStatsEntries(state)
	rep := &xplain.Report{
		Name:     title,
		URL:      title,
		Env:      workflowExplainEnv(state, entries),
		Status:   workflowExplainStatus(state, entries),
		Decision: workflowSummary(state),
		Failure:  workflowExplainFailure(state, entries),
		Vars:     workflowExplainVars(entries),
		Warnings: workflowExplainWarnings(entries),
	}
	for _, entry := range entries {
		rep.Stages = append(rep.Stages, workflowExplainStages(entry.result)...)
	}
	return rep
}

func workflowExplainEnv(state *workflowState, entries []workflowStatsEntry) string {
	for _, entry := range entries {
		rep := entry.result.Explain
		if rep == nil {
			continue
		}
		if env := strings.TrimSpace(rep.Env); env != "" {
			return env
		}
	}
	return ""
}

func workflowExplainStatus(state *workflowState, entries []workflowStatsEntry) xplain.Status {
	if state != nil && state.canceled {
		return xplain.StatusError
	}
	if len(entries) == 0 {
		return xplain.StatusReady
	}
	allSkipped := true
	for _, entry := range entries {
		result := entry.result
		switch {
		case result.Canceled:
			return xplain.StatusError
		case result.Skipped:
			continue
		case !result.Success:
			return xplain.StatusError
		default:
			allSkipped = false
		}
	}
	if allSkipped {
		return xplain.StatusSkipped
	}
	return xplain.StatusReady
}

func workflowExplainFailure(state *workflowState, entries []workflowStatsEntry) string {
	if state != nil && state.canceled {
		return strings.TrimSpace(state.cancelReason)
	}
	for i := len(entries) - 1; i >= 0; i-- {
		result := entries[i].result
		if result.Canceled {
			if msg := strings.TrimSpace(result.Message); msg != "" {
				return msg
			}
			continue
		}
		if result.Skipped || result.Success {
			continue
		}
		if rep := result.Explain; rep != nil {
			if failure := strings.TrimSpace(rep.Failure); failure != "" {
				return failure
			}
		}
		if result.ScriptErr != nil {
			return result.ScriptErr.Error()
		}
		if result.Err != nil {
			return result.Err.Error()
		}
		if msg := strings.TrimSpace(result.Message); msg != "" {
			return msg
		}
		if status := strings.TrimSpace(result.Status); status != "" {
			return status
		}
	}
	return ""
}

func workflowExplainWarnings(entries []workflowStatsEntry) []string {
	var out []string
	for _, entry := range entries {
		rep := entry.result.Explain
		if rep == nil {
			continue
		}
		label := workflowStepLabel(
			entry.result.Step,
			entry.result.Branch,
			entry.result.Iteration,
			entry.result.Total,
		)
		for _, warn := range rep.Warnings {
			warn = strings.TrimSpace(warn)
			if warn == "" {
				continue
			}
			if label != "" {
				warn = label + ": " + warn
			}
			out = appendWorkflowExplainNote(out, warn)
		}
	}
	return out
}

func workflowExplainVars(entries []workflowStatsEntry) []xplain.Var {
	var (
		out   []xplain.Var
		index = make(map[string]int)
	)
	for _, entry := range entries {
		rep := entry.result.Explain
		if rep == nil {
			continue
		}
		for _, v := range rep.Vars {
			key := normalizedExplainKey(v.Name) + "\x00" + normalizedExplainKey(v.Source)
			if idx, ok := index[key]; ok {
				curr := &out[idx]
				curr.Uses += v.Uses
				curr.Missing = curr.Missing || v.Missing
				curr.Dynamic = curr.Dynamic || v.Dynamic
				if strings.TrimSpace(curr.Value) == "" {
					curr.Value = v.Value
				}
				for _, shadowed := range v.Shadowed {
					if !containsString(curr.Shadowed, shadowed) {
						curr.Shadowed = append(curr.Shadowed, shadowed)
					}
				}
				continue
			}
			copyVar := v
			copyVar.Shadowed = append([]string(nil), v.Shadowed...)
			out = append(out, copyVar)
			index[key] = len(out) - 1
		}
	}
	return out
}

type wfExplainStage struct {
	key string
	st  xplain.Stage
}

func workflowExplainStages(r workflowStepResult) []xplain.Stage {
	lbl := workflowStepLabel(r.Step, r.Branch, r.Iteration, r.Total)
	xs := workflowExplainCloneStages(lbl, r.Explain)
	xs = workflowExplainMergeDiffs(xs, lbl, r)
	if workflowExplainNeedOutcome(r, xs) {
		xs = append(xs, wfExplainStage{st: workflowExplainOutcomeStage(lbl, r)})
	}
	out := make([]xplain.Stage, 0, len(xs))
	for _, x := range xs {
		out = append(out, x.st)
	}
	return out
}

func workflowExplainCloneStages(lbl string, rep *xplain.Report) []wfExplainStage {
	if rep == nil || len(rep.Stages) == 0 {
		return nil
	}
	out := make([]wfExplainStage, 0, len(rep.Stages))
	for _, s := range rep.Stages {
		sum := explainDisplayStageSummary(s)
		ns := append([]string(nil), explainDisplayStageNotes(s)...)
		cs := append([]xplain.Change(nil), s.Changes...)
		out = append(out, wfExplainStage{
			key: explainKey(s.Name),
			st: xplain.Stage{
				Name:    workflowExplainStageName(lbl, s.Name),
				Status:  s.Status,
				Summary: sum,
				Changes: cs,
				Notes:   ns,
			},
		})
	}
	return out
}

func workflowExplainMergeDiffs(
	xs []wfExplainStage,
	lbl string,
	r workflowStepResult,
) []wfExplainStage {
	if r.Src == nil || r.Req == nil {
		return xs
	}
	cs := explainReqChanges(r.Src, r.Req)
	if len(cs) == 0 {
		return xs
	}
	sc, ac, pc := workflowExplainSplitDiffs(r.Src, cs)
	var pre []wfExplainStage
	xs, pre = workflowExplainMergeStage(
		xs,
		pre,
		explainStageSettings,
		workflowExplainStageText(explainStageSettings, explainSummarySettingsMerged),
		lbl,
		sc,
	)
	xs, pre = workflowExplainMergeStage(
		xs,
		pre,
		explainStageAuth,
		workflowExplainStageText(explainStageAuth, explainSummaryAuthPrepared),
		lbl,
		ac,
	)
	k := workflowExplainProtoStageKey(r.Req)
	s := workflowExplainProtoStageSummary(k)
	xs, pre = workflowExplainMergeStage(xs, pre, k, s, lbl, pc)
	return append(pre, xs...)
}

func workflowExplainSplitDiffs(req *restfile.Request, cs []xplain.Change) (
	sc []xplain.Change,
	ac []xplain.Change,
	pc []xplain.Change,
) {
	for _, c := range cs {
		switch {
		case strings.HasPrefix(c.Field, "setting."):
			sc = append(sc, c)
		case workflowExplainIsAuthChange(req, c):
			ac = append(ac, c)
		default:
			pc = append(pc, c)
		}
	}
	return sc, ac, pc
}

func workflowExplainIsAuthChange(req *restfile.Request, c xplain.Change) bool {
	if !strings.HasPrefix(c.Field, "header.") {
		return false
	}
	h := strings.TrimSpace(strings.TrimPrefix(c.Field, "header."))
	if strings.EqualFold(h, "authorization") {
		return true
	}
	if req == nil || req.Metadata.Auth == nil {
		return false
	}
	a := req.Metadata.Auth
	switch strings.ToLower(strings.TrimSpace(a.Type)) {
	case "header":
		return strings.EqualFold(h, strings.TrimSpace(a.Params["header"]))
	case "apikey", "api-key":
		if !strings.EqualFold(strings.TrimSpace(a.Params["placement"]), "header") {
			return false
		}
		n := strings.TrimSpace(a.Params["name"])
		if n == "" {
			n = "X-API-Key"
		}
		return strings.EqualFold(h, n)
	default:
		return false
	}
}

func workflowExplainMergeStage(
	xs []wfExplainStage,
	pre []wfExplainStage,
	key, sum, lbl string,
	cs []xplain.Change,
) ([]wfExplainStage, []wfExplainStage) {
	if len(cs) == 0 {
		return xs, pre
	}
	k := explainKey(key)
	for i := range xs {
		if xs[i].key != k {
			continue
		}
		xs[i].st.Changes = prependExplainChangesUnique(xs[i].st.Changes, cs)
		if strings.TrimSpace(xs[i].st.Summary) == "" {
			xs[i].st.Summary = sum
		}
		return xs, pre
	}
	pre = append(pre, wfExplainStage{
		key: k,
		st: xplain.Stage{
			Name:    workflowExplainStageName(lbl, key),
			Status:  xplain.StageOK,
			Summary: sum,
			Changes: append([]xplain.Change(nil), cs...),
		},
	})
	return xs, pre
}

func workflowExplainStageName(lbl, key string) string {
	name := strings.TrimSpace(explainDisplayStageName(key))
	if name == "" {
		name = strings.TrimSpace(key)
	}
	lbl = strings.TrimSpace(lbl)
	switch {
	case lbl == "":
		return name
	case name == "":
		return lbl
	default:
		return lbl + " / " + name
	}
}

func workflowExplainStageText(key, sum string) string {
	st := xplain.Stage{Name: key, Summary: sum}
	txt := strings.TrimSpace(explainDisplayStageSummary(st))
	if txt != "" {
		return txt
	}
	return strings.TrimSpace(sum)
}

func workflowExplainProtoStageKey(req *restfile.Request) string {
	switch {
	case req != nil && req.GRPC != nil:
		return explainStageGRPCPrepare
	case req != nil && req.WebSocket != nil:
		return explainStageWebSocketPrepare
	default:
		return explainStageHTTPPrepare
	}
}

func workflowExplainProtoStageSummary(key string) string {
	switch explainKey(key) {
	case explainKey(explainStageGRPCPrepare):
		return workflowExplainStageText(explainStageGRPCPrepare, explainSummaryGRPCRequestPrepared)
	case explainKey(explainStageWebSocketPrepare):
		return workflowExplainStageText(
			explainStageWebSocketPrepare,
			explainSummaryWebSocketRequestPrepared,
		)
	default:
		return workflowExplainStageText(explainStageHTTPPrepare, explainSummaryHTTPRequestPrepared)
	}
}

func workflowExplainNeedOutcome(r workflowStepResult, xs []wfExplainStage) bool {
	if len(xs) == 0 {
		return true
	}
	want := workflowExplainOutcomeStatus(r)
	for _, x := range xs {
		if x.st.Status == want {
			return false
		}
	}
	return want != xplain.StageOK
}

func workflowExplainOutcomeStage(lbl string, r workflowStepResult) xplain.Stage {
	sum := workflowExplainOutcome(r)
	return xplain.Stage{
		Name:    strings.TrimSpace(lbl),
		Status:  workflowExplainOutcomeStatus(r),
		Summary: sum,
		Notes:   workflowExplainOutcomeNotes(r, sum),
	}
}

func workflowExplainOutcomeStatus(r workflowStepResult) xplain.StageStatus {
	switch {
	case r.Skipped:
		return xplain.StageSkipped
	case r.Canceled, !r.Success:
		return xplain.StageError
	default:
		return xplain.StageOK
	}
}

func workflowExplainOutcome(r workflowStepResult) string {
	switch {
	case r.Canceled:
		if msg := strings.TrimSpace(r.Message); msg != "" {
			return msg
		}
		return "canceled"
	case r.Skipped:
		if msg := strings.TrimSpace(r.Message); msg != "" {
			return msg
		}
		return "skipped"
	case !r.Success:
		if msg := strings.TrimSpace(r.Message); msg != "" {
			return msg
		}
		if status := strings.TrimSpace(r.Status); status != "" {
			return status
		}
		return "failed"
	default:
		if status := strings.TrimSpace(r.Status); status != "" {
			return status
		}
		if rep := r.Explain; rep != nil {
			if decision := strings.TrimSpace(rep.Decision); decision != "" {
				return decision
			}
		}
		return "completed"
	}
}

func workflowExplainOutcomeNotes(r workflowStepResult, sum string) []string {
	rep := r.Explain
	if rep == nil {
		return nil
	}
	var notes []string
	if decision := strings.TrimSpace(rep.Decision); decision != "" && decision != sum {
		notes = appendWorkflowExplainNote(notes, decision)
	}
	if failure := strings.TrimSpace(rep.Failure); failure != "" {
		notes = appendWorkflowExplainNote(notes, "Failure: "+failure)
	}
	for _, warn := range rep.Warnings {
		warn = strings.TrimSpace(warn)
		if warn == "" {
			continue
		}
		notes = appendWorkflowExplainNote(notes, "Warning: "+warn)
	}
	return notes
}

func appendWorkflowExplainNote(out []string, note string) []string {
	note = strings.TrimSpace(note)
	if note == "" {
		return out
	}
	for _, existing := range out {
		if existing == note {
			return out
		}
	}
	return append(out, note)
}

func containsString(xs []string, want string) bool {
	for _, item := range xs {
		if item == want {
			return true
		}
	}
	return false
}

func prependExplainChangesUnique(dst, src []xplain.Change) []xplain.Change {
	if len(src) == 0 {
		return dst
	}
	out := make([]xplain.Change, 0, len(src)+len(dst))
	out = append(out, src...)
	for _, d := range dst {
		if hasExplainChange(out, d) {
			continue
		}
		out = append(out, d)
	}
	return out
}

func hasExplainChange(xs []xplain.Change, want xplain.Change) bool {
	for _, x := range xs {
		if x.Field == want.Field && x.Before == want.Before && x.After == want.After {
			return true
		}
	}
	return false
}

func (m *Model) buildWorkflowReport(state *workflowState) string {
	if state == nil {
		return ""
	}
	var b strings.Builder
	label := workflowRunLabel(state)
	name := workflowRunSubject(state)
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
		Environment: m.cfg.EnvironmentName,
		RequestName: workflowName,
		FilePath:    m.historyFilePath(),
		Method:      restfile.HistoryMethodWorkflow,
		URL:         workflowName,
		Status:      summary,
		Duration:    time.Since(state.start),
		BodySnippet: report,
		RequestText: workflowDefinition(state),
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

func workflowDefinition(state *workflowState) string {
	if state == nil {
		return ""
	}
	var b strings.Builder
	name := state.workflow.Name
	if name == "" {
		name = fmt.Sprintf("workflow-%d", state.start.Unix())
	}
	b.WriteString("# @workflow ")
	b.WriteString(name)
	if state.workflow.DefaultOnFailure == restfile.WorkflowOnFailureContinue {
		b.WriteString(" on-failure=continue")
	}
	for key, value := range state.workflow.Options {
		if strings.HasPrefix(key, "vars.") {
			fmt.Fprintf(&b, " %s=%s", key, value)
		}
	}
	b.WriteString("\n")
	if desc := state.workflow.Description; desc != "" {
		for _, line := range strings.Split(desc, "\n") {
			b.WriteString("# @description ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	if len(state.workflow.Tags) > 0 {
		b.WriteString("# @tag ")
		b.WriteString(strings.Join(state.workflow.Tags, " "))
		b.WriteString("\n")
	}
	writer := newWorkflowDefinitionWriter(&b, state.workflow.DefaultOnFailure)
	for _, step := range state.workflow.Steps {
		writer.appendStep(step)
	}
	return strings.TrimRight(b.String(), "\n")
}

type workflowDefinitionWriter struct {
	builder          *strings.Builder
	defaultOnFailure restfile.WorkflowFailureMode
}

func newWorkflowDefinitionWriter(
	builder *strings.Builder,
	defaultOnFailure restfile.WorkflowFailureMode,
) workflowDefinitionWriter {
	return workflowDefinitionWriter{builder: builder, defaultOnFailure: defaultOnFailure}
}

func (w workflowDefinitionWriter) appendStep(step restfile.WorkflowStep) {
	if w.builder == nil {
		return
	}
	kind := step.Kind
	if kind == "" {
		kind = restfile.WorkflowStepKindRequest
	}
	switch kind {
	case restfile.WorkflowStepKindIf:
		w.appendIf(step.If)
	case restfile.WorkflowStepKindSwitch:
		w.appendSwitch(step.Switch)
	default:
		w.appendRequest(step)
	}
}

func (w workflowDefinitionWriter) appendIf(block *restfile.WorkflowIf) {
	if w.builder == nil || block == nil {
		return
	}
	w.builder.WriteString("# @if ")
	w.builder.WriteString(block.Then.Cond)
	w.builder.WriteString(w.runFailSuffix(block.Then.Run, block.Then.Fail))
	w.builder.WriteString("\n")
	for _, branch := range block.Elifs {
		w.builder.WriteString("# @elif ")
		w.builder.WriteString(branch.Cond)
		w.builder.WriteString(w.runFailSuffix(branch.Run, branch.Fail))
		w.builder.WriteString("\n")
	}
	if block.Else != nil {
		w.builder.WriteString("# @else")
		w.builder.WriteString(w.runFailSuffix(block.Else.Run, block.Else.Fail))
		w.builder.WriteString("\n")
	}
}

func (w workflowDefinitionWriter) appendSwitch(block *restfile.WorkflowSwitch) {
	if w.builder == nil || block == nil {
		return
	}
	w.builder.WriteString("# @switch ")
	w.builder.WriteString(block.Expr)
	w.builder.WriteString("\n")
	for _, branch := range block.Cases {
		w.builder.WriteString("# @case ")
		w.builder.WriteString(branch.Expr)
		w.builder.WriteString(w.runFailSuffix(branch.Run, branch.Fail))
		w.builder.WriteString("\n")
	}
	if block.Default != nil {
		w.builder.WriteString("# @default")
		w.builder.WriteString(w.runFailSuffix(block.Default.Run, block.Default.Fail))
		w.builder.WriteString("\n")
	}
}

func (w workflowDefinitionWriter) appendRequest(step restfile.WorkflowStep) {
	if w.builder == nil {
		return
	}
	if step.When != nil {
		tag := "@when"
		if step.When.Negate {
			tag = "@skip-if"
		}
		w.builder.WriteString("# ")
		w.builder.WriteString(tag)
		w.builder.WriteString(" ")
		w.builder.WriteString(step.When.Expression)
		w.builder.WriteString("\n")
	}
	if step.ForEach != nil {
		w.builder.WriteString("# @for-each ")
		w.builder.WriteString(step.ForEach.Expr)
		w.builder.WriteString(" as ")
		w.builder.WriteString(step.ForEach.Var)
		w.builder.WriteString("\n")
	}
	w.builder.WriteString("# @step ")
	if strings.TrimSpace(step.Name) != "" {
		w.builder.WriteString(strings.TrimSpace(step.Name))
		w.builder.WriteString(" ")
	}
	w.builder.WriteString("using=")
	w.builder.WriteString(step.Using)
	if step.OnFailure != w.defaultOnFailure {
		w.builder.WriteString(" on-failure=")
		w.builder.WriteString(string(step.OnFailure))
	}
	for key, value := range step.Expect {
		w.builder.WriteString(" expect.")
		w.builder.WriteString(key)
		w.builder.WriteString("=")
		w.builder.WriteString(value)
	}
	for key, value := range step.Vars {
		w.builder.WriteString(" vars.")
		w.builder.WriteString(key)
		w.builder.WriteString("=")
		w.builder.WriteString(value)
	}
	for key, value := range step.Options {
		w.builder.WriteString(" ")
		w.builder.WriteString(key)
		w.builder.WriteString("=")
		w.builder.WriteString(value)
	}
	w.builder.WriteString("\n")
}

func (w workflowDefinitionWriter) runFailSuffix(run, fail string) string {
	if run != "" {
		return " run=" + w.formatOption(run)
	}
	if fail != "" {
		return " fail=" + w.formatOption(fail)
	}
	return ""
}

func (w workflowDefinitionWriter) formatOption(value string) string {
	if value == "" {
		return value
	}
	if strings.ContainsAny(value, " \t\"") {
		return strconv.Quote(value)
	}
	return value
}
