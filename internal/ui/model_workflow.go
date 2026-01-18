package ui

import (
	"context"
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
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type workflowState struct {
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

type workflowStepResult struct {
	Step      restfile.WorkflowStep
	Success   bool
	Canceled  bool
	Skipped   bool
	Status    string
	Duration  time.Duration
	Message   string
	Iteration int
	Total     int
	Branch    string
	HTTP      *httpclient.Response
	GRPC      *grpcclient.Response
	Tests     []scripts.TestResult
	ScriptErr error
	Err       error
}

type workflowOrigin int

const (
	workflowOriginWorkflow workflowOrigin = iota
	workflowOriginForEach
)

const (
	workflowStatusPass     = "[PASS]"
	workflowStatusFail     = "[FAIL]"
	workflowStatusCanceled = "[CANCELED]"
	workflowStatusSkipped  = "[SKIPPED]"
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
	name := state.workflow.Name
	if name == "" {
		return label
	}
	return fmt.Sprintf("%s %s", label, name)
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

func workflowApplyVars(st *workflowState, vars map[string]string) {
	if st == nil || len(vars) == 0 {
		return
	}
	if st.vars == nil {
		st.vars = make(map[string]string)
	}
	for key, value := range vars {
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

func (m *Model) wfErr(st *workflowState, step restfile.WorkflowStep, tag string, err error) tea.Cmd {
	wrapped := errdef.Wrap(errdef.CodeScript, err, "%s", tag)
	m.lastError = wrapped
	cmd := m.consumeRequestError(wrapped)
	next := m.advanceWorkflow(
		st,
		makeWorkflowResult(st, step, false, false, wrapped.Error(), wrapped),
	)
	return batchCmds([]tea.Cmd{cmd, next})
}

func (m *Model) wfSkip(st *workflowState, step restfile.WorkflowStep, reason string) tea.Cmd {
	cmd := m.consumeSkippedRequest(reason)
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

	stepRuntimes, lookup, err := m.prepareWorkflowSteps(doc, workflow)
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}

	state := &workflowState{
		doc:              doc,
		options:          options,
		workflow:         workflow,
		steps:            stepRuntimes,
		vars:             make(map[string]string),
		requests:         lookup,
		origin:           workflowOriginWorkflow,
		loopVarsWorkflow: true,
		start:            time.Now(),
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

	label := requestBaseTitle(req)
	step := restfile.WorkflowStep{
		Kind:      restfile.WorkflowStepKindRequest,
		Name:      label,
		OnFailure: restfile.WorkflowOnFailureStop,
		Line:      req.LineRange.Start,
	}
	workflow := restfile.Workflow{
		Name:             label,
		DefaultOnFailure: restfile.WorkflowOnFailureStop,
		Steps:            []restfile.WorkflowStep{step},
	}
	state := &workflowState{
		doc:              doc,
		options:          options,
		workflow:         workflow,
		steps:            []workflowStepRuntime{{step: step, request: req}},
		vars:             make(map[string]string),
		origin:           workflowOriginForEach,
		loopVarsWorkflow: false,
		start:            time.Now(),
	}
	m.workflowRun = state
	m.statusPulseBase = ""
	m.statusPulseFrame = -1

	return m.executeWorkflowStep()
}

func (m *Model) prepareWorkflowSteps(
	doc *restfile.Document,
	workflow restfile.Workflow,
) ([]workflowStepRuntime, map[string]*restfile.Request, error) {
	if len(doc.Requests) == 0 {
		return nil, nil, fmt.Errorf("workflow %s: no requests defined", workflow.Name)
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
		if step.Kind == "" {
			step.Kind = restfile.WorkflowStepKindRequest
		}
		switch step.Kind {
		case restfile.WorkflowStepKindRequest, restfile.WorkflowStepKindForEach:
			key := strings.ToLower(step.Using)
			if key == "" {
				return nil, nil, fmt.Errorf(
					"workflow %s: step %d missing 'using' request",
					workflow.Name,
					idx+1,
				)
			}
			req, ok := lookup[key]
			if !ok {
				return nil, nil, fmt.Errorf(
					"workflow %s: request %s not found",
					workflow.Name,
					step.Using,
				)
			}
			if step.Kind == restfile.WorkflowStepKindForEach && step.ForEach == nil {
				return nil, nil, fmt.Errorf(
					"workflow %s: step %d missing @for-each spec",
					workflow.Name,
					idx+1,
				)
			}
			steps = append(steps, workflowStepRuntime{step: step, request: req})
		case restfile.WorkflowStepKindIf:
			if step.If == nil {
				return nil, nil, fmt.Errorf(
					"workflow %s: step %d missing @if definition",
					workflow.Name,
					idx+1,
				)
			}
			if err := validateWorkflowBranchRuns(
				workflow.Name,
				idx+1,
				lookup,
				step.If.Then.Run,
			); err != nil {
				return nil, nil, err
			}
			for _, branch := range step.If.Elifs {
				if err := validateWorkflowBranchRuns(
					workflow.Name,
					idx+1,
					lookup,
					branch.Run,
				); err != nil {
					return nil, nil, err
				}
			}
			if step.If.Else != nil {
				if err := validateWorkflowBranchRuns(
					workflow.Name,
					idx+1,
					lookup,
					step.If.Else.Run,
				); err != nil {
					return nil, nil, err
				}
			}
			steps = append(steps, workflowStepRuntime{step: step})
		case restfile.WorkflowStepKindSwitch:
			if step.Switch == nil {
				return nil, nil, fmt.Errorf(
					"workflow %s: step %d missing @switch definition",
					workflow.Name,
					idx+1,
				)
			}
			for _, branch := range step.Switch.Cases {
				if err := validateWorkflowBranchRuns(
					workflow.Name,
					idx+1,
					lookup,
					branch.Run,
				); err != nil {
					return nil, nil, err
				}
			}
			if step.Switch.Default != nil {
				if err := validateWorkflowBranchRuns(
					workflow.Name,
					idx+1,
					lookup,
					step.Switch.Default.Run,
				); err != nil {
					return nil, nil, err
				}
			}
			steps = append(steps, workflowStepRuntime{step: step})
		default:
			return nil, nil, fmt.Errorf(
				"workflow %s: step %d has unknown kind %q",
				workflow.Name,
				idx+1,
				step.Kind,
			)
		}
	}
	return steps, lookup, nil
}

func validateWorkflowBranchRuns(
	workflowName string,
	stepIndex int,
	lookup map[string]*restfile.Request,
	run string,
) error {
	if run == "" {
		return nil
	}
	if _, ok := lookup[strings.ToLower(run)]; ok {
		return nil
	}
	return fmt.Errorf("workflow %s: step %d request %s not found", workflowName, stepIndex, run)
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
		reason := "no @if branch matched"
		return m.wfSkip(st, step, reason)
	}
	if branch.Fail != "" {
		message := branch.Fail
		next := m.advanceWorkflow(
			st,
			makeWorkflowResult(st, step, false, false, message, fmt.Errorf("%s", message)),
		)
		return next
	}
	run := branch.Run
	if run == "" {
		reason := "no @if run target"
		return m.wfSkip(st, step, reason)
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
		caseExpr := c.Expr
		if caseExpr == "" {
			continue
		}
		casePos := m.rtsPosForLine(st.doc, nil, c.Line)
		caseVal, err := m.rtsEvalValue(
			ctx,
			st.doc,
			nil,
			env,
			opts.BaseDir,
			caseExpr,
			"@case "+caseExpr,
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
		reason := "no @switch case matched"
		return m.wfSkip(st, step, reason)
	}

	if selected.Fail != "" {
		message := selected.Fail
		next := m.advanceWorkflow(
			st,
			makeWorkflowResult(st, step, false, false, message, fmt.Errorf("%s", message)),
		)
		return next
	}
	run := selected.Run
	if run == "" {
		reason := "no @switch run target"
		return m.wfSkip(st, step, reason)
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
			if cmd := m.consumeRequestError(wrapped); cmd != nil {
				cmds = append(cmds, cmd)
			}
			res := makeWorkflowResult(st, loop.step, false, false, wrapped.Error(), wrapped)
			st.results = append(st.results, res)
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
				if cmd := m.consumeRequestError(wrapped); cmd != nil {
					cmds = append(cmds, cmd)
				}
				res := makeWorkflowResult(st, loop.step, false, false, wrapped.Error(), wrapped)
				st.results = append(st.results, res)
				if loop.step.OnFailure != restfile.WorkflowOnFailureContinue {
					st.loop = nil
					st.currentBranch = ""
					return batchCmds(append(cmds, m.finalizeWorkflowRun(st)))
				}
				loop.index++
				continue
			}
			if !shouldRun {
				if cmd := m.consumeSkippedRequest(reason); cmd != nil {
					cmds = append(cmds, cmd)
				}
				res := makeWorkflowResult(st, loop.step, false, true, reason, nil)
				st.results = append(st.results, res)
				loop.index++
				continue
			}
		}

		cmd := m.executeWorkflowRequest(
			st,
			loop.step,
			loop.request,
			opts,
			xv,
			vals,
		)
		return batchCmds(append(cmds, cmd))
	}

	st.loop = nil
	st.currentBranch = ""
	st.index++
	next := m.executeWorkflowStep()
	return batchCmds(append(cmds, next))
}

func (m *Model) handleWorkflowResponse(msg responseMsg) tea.Cmd {
	st := m.workflowRun
	if st == nil {
		return nil
	}
	cur := st.current
	st.current = nil
	m.stopSending()

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
	st.results = append(st.results, result)
	next := m.wfAdvanceResp(st, result, inLoop)
	if next != nil {
		cmds = append(cmds, next)
	}
	return batchCmds(cmds)
}

func (m *Model) wfConsume(st *workflowState, msg responseMsg) []tea.Cmd {
	var cmds []tea.Cmd
	switch {
	case msg.skipped:
		m.lastError = nil
		if cmd := m.consumeSkippedRequest(msg.skipReason); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case msg.err != nil:
		if cmd := m.consumeRequestError(msg.err); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case msg.response != nil:
		if cmd := m.consumeHTTPResponse(
			msg.response,
			msg.tests,
			msg.scriptErr,
			msg.environment,
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
		); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if st != nil && st.origin == workflowOriginForEach {
		switch {
		case msg.skipped:
			m.recordSkippedHistory(msg.executed, msg.requestText, msg.environment, msg.skipReason)
		case msg.response != nil:
			m.recordHTTPHistory(msg.response, msg.executed, msg.requestText, msg.environment)
		case msg.grpc != nil:
			m.recordGRPCHistory(msg.grpc, msg.executed, msg.requestText, msg.environment)
		}
	}
	return cmds
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
	if st == nil {
		return workflowStepResult{
			Success: false,
			Skipped: rm.skipped,
			Message: "workflow state missing",
			Err:     errdef.New(errdef.CodeUI, "workflow state missing"),
		}
	}
	if st.index < 0 || st.index >= len(st.steps) {
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
			HTTP:      nil,
			GRPC:      nil,
			Tests:     nil,
			ScriptErr: nil,
			Err:       nil,
		}
		wfMeta(st, &res)
		return res
	}

	var (
		status, msg, emsg string
		ok     = true
		dur    = time.Since(st.stepStart)
		http   = cloneHTTPResponse(rm.response)
		grpc   = cloneGRPCResponse(rm.grpc)
		tests  = append([]scripts.TestResult(nil), rm.tests...)
		hasExp = hasStatusExp(step.Expect)
		hasResp = rm.response != nil || rm.grpc != nil
		hasErr  = rm.err != nil
	)
	if hasErr {
		emsg = strings.TrimSpace(errdef.Message(rm.err))
		if emsg == "" {
			emsg = "request failed"
		}
	}

	if rm.response != nil {
		status = rm.response.Status
		if rm.response.Duration > 0 {
			dur = rm.response.Duration
		}
		if rm.response.StatusCode >= 400 && !hasErr && !hasExp {
			ok = false
			msg = fmt.Sprintf("unexpected status code %d", rm.response.StatusCode)
		}
	} else if rm.grpc != nil {
		status = rm.grpc.StatusCode.String()
	} else {
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

	if hasResp && !hasErr {
		if exp, okExp := step.Expect["status"]; okExp {
			expected := strings.TrimSpace(exp)
			trimmedStatus := strings.TrimSpace(status)
			if expected == "" {
				ok = false
				msg = "invalid expected status"
			} else if trimmedStatus == "" ||
				!strings.EqualFold(expected, trimmedStatus) {
				ok = false
				if expected != "" {
					msg = fmt.Sprintf("expected status %s", expected)
				}
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
		Step:      step,
		Success:   ok,
		Status:    status,
		Duration:  dur,
		Message:   msg,
		HTTP:      http,
		GRPC:      grpc,
		Tests:     tests,
		ScriptErr: rm.scriptErr,
		Err:       rm.err,
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
	m.workflowRun = nil
	m.stopSending()
	m.stopStatusPulseIfIdle()
	m.setStatusMessage(statusMsg{text: summary, level: workflowStatusLevel(state)})
	if state == nil || state.origin != workflowOriginForEach {
		m.recordWorkflowHistory(state, summary, report)
	}

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
		return fmt.Sprintf("Workflow %s finished with %d failure(s)", state.workflow.Name, failed)
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

func (m *Model) buildWorkflowReport(state *workflowState) string {
	var builder strings.Builder
	if state == nil {
		return ""
	}
	label := workflowRunLabel(state)
	name := state.workflow.Name
	if name == "" {
		name = label
	}
	builder.WriteString(fmt.Sprintf("%s: %s\n", label, name))
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
	switch step.Kind {
	case restfile.WorkflowStepKindIf:
		return "@if"
	case restfile.WorkflowStepKindSwitch:
		return "@switch"
	case restfile.WorkflowStepKindForEach:
		if step.Using != "" {
			return step.Using
		}
		return "@for-each"
	default:
		return step.Using
	}
}

func workflowStepLabel(step restfile.WorkflowStep, branch string, iter, total int) string {
	label := displayStepName(step)
	if label == "" {
		label = "step"
	}
	if branch != "" {
		label = fmt.Sprintf("%s -> %s", label, branch)
	}
	if iter > 0 && total > 0 {
		label = fmt.Sprintf("%s (%d/%d)", label, iter, total)
	}
	return label
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
		Message:         resp.Message,
		Body:            append([]byte(nil), resp.Body...),
		Wire:            append([]byte(nil), resp.Wire...),
		ContentType:     resp.ContentType,
		WireContentType: resp.WireContentType,
		Headers:         headers,
		Trailers:        trailers,
		StatusCode:      resp.StatusCode,
		StatusMessage:   resp.StatusMessage,
		Duration:        resp.Duration,
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
		_ = m.setFocus(focusRequests)
		return
	}
	if len(m.fileList.Items()) > 0 {
		_ = m.setFocus(focusFile)
		return
	}
	_ = m.setFocus(focusEditor)
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
		FilePath:    m.historyFilePath(),
		Method:      restfile.HistoryMethodWorkflow,
		URL:         workflowName,
		Status:      summary,
		StatusCode:  0,
		Duration:    time.Since(state.start),
		BodySnippet: report,
		RequestText: workflowDefinition(state),
		Description: state.workflow.Description,
		Tags:        normalizedTags(state.workflow.Tags),
	}
	if entry.RequestName == "" {
		entry.RequestName = "Workflow"
	}
	if err := m.historyStore.Append(entry); err != nil {
		m.setStatusMessage(
			statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn},
		)
		return
	}
	m.historySelectedID = entry.ID
	m.historyJumpToLatest = false
	m.setHistoryWorkflow(workflowName)
}

func (m *Model) setHistoryWorkflow(name string) {
	trimmed := history.NormalizeWorkflowName(name)
	if trimmed == "" {
		if m.historyWorkflowName == "" && m.historyScope != historyScopeWorkflow {
			return
		}
		m.historyWorkflowName = ""
		if m.historyScope == historyScopeWorkflow {
			m.historyScope = historyScopeRequest
		}
		if m.ready {
			m.syncHistory()
		}
		return
	}
	if m.historyWorkflowName == trimmed && m.historyScope == historyScopeWorkflow {
		return
	}
	m.historyWorkflowName = trimmed
	m.historyScope = historyScopeWorkflow
	if m.ready {
		m.syncHistory()
	}
}

func workflowDefinition(state *workflowState) string {
	if state == nil {
		return ""
	}
	var builder strings.Builder
	name := state.workflow.Name
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
	if desc := state.workflow.Description; desc != "" {
		for _, line := range strings.Split(desc, "\n") {
			builder.WriteString("# @description ")
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	if len(state.workflow.Tags) > 0 {
		builder.WriteString("# @tag ")
		builder.WriteString(strings.Join(state.workflow.Tags, " "))
		builder.WriteString("\n")
	}
	writer := newWorkflowDefinitionWriter(&builder, state.workflow.DefaultOnFailure)
	for _, step := range state.workflow.Steps {
		writer.appendStep(step)
	}
	return strings.TrimRight(builder.String(), "\n")
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
