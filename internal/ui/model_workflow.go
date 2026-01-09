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
	name := strings.TrimSpace(state.workflow.Name)
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
	if iter, total := workflowIterationInfo(state); total > 0 {
		res.Iteration = iter
		res.Total = total
	}
	if state != nil && state.currentBranch != "" {
		res.Branch = state.currentBranch
	}
	return res
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

func workflowStepExtras(
	state *workflowState,
	step restfile.WorkflowStep,
	extra map[string]string,
) map[string]string {
	out := make(map[string]string)
	if state != nil {
		for key, value := range state.vars {
			out[key] = value
		}
	}
	for key, value := range step.Vars {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "vars.") {
			trimmed = "vars." + trimmed
		}
		out[trimmed] = value
		if state != nil && strings.HasPrefix(trimmed, "vars.workflow.") {
			state.vars[trimmed] = value
		}
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

func workflowLoopKeys(state *workflowState, name string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	reqKey := "vars.request." + name
	wfKey := ""
	if state != nil && state.loopVarsWorkflow {
		wfKey = "vars.workflow." + name
	}
	return reqKey, wfKey
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

	label := strings.TrimSpace(requestBaseTitle(req))
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
			key := strings.ToLower(strings.TrimSpace(step.Using))
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
	run = strings.TrimSpace(run)
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
	state *workflowState,
	step restfile.WorkflowStep,
	req *restfile.Request,
	options httpclient.Options,
	extraVars map[string]string,
	extraVals map[string]rts.Value,
) tea.Cmd {
	if state == nil || req == nil {
		return nil
	}
	clone := cloneRequest(req)
	state.current = clone
	state.stepStart = time.Now()

	title := workflowRunDisplayName(state)
	iter, total := workflowIterationInfo(state)
	label := workflowStepLabel(step, state.currentBranch, iter, total)
	message := fmt.Sprintf("%s %d/%d: %s", title, state.index+1, len(state.steps), label)
	m.statusPulseBase = message
	m.setStatusMessage(statusMsg{text: message, level: statusInfo})
	spin := m.startSending()

	cmd := m.executeRequest(state.doc, clone, options, "", extraVals, extraVars)
	pulse := m.startStatusPulse()
	return batchCmds([]tea.Cmd{cmd, pulse, spin})
}

func (m *Model) executeWorkflowRequestStep(
	state *workflowState,
	runtime workflowStepRuntime,
	options httpclient.Options,
) tea.Cmd {
	step := runtime.step
	req := runtime.request
	if req == nil {
		err := fmt.Errorf("workflow step missing request")
		return m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, err.Error(), err),
		)
	}
	state.currentBranch = ""
	extraVars := workflowStepExtras(state, step, nil)
	envName := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	ctx := context.Background()
	v := m.wfVars(state.doc, req, envName, extraVars)

	if step.When != nil {
		shouldRun, reason, err := m.evalCondition(
			ctx,
			state.doc,
			req,
			envName,
			options.BaseDir,
			step.When,
			v,
			nil,
		)
		if err != nil {
			wrapped := errdef.Wrap(errdef.CodeScript, err, "@when")
			m.lastError = wrapped
			errCmd := m.consumeRequestError(wrapped)
			next := m.advanceWorkflow(
				state,
				makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
			)
			return batchCmds([]tea.Cmd{errCmd, next})
		}
		if !shouldRun {
			skipCmd := m.consumeSkippedRequest(reason)
			next := m.advanceWorkflow(
				state,
				makeWorkflowResult(state, step, false, true, reason, nil),
			)
			return batchCmds([]tea.Cmd{skipCmd, next})
		}
	}

	spec, err := workflowForEachSpec(step, req)
	if err != nil {
		wrapped := errdef.Wrap(errdef.CodeScript, err, "@for-each")
		m.lastError = wrapped
		errCmd := m.consumeRequestError(wrapped)
		next := m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
		)
		return batchCmds([]tea.Cmd{errCmd, next})
	}
	if spec != nil {
		items, err := m.evalForEachItems(
			ctx,
			state.doc,
			req,
			envName,
			options.BaseDir,
			*spec,
			v,
			nil,
		)
		if err != nil {
			wrapped := errdef.Wrap(errdef.CodeScript, err, "@for-each")
			m.lastError = wrapped
			errCmd := m.consumeRequestError(wrapped)
			next := m.advanceWorkflow(
				state,
				makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
			)
			return batchCmds([]tea.Cmd{errCmd, next})
		}
		if len(items) == 0 {
			reason := "for-each produced no items"
			skipCmd := m.consumeSkippedRequest(reason)
			next := m.advanceWorkflow(
				state,
				makeWorkflowResult(state, step, false, true, reason, nil),
			)
			return batchCmds([]tea.Cmd{skipCmd, next})
		}

		loopStep := step
		if loopStep.Kind != restfile.WorkflowStepKindForEach {
			loopStep.Kind = restfile.WorkflowStepKindForEach
			loopStep.ForEach = &restfile.WorkflowForEach{
				Expr: spec.Expr,
				Var:  spec.Var,
				Line: spec.Line,
			}
		}
		name := strings.TrimSpace(spec.Var)
		reqVarKey, wfVarKey := workflowLoopKeys(state, name)
		state.loop = &workflowLoopState{
			step:      loopStep,
			request:   req,
			items:     items,
			index:     0,
			varName:   name,
			reqVarKey: reqVarKey,
			wfVarKey:  wfVarKey,
			line:      spec.Line,
		}
		return m.executeWorkflowLoopIteration(state, options)
	}

	return m.executeWorkflowRequest(state, step, req, options, extraVars, nil)
}

func (m *Model) executeWorkflowIfStep(
	state *workflowState,
	step restfile.WorkflowStep,
	options httpclient.Options,
) tea.Cmd {
	if step.If == nil {
		err := fmt.Errorf("workflow @if missing definition")
		return m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, err.Error(), err),
		)
	}
	state.currentBranch = ""
	extraVars := workflowStepExtras(state, step, nil)
	envName := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	ctx := context.Background()
	v := m.wfVars(state.doc, nil, envName, extraVars)

	evalBranch := func(cond string, line int, tag string) (bool, error) {
		cond = strings.TrimSpace(cond)
		if cond == "" {
			return false, fmt.Errorf("%s expression missing", tag)
		}
		pos := m.rtsPosForLine(state.doc, nil, line)
		val, err := m.rtsEvalValue(
			ctx,
			state.doc,
			nil,
			envName,
			options.BaseDir,
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
		wrapped := errdef.Wrap(errdef.CodeScript, err, "@if")
		m.lastError = wrapped
		errCmd := m.consumeRequestError(wrapped)
		next := m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
		)
		return batchCmds([]tea.Cmd{errCmd, next})
	}
	if ok {
		branch = &step.If.Then
	} else {
		for i := range step.If.Elifs {
			el := &step.If.Elifs[i]
			ok, err = evalBranch(el.Cond, el.Line, "@elif")
			if err != nil {
				wrapped := errdef.Wrap(errdef.CodeScript, err, "@elif")
				m.lastError = wrapped
				errCmd := m.consumeRequestError(wrapped)
				next := m.advanceWorkflow(
					state,
					makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
				)
				return batchCmds([]tea.Cmd{errCmd, next})
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
		skipCmd := m.consumeSkippedRequest(reason)
		next := m.advanceWorkflow(state, makeWorkflowResult(state, step, false, true, reason, nil))
		return batchCmds([]tea.Cmd{skipCmd, next})
	}
	if strings.TrimSpace(branch.Fail) != "" {
		message := strings.TrimSpace(branch.Fail)
		next := m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, message, fmt.Errorf("%s", message)),
		)
		return next
	}
	run := strings.TrimSpace(branch.Run)
	if run == "" {
		reason := "no @if run target"
		skipCmd := m.consumeSkippedRequest(reason)
		next := m.advanceWorkflow(state, makeWorkflowResult(state, step, false, true, reason, nil))
		return batchCmds([]tea.Cmd{skipCmd, next})
	}
	req := state.requests[strings.ToLower(run)]
	if req == nil {
		err := fmt.Errorf("request %s not found", run)
		return m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, err.Error(), err),
		)
	}
	state.currentBranch = run
	spec, err := workflowForEachSpec(step, req)
	if err != nil {
		wrapped := errdef.Wrap(errdef.CodeScript, err, "@for-each")
		m.lastError = wrapped
		errCmd := m.consumeRequestError(wrapped)
		next := m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
		)
		return batchCmds([]tea.Cmd{errCmd, next})
	}
	if spec != nil {
		rv := m.wfVars(state.doc, req, envName, extraVars)
		items, err := m.evalForEachItems(
			ctx,
			state.doc,
			req,
			envName,
			options.BaseDir,
			*spec,
			rv,
			nil,
		)
		if err != nil {
			wrapped := errdef.Wrap(errdef.CodeScript, err, "@for-each")
			m.lastError = wrapped
			errCmd := m.consumeRequestError(wrapped)
			next := m.advanceWorkflow(
				state,
				makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
			)
			return batchCmds([]tea.Cmd{errCmd, next})
		}
		if len(items) == 0 {
			reason := "for-each produced no items"
			skipCmd := m.consumeSkippedRequest(reason)
			next := m.advanceWorkflow(
				state,
				makeWorkflowResult(state, step, false, true, reason, nil),
			)
			return batchCmds([]tea.Cmd{skipCmd, next})
		}
		loopStep := step
		loopStep.Kind = restfile.WorkflowStepKindForEach
		loopStep.ForEach = &restfile.WorkflowForEach{
			Expr: spec.Expr,
			Var:  spec.Var,
			Line: spec.Line,
		}
		name := strings.TrimSpace(spec.Var)
		reqVarKey, wfVarKey := workflowLoopKeys(state, name)
		state.loop = &workflowLoopState{
			step:      loopStep,
			request:   req,
			items:     items,
			index:     0,
			varName:   name,
			reqVarKey: reqVarKey,
			wfVarKey:  wfVarKey,
			line:      spec.Line,
		}
		return m.executeWorkflowLoopIteration(state, options)
	}
	return m.executeWorkflowRequest(state, step, req, options, extraVars, nil)
}

func (m *Model) executeWorkflowSwitchStep(
	state *workflowState,
	step restfile.WorkflowStep,
	options httpclient.Options,
) tea.Cmd {
	if step.Switch == nil {
		err := fmt.Errorf("workflow @switch missing definition")
		return m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, err.Error(), err),
		)
	}
	state.currentBranch = ""
	extraVars := workflowStepExtras(state, step, nil)
	envName := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	ctx := context.Background()
	v := m.wfVars(state.doc, nil, envName, extraVars)

	expr := strings.TrimSpace(step.Switch.Expr)
	if expr == "" {
		err := fmt.Errorf("@switch expression missing")
		return m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, err.Error(), err),
		)
	}
	switchPos := m.rtsPosForLine(state.doc, nil, step.Switch.Line)
	switchVal, err := m.rtsEvalValue(
		ctx,
		state.doc,
		nil,
		envName,
		options.BaseDir,
		expr,
		"@switch "+expr,
		switchPos,
		v,
		nil,
	)
	if err != nil {
		wrapped := errdef.Wrap(errdef.CodeScript, err, "@switch")
		m.lastError = wrapped
		errCmd := m.consumeRequestError(wrapped)
		next := m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
		)
		return batchCmds([]tea.Cmd{errCmd, next})
	}

	var selected *restfile.WorkflowSwitchCase
	for i := range step.Switch.Cases {
		c := &step.Switch.Cases[i]
		caseExpr := strings.TrimSpace(c.Expr)
		if caseExpr == "" {
			continue
		}
		casePos := m.rtsPosForLine(state.doc, nil, c.Line)
		caseVal, err := m.rtsEvalValue(
			ctx,
			state.doc,
			nil,
			envName,
			options.BaseDir,
			caseExpr,
			"@case "+caseExpr,
			casePos,
			v,
			nil,
		)
		if err != nil {
			wrapped := errdef.Wrap(errdef.CodeScript, err, "@case")
			m.lastError = wrapped
			errCmd := m.consumeRequestError(wrapped)
			next := m.advanceWorkflow(
				state,
				makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
			)
			return batchCmds([]tea.Cmd{errCmd, next})
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
		skipCmd := m.consumeSkippedRequest(reason)
		next := m.advanceWorkflow(state, makeWorkflowResult(state, step, false, true, reason, nil))
		return batchCmds([]tea.Cmd{skipCmd, next})
	}

	if strings.TrimSpace(selected.Fail) != "" {
		message := strings.TrimSpace(selected.Fail)
		next := m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, message, fmt.Errorf("%s", message)),
		)
		return next
	}
	run := strings.TrimSpace(selected.Run)
	if run == "" {
		reason := "no @switch run target"
		skipCmd := m.consumeSkippedRequest(reason)
		next := m.advanceWorkflow(state, makeWorkflowResult(state, step, false, true, reason, nil))
		return batchCmds([]tea.Cmd{skipCmd, next})
	}
	req := state.requests[strings.ToLower(run)]
	if req == nil {
		err := fmt.Errorf("request %s not found", run)
		return m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, err.Error(), err),
		)
	}
	state.currentBranch = run
	spec, err := workflowForEachSpec(step, req)
	if err != nil {
		wrapped := errdef.Wrap(errdef.CodeScript, err, "@for-each")
		m.lastError = wrapped
		errCmd := m.consumeRequestError(wrapped)
		next := m.advanceWorkflow(
			state,
			makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
		)
		return batchCmds([]tea.Cmd{errCmd, next})
	}
	if spec != nil {
		rv := m.wfVars(state.doc, req, envName, extraVars)
		items, err := m.evalForEachItems(
			ctx,
			state.doc,
			req,
			envName,
			options.BaseDir,
			*spec,
			rv,
			nil,
		)
		if err != nil {
			wrapped := errdef.Wrap(errdef.CodeScript, err, "@for-each")
			m.lastError = wrapped
			errCmd := m.consumeRequestError(wrapped)
			next := m.advanceWorkflow(
				state,
				makeWorkflowResult(state, step, false, false, wrapped.Error(), wrapped),
			)
			return batchCmds([]tea.Cmd{errCmd, next})
		}
		if len(items) == 0 {
			reason := "for-each produced no items"
			skipCmd := m.consumeSkippedRequest(reason)
			next := m.advanceWorkflow(
				state,
				makeWorkflowResult(state, step, false, true, reason, nil),
			)
			return batchCmds([]tea.Cmd{skipCmd, next})
		}
		loopStep := step
		loopStep.Kind = restfile.WorkflowStepKindForEach
		loopStep.ForEach = &restfile.WorkflowForEach{
			Expr: spec.Expr,
			Var:  spec.Var,
			Line: spec.Line,
		}
		name := strings.TrimSpace(spec.Var)
		reqVarKey, wfVarKey := workflowLoopKeys(state, name)
		state.loop = &workflowLoopState{
			step:      loopStep,
			request:   req,
			items:     items,
			index:     0,
			varName:   name,
			reqVarKey: reqVarKey,
			wfVarKey:  wfVarKey,
			line:      spec.Line,
		}
		return m.executeWorkflowLoopIteration(state, options)
	}
	return m.executeWorkflowRequest(state, step, req, options, extraVars, nil)
}

func (m *Model) executeWorkflowLoopIteration(
	state *workflowState,
	options httpclient.Options,
) tea.Cmd {
	loop := state.loop
	if loop == nil {
		return m.executeWorkflowStep()
	}
	envName := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	ctx := context.Background()
	var pending []tea.Cmd

	for loop.index < len(loop.items) {
		item := loop.items[loop.index]
		extraVars := workflowStepExtras(state, loop.step, nil)
		pos := m.rtsPosForLine(state.doc, loop.request, loop.line)
		itemStr, err := m.rtsValueString(ctx, pos, item)
		if err != nil {
			wrapped := errdef.Wrap(errdef.CodeScript, err, "@for-each")
			m.lastError = wrapped
			if cmd := m.consumeRequestError(wrapped); cmd != nil {
				pending = append(pending, cmd)
			}
			res := makeWorkflowResult(state, loop.step, false, false, wrapped.Error(), wrapped)
			state.results = append(state.results, res)
			if loop.step.OnFailure != restfile.WorkflowOnFailureContinue {
				state.loop = nil
				state.currentBranch = ""
				return batchCmds(append(pending, m.finalizeWorkflowRun(state)))
			}
			loop.index++
			continue
		}
		if loop.wfVarKey != "" {
			state.vars[loop.wfVarKey] = itemStr
			extraVars[loop.wfVarKey] = itemStr
		}
		if loop.reqVarKey != "" {
			extraVars[loop.reqVarKey] = itemStr
		}
		extraVals := map[string]rts.Value{loop.varName: item}
		v := m.wfVars(state.doc, loop.request, envName, extraVars)

		if loop.step.When != nil {
			shouldRun, reason, err := m.evalCondition(
				ctx,
				state.doc,
				loop.request,
				envName,
				options.BaseDir,
				loop.step.When,
				v,
				extraVals,
			)
			if err != nil {
				wrapped := errdef.Wrap(errdef.CodeScript, err, "@when")
				m.lastError = wrapped
				if cmd := m.consumeRequestError(wrapped); cmd != nil {
					pending = append(pending, cmd)
				}
				res := makeWorkflowResult(state, loop.step, false, false, wrapped.Error(), wrapped)
				state.results = append(state.results, res)
				if loop.step.OnFailure != restfile.WorkflowOnFailureContinue {
					state.loop = nil
					state.currentBranch = ""
					return batchCmds(append(pending, m.finalizeWorkflowRun(state)))
				}
				loop.index++
				continue
			}
			if !shouldRun {
				if cmd := m.consumeSkippedRequest(reason); cmd != nil {
					pending = append(pending, cmd)
				}
				res := makeWorkflowResult(state, loop.step, false, true, reason, nil)
				state.results = append(state.results, res)
				loop.index++
				continue
			}
		}

		cmd := m.executeWorkflowRequest(
			state,
			loop.step,
			loop.request,
			options,
			extraVars,
			extraVals,
		)
		return batchCmds(append(pending, cmd))
	}

	state.loop = nil
	state.currentBranch = ""
	state.index++
	next := m.executeWorkflowStep()
	return batchCmds(append(pending, next))
}

func (m *Model) handleWorkflowResponse(msg responseMsg) tea.Cmd {
	state := m.workflowRun
	if state == nil {
		return nil
	}
	current := state.current
	state.current = nil
	m.stopSending()

	canceled := state.canceled || isCanceled(msg.err)
	inLoop := state.loop != nil

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
	}

	if !canceled && state.origin == workflowOriginForEach {
		switch {
		case msg.skipped:
			m.recordSkippedHistory(msg.executed, msg.requestText, msg.environment, msg.skipReason)
		case msg.response != nil:
			m.recordHTTPHistory(msg.response, msg.executed, msg.requestText, msg.environment)
		case msg.grpc != nil:
			m.recordGRPCHistory(msg.grpc, msg.executed, msg.requestText, msg.environment)
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
	shouldStop := !result.Skipped && !result.Success &&
		result.Step.OnFailure != restfile.WorkflowOnFailureContinue

	var next tea.Cmd
	if shouldStop {
		state.loop = nil
		state.currentBranch = ""
		next = m.finalizeWorkflowRun(state)
	} else if inLoop && state.loop != nil {
		state.loop.index++
		if state.loop.index >= len(state.loop.items) {
			state.loop = nil
			state.currentBranch = ""
			state.index++
			next = m.executeWorkflowStep()
		} else {
			next = m.executeWorkflowStep()
		}
	} else {
		state.currentBranch = ""
		state.index++
		if state.index >= len(state.steps) {
			next = m.finalizeWorkflowRun(state)
		} else {
			next = m.executeWorkflowStep()
		}
	}
	if next != nil {
		cmds = append(cmds, next)
	}
	return batchCmds(cmds)
}

func evaluateWorkflowStep(state *workflowState, msg responseMsg) workflowStepResult {
	if state == nil {
		return workflowStepResult{
			Success: false,
			Skipped: msg.skipped,
			Message: "workflow state missing",
			Err:     errdef.New(errdef.CodeUI, "workflow state missing"),
		}
	}
	if state.index < 0 || state.index >= len(state.steps) {
		return workflowStepResult{
			Success: false,
			Skipped: msg.skipped,
			Message: "workflow state missing",
			Err:     errdef.New(errdef.CodeUI, "workflow state missing"),
		}
	}
	step := state.steps[state.index].step
	if msg.skipped {
		res := workflowStepResult{
			Step:      step,
			Success:   false,
			Skipped:   true,
			Message:   strings.TrimSpace(msg.skipReason),
			Duration:  0,
			HTTP:      nil,
			GRPC:      nil,
			Tests:     nil,
			ScriptErr: nil,
			Err:       nil,
		}
		if iter, total := workflowIterationInfo(state); total > 0 {
			res.Iteration = iter
			res.Total = total
		}
		if state.currentBranch != "" {
			res.Branch = state.currentBranch
		}
		return res
	}

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
		if strings.TrimSpace(status) == "" ||
			!strings.EqualFold(expected, strings.TrimSpace(status)) {
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

	res := workflowStepResult{
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
	if iter, total := workflowIterationInfo(state); total > 0 {
		res.Iteration = iter
		res.Total = total
	}
	if state.currentBranch != "" {
		res.Branch = state.currentBranch
	}
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
	name := strings.TrimSpace(state.workflow.Name)
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
		if using := strings.TrimSpace(step.Using); using != "" {
			return using
		}
		return "@for-each"
	default:
		return strings.TrimSpace(step.Using)
	}
}

func workflowStepLabel(step restfile.WorkflowStep, branch string, iter, total int) string {
	label := strings.TrimSpace(displayStepName(step))
	if label == "" {
		label = "step"
	}
	branch = strings.TrimSpace(branch)
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
		Description: strings.TrimSpace(state.workflow.Description),
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
	w.builder.WriteString(strings.TrimSpace(block.Then.Cond))
	w.builder.WriteString(w.runFailSuffix(block.Then.Run, block.Then.Fail))
	w.builder.WriteString("\n")
	for _, branch := range block.Elifs {
		w.builder.WriteString("# @elif ")
		w.builder.WriteString(strings.TrimSpace(branch.Cond))
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
	w.builder.WriteString(strings.TrimSpace(block.Expr))
	w.builder.WriteString("\n")
	for _, branch := range block.Cases {
		w.builder.WriteString("# @case ")
		w.builder.WriteString(strings.TrimSpace(branch.Expr))
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
		w.builder.WriteString(strings.TrimSpace(step.When.Expression))
		w.builder.WriteString("\n")
	}
	if step.ForEach != nil {
		w.builder.WriteString("# @for-each ")
		w.builder.WriteString(strings.TrimSpace(step.ForEach.Expr))
		w.builder.WriteString(" as ")
		w.builder.WriteString(strings.TrimSpace(step.ForEach.Var))
		w.builder.WriteString("\n")
	}
	w.builder.WriteString("# @step ")
	if strings.TrimSpace(step.Name) != "" {
		w.builder.WriteString(strings.TrimSpace(step.Name))
		w.builder.WriteString(" ")
	}
	w.builder.WriteString("using=")
	w.builder.WriteString(strings.TrimSpace(step.Using))
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
	run = strings.TrimSpace(run)
	fail = strings.TrimSpace(fail)
	if run != "" {
		return " run=" + w.formatOption(run)
	}
	if fail != "" {
		return " fail=" + w.formatOption(fail)
	}
	return ""
}

func (w workflowDefinitionWriter) formatOption(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if strings.ContainsAny(value, " \t\"") {
		return strconv.Quote(value)
	}
	return value
}
