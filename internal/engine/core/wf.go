package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

type WorkflowPlan struct {
	Run      RunMeta
	Doc      *restfile.Document
	Workflow restfile.Workflow
	Steps    []WorkflowStepRuntime
	Reqs     map[string]*restfile.Request
	Vars     map[string]string
	WfVars   bool
}

type WorkflowStepRuntime struct {
	Step restfile.WorkflowStep
	Req  *restfile.Request
}

// A finished step lands in exactly one of these states, and only a failure is
// subject to the step's on-failure policy. Failure is the zero value so a state
// that never got set does not read as success.
type stepOutcome int

const (
	stepFailed stepOutcome = iota
	stepPassed
	stepSkipped
	stepCanceled
)

type wfRun struct {
	dep      Dep
	sink     Sink
	pl       *WorkflowPlan
	idx      int
	seq      int
	vars     map[string]string
	done     bool
	seen     bool
	skip     bool
	fail     bool
	canceled bool
}

const (
	wfTagWhen    = "@" + string(directive.When)
	wfTagForEach = "@" + string(directive.ForEach)
	wfTagIf      = "@" + string(directive.If)
	wfTagElif    = "@" + string(directive.Elif)
	wfTagSwitch  = "@" + string(directive.Switch)
	wfTagCase    = "@" + string(directive.Case)

	wfSkipIfNoBranch     = "no @if branch matched"
	wfSkipIfNoRun        = "no @if run target"
	wfSkipSwitchNoCase   = "no @switch case matched"
	wfSkipSwitchNoRun    = "no @switch run target"
	wfSkipForEachNoItems = "for-each produced no items"
)

func PrepareWorkflow(
	doc *restfile.Document,
	wf restfile.Workflow,
	run RunMeta,
) (*WorkflowPlan, error) {
	if doc == nil {
		return nil, fmt.Errorf("no document loaded")
	}
	wf = normWf(wf)
	steps, reqs, err := prepareWorkflow(doc, wf)
	if err != nil {
		return nil, err
	}
	run = normRun(run, ModeWorkflow, wf.Name)
	out := &WorkflowPlan{
		Run:      run,
		Doc:      doc,
		Workflow: wf,
		Steps:    steps,
		Reqs:     reqs,
		Vars:     make(map[string]string),
		WfVars:   true,
	}
	for k, v := range wf.Options {
		if strings.HasPrefix(k, "vars.") {
			out.Vars[k] = v
		}
	}
	return out, nil
}

func PrepareForEach(
	doc *restfile.Document,
	req *restfile.Request,
	run RunMeta,
) (*WorkflowPlan, error) {
	if doc == nil || req == nil {
		return nil, fmt.Errorf("no request loaded")
	}
	name := engine.ReqTitle(req)
	step := restfile.WorkflowStep{
		Kind:      restfile.WorkflowStepKindRequest,
		Name:      name,
		OnFailure: restfile.WorkflowOnFailureStop,
		Line:      req.LineRange.Start,
	}
	run = normRun(run, ModeForEach, name)
	return &WorkflowPlan{
		Run: run,
		Doc: doc,
		Workflow: restfile.Workflow{
			Name:             name,
			DefaultOnFailure: restfile.WorkflowOnFailureStop,
			Steps:            []restfile.WorkflowStep{step},
		},
		Steps: []WorkflowStepRuntime{{
			Step: step,
			Req:  req,
		}},
		Vars:   make(map[string]string),
		WfVars: false,
	}, nil
}

func RunPlan(ctx context.Context, dep Dep, sink Sink, pl *WorkflowPlan) error {
	if dep == nil {
		return fmt.Errorf("workflow dependency is nil")
	}
	if pl == nil {
		return fmt.Errorf("workflow plan is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r := &wfRun{
		dep:  dep,
		sink: sink,
		pl:   pl,
		vars: cloneStrMap(pl.Vars),
		skip: true,
	}
	if r.vars == nil {
		r.vars = make(map[string]string)
	}
	if err := r.emitRunStart(ctx); err != nil {
		return err
	}
	err := r.run(ctx)
	if derr := r.emitRunDone(ctx, err); err == nil {
		err = derr
	}
	return err
}

func (r *wfRun) run(ctx context.Context) error {
	for r.idx < len(r.pl.Steps) {
		if ctx.Err() != nil {
			r.canceled = true
			r.done = true
			return nil
		}
		stop, err := r.runStep(ctx, r.pl.Steps[r.idx])
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}
	r.done = true
	return nil
}

func (r *wfRun) runStep(ctx context.Context, rt WorkflowStepRuntime) (bool, error) {
	step := stepOrDefault(rt.Step)
	switch step.Kind {
	case restfile.WorkflowStepKindIf:
		return r.runIf(ctx, step)
	case restfile.WorkflowStepKindSwitch:
		return r.runSwitch(ctx, step)
	case restfile.WorkflowStepKindRequest, restfile.WorkflowStepKindForEach:
		return r.runReqStep(ctx, step, rt.Req, "")
	default:
		return r.manualFinish(ctx, step, rt.Req, "", engine.RequestResult{
			Err: diag.Newf(diag.ClassUI, "unknown workflow step kind %q", step.Kind),
		})
	}
}

func (r *wfRun) runReqStep(
	ctx context.Context,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
) (bool, error) {
	if ctx.Err() != nil {
		r.canceled = true
		return true, nil
	}
	if req == nil {
		return r.manualFinish(ctx, step, nil, branch, engine.RequestResult{
			Err: diag.New(diag.ClassUI, "workflow step missing request"),
		})
	}

	xv, vv := r.stepScope(step, req, nil)

	if step.When != nil {
		ok, reason, err := r.dep.EvalCondition(
			ctx,
			r.pl.Doc,
			req,
			r.pl.Run.Env,
			baseDir(r.pl.Doc),
			step.When,
			vv,
			nil,
		)
		if err != nil {
			if ctx.Err() != nil {
				r.canceled = true
				r.idx++
				return true, nil
			}
			return r.manualFinish(ctx, step, req, branch, engine.RequestResult{
				Err: diag.WrapAs(diag.ClassScript, err, wfTagWhen),
			})
		}
		if !ok {
			return r.manualFinish(ctx, step, req, branch, engine.RequestResult{Skipped: true, SkipReason: reason})
		}
	}

	spec, err := workflowForEach(step, req)
	if err != nil {
		if ctx.Err() != nil {
			r.canceled = true
			r.idx++
			return true, nil
		}
		return r.manualFinish(ctx, step, req, branch, engine.RequestResult{
			Err: diag.WrapAs(diag.ClassScript, err, wfTagForEach),
		})
	}
	if spec == nil {
		out, err := r.executeStepRequest(ctx, step, req, branch, 0, 0, xv, nil)
		if err != nil {
			return false, err
		}
		return r.finishStep(step, out, true), nil
	}

	items, err := r.dep.EvalForEachItems(
		ctx,
		r.pl.Doc,
		req,
		r.pl.Run.Env,
		baseDir(r.pl.Doc),
		*spec,
		vv,
		nil,
	)
	if err != nil {
		return r.manualFinish(ctx, step, req, branch, engine.RequestResult{
			Err: diag.WrapAs(diag.ClassScript, err, wfTagForEach),
		})
	}
	if len(items) == 0 {
		return r.manualFinish(ctx, step, req, branch, engine.RequestResult{
			Skipped:    true,
			SkipReason: wfSkipForEachNoItems,
		})
	}

	reqKey, wfKey := restfile.WorkflowVarKeys(spec.Var, r.pl.WfVars)
	for i, item := range items {
		itemStr, err := r.dep.ValueString(ctx, r.dep.PosForLine(r.pl.Doc, req, spec.Line), item)
		if err != nil {
			if ctx.Err() != nil {
				r.canceled = true
				r.idx++
				return true, nil
			}
			out, emitErr := r.emitManualStep(
				ctx,
				step,
				req,
				branch,
				i+1,
				len(items),
				engine.RequestResult{Err: diag.WrapAs(diag.ClassScript, err, wfTagForEach)},
			)
			if emitErr != nil {
				return false, emitErr
			}
			if r.finishStep(step, out, false) {
				r.idx++
				return true, nil
			}
			continue
		}

		loopVars := cloneStrMap(xv)
		if loopVars == nil {
			loopVars = make(map[string]string)
		}
		if wfKey != "" {
			r.vars[wfKey] = itemStr
			loopVars[wfKey] = itemStr
		}
		if reqKey != "" {
			loopVars[reqKey] = itemStr
		}
		ev := map[string]rts.Value{spec.Var: item}
		vv := r.dep.CollectVariables(r.pl.Doc, req, r.pl.Run.Env, loopVars)

		if step.When != nil {
			ok, reason, err := r.dep.EvalCondition(
				ctx,
				r.pl.Doc,
				req,
				r.pl.Run.Env,
				baseDir(r.pl.Doc),
				step.When,
				vv,
				ev,
			)
			if err != nil {
				if ctx.Err() != nil {
					r.canceled = true
					r.idx++
					return true, nil
				}
				out, emitErr := r.emitManualStep(
					ctx,
					step,
					req,
					branch,
					i+1,
					len(items),
					engine.RequestResult{Err: diag.WrapAs(diag.ClassScript, err, wfTagWhen)},
				)
				if emitErr != nil {
					return false, emitErr
				}
				if r.finishStep(step, out, false) {
					r.idx++
					return true, nil
				}
				continue
			}
			if !ok {
				// A skipped iteration never ends the loop, so there is no policy
				// to consult and the index stays where it is.
				_, emitErr := r.emitManualStep(
					ctx,
					step,
					req,
					branch,
					i+1,
					len(items),
					engine.RequestResult{Skipped: true, SkipReason: reason},
				)
				if emitErr != nil {
					return false, emitErr
				}
				continue
			}
		}

		out, err := r.executeStepRequest(
			ctx,
			step,
			req,
			branch,
			i+1,
			len(items),
			loopVars,
			ev,
		)
		if err != nil {
			return false, err
		}
		if r.finishStep(step, out, false) {
			r.idx++
			return true, nil
		}
	}

	r.idx++
	return false, nil
}

func (r *wfRun) runIf(ctx context.Context, step restfile.WorkflowStep) (bool, error) {
	if ctx.Err() != nil {
		r.canceled = true
		return true, nil
	}
	if step.If == nil {
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{
			Err: diag.New(diag.ClassUI, "workflow @if missing definition"),
		})
	}

	xv, vv := r.stepScope(step, nil, nil)
	br, err := r.selectIfBranch(ctx, step, vv)
	if err != nil {
		if ctx.Err() != nil {
			r.canceled = true
			return true, nil
		}
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{Err: err})
	}
	if br == nil {
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{Skipped: true, SkipReason: wfSkipIfNoBranch})
	}
	if msg := strings.TrimSpace(br.Fail); msg != "" {
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{Err: fmt.Errorf("%s", msg)})
	}
	branch, req := r.resolveBranchRequest(br.Run)
	if branch == "" {
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{Skipped: true, SkipReason: wfSkipIfNoRun})
	}
	if req == nil {
		return r.manualFinish(ctx, step, nil, branch, engine.RequestResult{
			Err: fmt.Errorf("request %s not found", branch),
		})
	}

	out, err := r.executeStepRequest(ctx, step, req, branch, 0, 0, xv, nil)
	if err != nil {
		return false, err
	}
	return r.finishStep(step, out, true), nil
}

func (r *wfRun) runSwitch(ctx context.Context, step restfile.WorkflowStep) (bool, error) {
	if ctx.Err() != nil {
		r.canceled = true
		return true, nil
	}
	if step.Switch == nil {
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{
			Err: diag.New(diag.ClassUI, "workflow @switch missing definition"),
		})
	}

	xv, vv := r.stepScope(step, nil, nil)
	sel, err := r.selectSwitchCase(ctx, step, vv)
	if err != nil {
		if ctx.Err() != nil {
			r.canceled = true
			return true, nil
		}
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{Err: err})
	}
	if sel == nil {
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{Skipped: true, SkipReason: wfSkipSwitchNoCase})
	}
	if msg := strings.TrimSpace(sel.Fail); msg != "" {
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{Err: fmt.Errorf("%s", msg)})
	}
	branch, req := r.resolveBranchRequest(sel.Run)
	if branch == "" {
		return r.manualFinish(ctx, step, nil, "", engine.RequestResult{Skipped: true, SkipReason: wfSkipSwitchNoRun})
	}
	if req == nil {
		return r.manualFinish(ctx, step, nil, branch, engine.RequestResult{
			Err: fmt.Errorf("request %s not found", branch),
		})
	}

	out, err := r.executeStepRequest(ctx, step, req, branch, 0, 0, xv, nil)
	if err != nil {
		return false, err
	}
	return r.finishStep(step, out, true), nil
}

func (r *wfRun) execReq(
	ctx context.Context,
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
	extra map[string]string,
	vals map[string]rts.Value,
) (engine.RequestResult, error) {
	clone := request.CloneRequest(req)
	if err := r.emitReqStart(ctx, i, step, clone, branch, iter, total); err != nil {
		return engine.RequestResult{}, err
	}
	out, err := r.dep.ExecuteWith(
		r.pl.Doc,
		clone,
		r.pl.Run.Env,
		request.ExecOptions{
			Extra:  extra,
			Values: vals,
			Record: r.pl.Run.Mode == ModeForEach,
			Ctx:    ctx,
		},
	)
	if err != nil {
		return engine.RequestResult{}, err
	}
	if err := r.emitReqDone(ctx, i, step, clone, branch, iter, total, out); err != nil {
		return engine.RequestResult{}, err
	}
	return out, nil
}

func (r *wfRun) stepScope(
	step restfile.WorkflowStep,
	req *restfile.Request,
	extra map[string]string,
) (map[string]string, map[string]string) {
	applyVars(r.vars, step.Vars)
	xv := stepExtras(r.vars, step.Vars, extra)
	vv := r.dep.CollectVariables(r.pl.Doc, req, r.pl.Run.Env, xv)
	return xv, vv
}

func (r *wfRun) emitManualStep(
	ctx context.Context,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
	res engine.RequestResult,
) (stepOutcome, error) {
	if err := r.emitStepStart(ctx, r.idx, step, req, branch, iter, total); err != nil {
		return stepFailed, err
	}
	if err := r.emitStepDone(ctx, r.idx, step, req, branch, iter, total, res); err != nil {
		return stepFailed, err
	}
	out := evalReq(step, res)
	r.note(out)
	return out, nil
}

// manualFinish emits a synthetic result and ends the step in one call. The
// for-each iterations advance the loop themselves, so they use the pieces
// directly.
func (r *wfRun) manualFinish(
	ctx context.Context,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	res engine.RequestResult,
) (bool, error) {
	out, err := r.emitManualStep(ctx, step, req, branch, 0, 0, res)
	if err != nil {
		return false, err
	}
	return r.finishStep(step, out, true), nil
}

func (r *wfRun) executeStepRequest(
	ctx context.Context,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
	extra map[string]string,
	vals map[string]rts.Value,
) (stepOutcome, error) {
	if err := r.emitStepStart(ctx, r.idx, step, req, branch, iter, total); err != nil {
		return stepFailed, err
	}
	res, err := r.execReq(ctx, r.idx, step, req, branch, iter, total, extra, vals)
	if err != nil {
		return stepFailed, err
	}
	if err := r.emitStepDone(ctx, r.idx, step, req, branch, iter, total, res); err != nil {
		return stepFailed, err
	}
	out := evalReq(step, res)
	r.note(out)
	return out, nil
}

// finishStep is the single place that decides whether a run continues, so no
// individual branch can give itself a failure policy of its own. Cancellation is
// not a failure and no policy may override it.
func (r *wfRun) finishStep(step restfile.WorkflowStep, out stepOutcome, advance bool) bool {
	if advance {
		r.idx++
	}
	if out == stepCanceled {
		return true
	}
	return out == stepFailed && step.OnFailure != restfile.WorkflowOnFailureContinue
}

func (r *wfRun) evalStepValue(
	ctx context.Context,
	req *restfile.Request,
	line int,
	tag string,
	expr string,
	vv map[string]string,
	extra map[string]rts.Value,
) (rts.Value, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return rts.Value{}, fmt.Errorf("%s expression missing", tag)
	}
	return r.dep.EvalValue(
		ctx,
		r.pl.Doc,
		req,
		r.pl.Run.Env,
		baseDir(r.pl.Doc),
		expr,
		tag+" "+expr,
		r.dep.PosForLine(r.pl.Doc, req, line),
		vv,
		extra,
	)
}

func (r *wfRun) evalStepBool(
	ctx context.Context,
	req *restfile.Request,
	line int,
	tag string,
	expr string,
	vv map[string]string,
	extra map[string]rts.Value,
) (bool, error) {
	val, err := r.evalStepValue(ctx, req, line, tag, expr, vv, extra)
	if err != nil {
		return false, err
	}
	return val.IsTruthy(), nil
}

func (r *wfRun) resolveBranchRequest(run string) (string, *restfile.Request) {
	branch := strings.TrimSpace(run)
	if branch == "" {
		return "", nil
	}
	return branch, r.pl.Reqs[strings.ToLower(branch)]
}

func (r *wfRun) selectIfBranch(
	ctx context.Context,
	step restfile.WorkflowStep,
	vv map[string]string,
) (*restfile.WorkflowIfBranch, error) {
	ok, err := r.evalStepBool(ctx, nil, step.If.Then.Line, wfTagIf, step.If.Then.Cond, vv, nil)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassScript, err, wfTagIf)
	}
	if ok {
		return &step.If.Then, nil
	}
	for i := range step.If.Elifs {
		item := &step.If.Elifs[i]
		ok, err = r.evalStepBool(ctx, nil, item.Line, wfTagElif, item.Cond, vv, nil)
		if err != nil {
			return nil, diag.WrapAs(diag.ClassScript, err, wfTagElif)
		}
		if ok {
			return item, nil
		}
	}
	if step.If.Else != nil {
		return step.If.Else, nil
	}
	return nil, nil
}

func (r *wfRun) selectSwitchCase(
	ctx context.Context,
	step restfile.WorkflowStep,
	vv map[string]string,
) (*restfile.WorkflowSwitchCase, error) {
	if strings.TrimSpace(step.Switch.Expr) == "" {
		return nil, diag.New(diag.ClassUI, "@switch expression missing")
	}
	base, err := r.evalStepValue(
		ctx,
		nil,
		step.Switch.Line,
		wfTagSwitch,
		step.Switch.Expr,
		vv,
		nil,
	)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassScript, err, wfTagSwitch)
	}
	for i := range step.Switch.Cases {
		item := &step.Switch.Cases[i]
		expr := strings.TrimSpace(item.Expr)
		if expr == "" {
			continue
		}
		val, err := r.evalStepValue(ctx, nil, item.Line, wfTagCase, expr, vv, nil)
		if err != nil {
			return nil, diag.WrapAs(diag.ClassScript, err, wfTagCase)
		}
		if rts.ValueEqual(base, val) {
			return item, nil
		}
	}
	return step.Switch.Default, nil
}

func (r *wfRun) note(out stepOutcome) {
	r.seen = true
	if out == stepSkipped {
		return
	}
	r.skip = false
	if out != stepPassed {
		r.fail = true
	}
	if out == stepCanceled {
		r.canceled = true
	}
}

func (r *wfRun) meta(at time.Time) EvtMeta {
	return NewMeta(r.pl.Run, at)
}

func evalReq(step restfile.WorkflowStep, res engine.RequestResult) stepOutcome {
	if res.Skipped {
		return stepSkipped
	}
	if res.Err != nil {
		if errors.Is(res.Err, context.Canceled) {
			return stepCanceled
		}
		return stepFailed
	}
	if res.ScriptErr != nil {
		return stepFailed
	}
	for _, t := range res.Tests {
		if !t.Passed {
			return stepFailed
		}
	}
	if step.Expect.Status != "" {
		want := strings.TrimSpace(step.Expect.Status)
		switch {
		case res.Response != nil:
			if !strings.EqualFold(want, strings.TrimSpace(res.Response.Status)) {
				return stepFailed
			}
		case res.GRPC != nil:
			if !strings.EqualFold(want, strings.TrimSpace(res.GRPC.StatusCode.String())) {
				return stepFailed
			}
		default:
			return stepFailed
		}
	}
	if step.Expect.StatusCode != nil {
		got := 0
		switch {
		case res.Response != nil:
			got = res.Response.StatusCode
		case res.GRPC != nil:
			got = int(res.GRPC.StatusCode)
		default:
			return stepFailed
		}
		if got != *step.Expect.StatusCode {
			return stepFailed
		}
	}
	switch {
	case res.Response != nil:
		if res.Response.StatusCode >= 400 && !step.Expect.HasStatus() {
			return stepFailed
		}
		return stepPassed
	case res.GRPC != nil, res.Stream != nil, len(res.Transcript) > 0:
		return stepPassed
	default:
		return stepFailed
	}
}

func prepareWorkflow(
	doc *restfile.Document,
	wf restfile.Workflow,
) ([]WorkflowStepRuntime, map[string]*restfile.Request, error) {
	if len(wf.Steps) == 0 {
		return nil, nil, fmt.Errorf("workflow %s has no steps", wf.Name)
	}
	if len(doc.Requests) == 0 {
		return nil, nil, fmt.Errorf("workflow %s: no requests defined", wf.Name)
	}
	reqs := make(map[string]*restfile.Request)
	for _, req := range doc.Requests {
		if name := strings.ToLower(strings.TrimSpace(req.Metadata.Name)); name != "" {
			reqs[name] = req
		}
	}
	out := make([]WorkflowStepRuntime, 0, len(wf.Steps))
	for i, step := range wf.Steps {
		step = stepOrDefault(step)
		switch step.Kind {
		case restfile.WorkflowStepKindRequest, restfile.WorkflowStepKindForEach:
			key := strings.ToLower(strings.TrimSpace(step.Using))
			if key == "" {
				return nil, nil, fmt.Errorf(
					"workflow %s: step %d missing 'using' request",
					wf.Name,
					i+1,
				)
			}
			req := reqs[key]
			if req == nil {
				return nil, nil, fmt.Errorf(
					"workflow %s: request %s not found",
					wf.Name,
					step.Using,
				)
			}
			if step.Kind == restfile.WorkflowStepKindForEach && step.ForEach == nil {
				return nil, nil, fmt.Errorf(
					"workflow %s: step %d missing @for-each spec",
					wf.Name,
					i+1,
				)
			}
			out = append(out, WorkflowStepRuntime{Step: step, Req: req})
		case restfile.WorkflowStepKindIf:
			if step.If == nil {
				return nil, nil, fmt.Errorf(
					"workflow %s: step %d missing @if definition",
					wf.Name,
					i+1,
				)
			}
			if err := validateRun(wf.Name, i+1, reqs, step.If.Then.Run); err != nil {
				return nil, nil, err
			}
			for _, br := range step.If.Elifs {
				if err := validateRun(wf.Name, i+1, reqs, br.Run); err != nil {
					return nil, nil, err
				}
			}
			if step.If.Else != nil {
				if err := validateRun(wf.Name, i+1, reqs, step.If.Else.Run); err != nil {
					return nil, nil, err
				}
			}
			out = append(out, WorkflowStepRuntime{Step: step})
		case restfile.WorkflowStepKindSwitch:
			if step.Switch == nil {
				return nil, nil, fmt.Errorf(
					"workflow %s: step %d missing @switch definition",
					wf.Name,
					i+1,
				)
			}
			for _, br := range step.Switch.Cases {
				if err := validateRun(wf.Name, i+1, reqs, br.Run); err != nil {
					return nil, nil, err
				}
			}
			if step.Switch.Default != nil {
				if err := validateRun(wf.Name, i+1, reqs, step.Switch.Default.Run); err != nil {
					return nil, nil, err
				}
			}
			out = append(out, WorkflowStepRuntime{Step: step})
		default:
			return nil, nil, fmt.Errorf(
				"workflow %s: step %d has unknown kind %q",
				wf.Name,
				i+1,
				step.Kind,
			)
		}
	}
	return out, reqs, nil
}

func validateRun(name string, i int, reqs map[string]*restfile.Request, run string) error {
	if strings.TrimSpace(run) == "" {
		return nil
	}
	if reqs[strings.ToLower(strings.TrimSpace(run))] != nil {
		return nil
	}
	return fmt.Errorf("workflow %s: step %d request %s not found", name, i, run)
}

func workflowForEach(
	step restfile.WorkflowStep,
	req *restfile.Request,
) (*request.ForEachSpec, error) {
	var spec *request.ForEachSpec
	if step.Kind == restfile.WorkflowStepKindForEach {
		if step.ForEach == nil {
			return nil, fmt.Errorf("@for-each spec missing")
		}
		spec = &request.ForEachSpec{
			Expr: step.ForEach.Expr,
			Var:  step.ForEach.Var,
			Line: step.ForEach.Line,
		}
	}
	if req != nil && req.Metadata.ForEach != nil {
		if spec != nil {
			return nil, fmt.Errorf("cannot combine workflow @for-each with request @for-each")
		}
		spec = &request.ForEachSpec{
			Expr: req.Metadata.ForEach.Expression,
			Var:  req.Metadata.ForEach.Var,
			Line: req.Metadata.ForEach.Line,
		}
	}
	return spec, nil
}

func baseDir(doc *restfile.Document) string {
	if doc == nil || strings.TrimSpace(doc.Path) == "" {
		return ""
	}
	return filepath.Dir(doc.Path)
}

func normRun(run RunMeta, mode Mode, name string) RunMeta {
	if run.Mode == ModeUnknown {
		run.Mode = mode
	}
	if strings.TrimSpace(run.Name) == "" {
		run.Name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(run.ID) == "" {
		run.ID = run.Mode.String() + ":" + run.Name + ":" + run.Env.Scope()
	}
	return run
}

func stepOrDefault(step restfile.WorkflowStep) restfile.WorkflowStep {
	if step.Kind == "" {
		step.Kind = restfile.WorkflowStepKindRequest
	}
	return step
}

func normWf(wf restfile.Workflow) restfile.Workflow {
	wf.Name = strings.TrimSpace(wf.Name)
	wf.Tags = engine.Tags(wf.Tags)
	for i := range wf.Steps {
		normWfStep(&wf.Steps[i], wf.DefaultOnFailure)
	}
	return wf
}

// The parser already resolves the workflow default, but a plan can be built
// without it, and finishStep only ever looks at the step.
func normWfStep(step *restfile.WorkflowStep, fail restfile.WorkflowFailureMode) {
	step.Name = strings.TrimSpace(step.Name)
	step.Using = strings.TrimSpace(step.Using)
	if step.OnFailure == "" {
		step.OnFailure = fail
	}
}

func applyVars(dst map[string]string, vals map[string]string) {
	if len(vals) == 0 {
		return
	}
	if dst == nil {
		return
	}
	for k, v := range vals {
		if restfile.IsWorkflowScopedVar(k) {
			dst[k] = v
		}
	}
}

func stepExtras(
	base map[string]string,
	vals map[string]string,
	extra map[string]string,
) map[string]string {
	n := len(base) + len(vals) + len(extra)
	if n == 0 {
		return nil
	}
	out := make(map[string]string, n)
	maps.Copy(out, base)
	maps.Copy(out, vals)
	maps.Copy(out, extra)
	return out
}

// Branch and iteration details need to look the same in engine events, reports, and the UI.
func StepLabel(step restfile.WorkflowStep, branch string, iter, total int) string {
	lbl := step.Label()
	if lbl == "" {
		lbl = "step"
	}
	if branch != "" {
		lbl = fmt.Sprintf("%s -> %s", lbl, branch)
	}
	if iter > 0 && total > 0 {
		lbl = fmt.Sprintf("%s (%d/%d)", lbl, iter, total)
	}
	return lbl
}

func stepMeta(
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
) StepMeta {
	step = stepOrDefault(step)
	target := step.Using
	if req != nil {
		target = engine.ReqTarget(req)
	}
	return StepMeta{
		Index:  i,
		Name:   step.Name,
		Kind:   step.Kind,
		Target: target,
		Branch: branch,
		Iter:   iter,
		Total:  total,
	}
}

func cloneStrMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	maps.Copy(out, src)
	return out
}
