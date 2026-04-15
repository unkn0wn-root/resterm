package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/errdef"
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
	wfTagWhen    = "@when"
	wfTagForEach = "@for-each"
	wfTagIf      = "@if"
	wfTagElif    = "@elif"
	wfTagSwitch  = "@switch"
	wfTagCase    = "@case"

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
	wf = engine.NormWf(wf)
	steps, reqs, err := prepareWorkflow(doc, wf)
	if err != nil {
		return nil, err
	}
	run = normRun(run, ModeWorkflow, wf.Name, run.Env)
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
	run = normRun(run, ModeForEach, name, run.Env)
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
		res := engine.RequestResult{
			Err: errdef.New(errdef.CodeUI, "unknown workflow step kind %q", step.Kind),
		}
		if err := r.emitStepStart(ctx, r.idx, step, rt.Req, "", 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(ctx, r.idx, step, rt.Req, "", 0, 0, res); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return true, nil
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
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			branch,
			0,
			0,
			engine.RequestResult{Err: errdef.New(errdef.CodeUI, "workflow step missing request")},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, true), nil
	}

	xv, vv := r.stepScope(step, req, nil)
	stopOnFailure := step.OnFailure != restfile.WorkflowOnFailureContinue

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
			ok, skip, cancel, emitErr := r.emitManualStep(
				ctx,
				step,
				req,
				branch,
				0,
				0,
				engine.RequestResult{Err: errdef.Wrap(errdef.CodeScript, err, wfTagWhen)},
			)
			if emitErr != nil {
				return false, emitErr
			}
			return r.finishStep(ok, skip, cancel, true, stopOnFailure), nil
		}
		if !ok {
			ok, skip, cancel, emitErr := r.emitManualStep(
				ctx,
				step,
				req,
				branch,
				0,
				0,
				engine.RequestResult{Skipped: true, SkipReason: reason},
			)
			if emitErr != nil {
				return false, emitErr
			}
			return r.finishStep(ok, skip, cancel, true, false), nil
		}
	}

	spec, err := workflowForEach(step, req)
	if err != nil {
		if ctx.Err() != nil {
			r.canceled = true
			r.idx++
			return true, nil
		}
		ok, skip, cancel, emitErr := r.emitManualStep(
			ctx,
			step,
			req,
			branch,
			0,
			0,
			engine.RequestResult{Err: errdef.Wrap(errdef.CodeScript, err, wfTagForEach)},
		)
		if emitErr != nil {
			return false, emitErr
		}
		return r.finishStep(ok, skip, cancel, true, stopOnFailure), nil
	}
	if spec == nil {
		ok, skip, cancel, err := r.executeStepRequest(ctx, step, req, branch, 0, 0, xv, nil)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, stopOnFailure), nil
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
		ok, skip, cancel, emitErr := r.emitManualStep(
			ctx,
			step,
			req,
			branch,
			0,
			0,
			engine.RequestResult{Err: errdef.Wrap(errdef.CodeScript, err, wfTagForEach)},
		)
		if emitErr != nil {
			return false, emitErr
		}
		return r.finishStep(ok, skip, cancel, true, stopOnFailure), nil
	}
	if len(items) == 0 {
		ok, skip, cancel, emitErr := r.emitManualStep(
			ctx,
			step,
			req,
			branch,
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: wfSkipForEachNoItems},
		)
		if emitErr != nil {
			return false, emitErr
		}
		return r.finishStep(ok, skip, cancel, true, false), nil
	}

	reqKey, wfKey := loopKeys(r.pl.WfVars, spec.Var)
	for i, item := range items {
		itemStr, err := r.dep.ValueString(ctx, r.dep.PosForLine(r.pl.Doc, req, spec.Line), item)
		if err != nil {
			if ctx.Err() != nil {
				r.canceled = true
				r.idx++
				return true, nil
			}
			ok, skip, cancel, emitErr := r.emitManualStep(
				ctx,
				step,
				req,
				branch,
				i+1,
				len(items),
				engine.RequestResult{Err: errdef.Wrap(errdef.CodeScript, err, wfTagForEach)},
			)
			if emitErr != nil {
				return false, emitErr
			}
			if r.finishStep(ok, skip, cancel, false, stopOnFailure) {
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
				ok, skip, cancel, emitErr := r.emitManualStep(
					ctx,
					step,
					req,
					branch,
					i+1,
					len(items),
					engine.RequestResult{Err: errdef.Wrap(errdef.CodeScript, err, wfTagWhen)},
				)
				if emitErr != nil {
					return false, emitErr
				}
				if r.finishStep(ok, skip, cancel, false, stopOnFailure) {
					r.idx++
					return true, nil
				}
				continue
			}
			if !ok {
				ok, skip, cancel, emitErr := r.emitManualStep(
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
				r.finishStep(ok, skip, cancel, false, false)
				continue
			}
		}

		ok, skip, cancel, err := r.executeStepRequest(
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
		if r.finishStep(ok, skip, cancel, false, stopOnFailure) {
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
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Err: errdef.New(errdef.CodeUI, "workflow @if missing definition")},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, true), nil
	}

	xv, vv := r.stepScope(step, nil, nil)
	br, err := r.selectIfBranch(ctx, step, vv)
	if err != nil {
		if ctx.Err() != nil {
			r.canceled = true
			return true, nil
		}
		ok, skip, cancel, emitErr := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Err: err},
		)
		if emitErr != nil {
			return false, emitErr
		}
		return r.finishStep(ok, skip, cancel, true, true), nil
	}
	if br == nil {
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: wfSkipIfNoBranch},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, false), nil
	}
	if msg := strings.TrimSpace(br.Fail); msg != "" {
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Err: fmt.Errorf("%s", msg)},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, true), nil
	}
	branch, req := r.resolveBranchRequest(br.Run)
	if branch == "" {
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: wfSkipIfNoRun},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, false), nil
	}
	if req == nil {
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			branch,
			0,
			0,
			engine.RequestResult{Err: fmt.Errorf("request %s not found", branch)},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, true), nil
	}

	ok, skip, cancel, err := r.executeStepRequest(ctx, step, req, branch, 0, 0, xv, nil)
	if err != nil {
		return false, err
	}
	return r.finishStep(
		ok,
		skip,
		cancel,
		true,
		step.OnFailure != restfile.WorkflowOnFailureContinue,
	), nil
}

func (r *wfRun) runSwitch(ctx context.Context, step restfile.WorkflowStep) (bool, error) {
	if ctx.Err() != nil {
		r.canceled = true
		return true, nil
	}
	if step.Switch == nil {
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{
				Err: errdef.New(errdef.CodeUI, "workflow @switch missing definition"),
			},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, true), nil
	}

	xv, vv := r.stepScope(step, nil, nil)
	sel, err := r.selectSwitchCase(ctx, step, vv)
	if err != nil {
		if ctx.Err() != nil {
			r.canceled = true
			return true, nil
		}
		ok, skip, cancel, emitErr := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Err: err},
		)
		if emitErr != nil {
			return false, emitErr
		}
		return r.finishStep(ok, skip, cancel, true, true), nil
	}
	if sel == nil {
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: wfSkipSwitchNoCase},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, false), nil
	}
	if msg := strings.TrimSpace(sel.Fail); msg != "" {
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Err: fmt.Errorf("%s", msg)},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, true), nil
	}
	branch, req := r.resolveBranchRequest(sel.Run)
	if branch == "" {
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: wfSkipSwitchNoRun},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, false), nil
	}
	if req == nil {
		ok, skip, cancel, err := r.emitManualStep(
			ctx,
			step,
			nil,
			branch,
			0,
			0,
			engine.RequestResult{Err: fmt.Errorf("request %s not found", branch)},
		)
		if err != nil {
			return false, err
		}
		return r.finishStep(ok, skip, cancel, true, true), nil
	}

	ok, skip, cancel, err := r.executeStepRequest(ctx, step, req, branch, 0, 0, xv, nil)
	if err != nil {
		return false, err
	}
	return r.finishStep(
		ok,
		skip,
		cancel,
		true,
		step.OnFailure != restfile.WorkflowOnFailureContinue,
	), nil
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
	vals := stepVars(step)
	applyVars(r.vars, vals)
	xv := stepExtras(r.vars, vals, extra)
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
	out engine.RequestResult,
) (bool, bool, bool, error) {
	if err := r.emitStepStart(ctx, r.idx, step, req, branch, iter, total); err != nil {
		return false, false, false, err
	}
	if err := r.emitStepDone(ctx, r.idx, step, req, branch, iter, total, out); err != nil {
		return false, false, false, err
	}
	ok, skip, cancel := evalReq(step, out)
	r.note(ok, skip, cancel)
	return ok, skip, cancel, nil
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
) (bool, bool, bool, error) {
	if err := r.emitStepStart(ctx, r.idx, step, req, branch, iter, total); err != nil {
		return false, false, false, err
	}
	out, err := r.execReq(ctx, r.idx, step, req, branch, iter, total, extra, vals)
	if err != nil {
		return false, false, false, err
	}
	if err := r.emitStepDone(ctx, r.idx, step, req, branch, iter, total, out); err != nil {
		return false, false, false, err
	}
	ok, skip, cancel := evalReq(step, out)
	r.note(ok, skip, cancel)
	return ok, skip, cancel, nil
}

func (r *wfRun) finishStep(
	ok bool,
	skip bool,
	cancel bool,
	advance bool,
	stopOnFailure bool,
) bool {
	if advance {
		r.idx++
	}
	if cancel {
		r.canceled = true
		return true
	}
	return !skip && !ok && stopOnFailure
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
		return nil, errdef.Wrap(errdef.CodeScript, err, wfTagIf)
	}
	if ok {
		return &step.If.Then, nil
	}
	for i := range step.If.Elifs {
		item := &step.If.Elifs[i]
		ok, err = r.evalStepBool(ctx, nil, item.Line, wfTagElif, item.Cond, vv, nil)
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeScript, err, wfTagElif)
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
		return nil, errdef.New(errdef.CodeUI, "@switch expression missing")
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
		return nil, errdef.Wrap(errdef.CodeScript, err, wfTagSwitch)
	}
	for i := range step.Switch.Cases {
		item := &step.Switch.Cases[i]
		expr := strings.TrimSpace(item.Expr)
		if expr == "" {
			continue
		}
		val, err := r.evalStepValue(ctx, nil, item.Line, wfTagCase, expr, vv, nil)
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeScript, err, wfTagCase)
		}
		if rts.ValueEqual(base, val) {
			return item, nil
		}
	}
	return step.Switch.Default, nil
}

func (r *wfRun) note(ok, skip, cancel bool) {
	r.seen = true
	if !skip {
		r.skip = false
	}
	if !skip && !ok {
		r.fail = true
	}
	if cancel {
		r.canceled = true
	}
}

func (r *wfRun) meta(at time.Time) EvtMeta {
	return NewMeta(r.pl.Run, at)
}

func evalReq(step restfile.WorkflowStep, out engine.RequestResult) (bool, bool, bool) {
	if out.Skipped {
		return false, true, false
	}
	if out.Err != nil {
		return false, false, errors.Is(out.Err, context.Canceled)
	}
	if out.ScriptErr != nil {
		return false, false, false
	}
	for _, t := range out.Tests {
		if !t.Passed {
			return false, false, false
		}
	}
	if exp, ok := step.Expect["status"]; ok {
		want := strings.TrimSpace(exp)
		if want == "" {
			return false, false, false
		}
		switch {
		case out.Response != nil:
			if !strings.EqualFold(want, strings.TrimSpace(out.Response.Status)) {
				return false, false, false
			}
		case out.GRPC != nil:
			if !strings.EqualFold(want, strings.TrimSpace(out.GRPC.StatusCode.String())) {
				return false, false, false
			}
		default:
			return false, false, false
		}
	}
	if exp, ok := step.Expect["statuscode"]; ok {
		want, err := strconv.Atoi(strings.TrimSpace(exp))
		if err != nil {
			return false, false, false
		}
		got := 0
		switch {
		case out.Response != nil:
			got = out.Response.StatusCode
			if got >= 400 && !hasStatusExp(step.Expect) {
				return false, false, false
			}
		case out.GRPC != nil:
			got = int(out.GRPC.StatusCode)
		default:
			return false, false, false
		}
		if got != want {
			return false, false, false
		}
	}
	switch {
	case out.Response != nil:
		if out.Response.StatusCode >= 400 && !hasStatusExp(step.Expect) {
			return false, false, false
		}
		return true, false, false
	case out.GRPC != nil, out.Stream != nil, len(out.Transcript) > 0:
		return true, false, false
	default:
		return false, false, false
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

func normRun(run RunMeta, mode Mode, name, env string) RunMeta {
	if run.Mode == ModeUnknown {
		run.Mode = mode
	}
	if strings.TrimSpace(run.Name) == "" {
		run.Name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(run.Env) == "" {
		run.Env = strings.TrimSpace(env)
	}
	if strings.TrimSpace(run.ID) == "" {
		run.ID = run.Mode.String() + ":" + run.Name + ":" + run.Env
	}
	return run
}

func stepOrDefault(step restfile.WorkflowStep) restfile.WorkflowStep {
	if step.Kind == "" {
		step.Kind = restfile.WorkflowStepKindRequest
	}
	return step
}

func stepVars(step restfile.WorkflowStep) map[string]string {
	if len(step.Vars) == 0 {
		return nil
	}
	out := make(map[string]string, len(step.Vars))
	for k, v := range step.Vars {
		if k == "" {
			continue
		}
		if !strings.HasPrefix(k, "vars.") {
			k = "vars." + k
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func applyVars(dst map[string]string, vals map[string]string) {
	if len(vals) == 0 {
		return
	}
	if dst == nil {
		return
	}
	for k, v := range vals {
		if strings.HasPrefix(k, "vars.workflow.") {
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
	for k, v := range base {
		out[k] = v
	}
	for k, v := range vals {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func loopKeys(wfVars bool, name string) (string, string) {
	if name == "" {
		return "", ""
	}
	reqKey := "vars.request." + name
	wfKey := ""
	if wfVars {
		wfKey = "vars.workflow." + name
	}
	return reqKey, wfKey
}

func stepLabel(step restfile.WorkflowStep, branch string, iter, total int) string {
	lbl := step.Name
	if lbl == "" {
		switch step.Kind {
		case restfile.WorkflowStepKindIf:
			lbl = "@if"
		case restfile.WorkflowStepKindSwitch:
			lbl = "@switch"
		case restfile.WorkflowStepKindForEach:
			lbl = step.Using
			if lbl == "" {
				lbl = "@for-each"
			}
		default:
			lbl = step.Using
		}
	}
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
	for k, v := range src {
		out[k] = v
	}
	return out
}

func hasStatusExp(exp map[string]string) bool {
	if len(exp) == 0 {
		return false
	}
	if _, ok := exp["status"]; ok {
		return true
	}
	_, ok := exp["statuscode"]
	return ok
}
