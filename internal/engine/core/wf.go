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

func PrepareWorkflow(
	doc *restfile.Document,
	wf restfile.Workflow,
	run RunMeta,
) (*WorkflowPlan, error) {
	if doc == nil {
		return nil, fmt.Errorf("no document loaded")
	}
	steps, reqs, err := prepareWorkflow(doc, wf)
	if err != nil {
		return nil, err
	}
	run = normRun(run, ModeWorkflow, strings.TrimSpace(wf.Name), run.Env)
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
	name := requestBaseTitle(req)
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
		res := engine.RequestResult{
			Err: errdef.New(errdef.CodeUI, "workflow step missing request"),
		}
		if err := r.emitStepStart(ctx, r.idx, step, nil, branch, 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(ctx, r.idx, step, nil, branch, 0, 0, res); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return true, nil
	}

	vals := stepVars(step)
	applyVars(r.vars, vals)
	xv := stepExtras(r.vars, vals, nil)
	vv := r.dep.CollectVariables(r.pl.Doc, req, r.pl.Run.Env, xv)

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
			wrap := errdef.Wrap(errdef.CodeScript, err, "@when")
			if err := r.emitStepStart(ctx, r.idx, step, req, branch, 0, 0); err != nil {
				return false, err
			}
			if err := r.emitStepDone(
				ctx,
				r.idx,
				step,
				req,
				branch,
				0,
				0,
				engine.RequestResult{Err: wrap},
			); err != nil {
				return false, err
			}
			r.note(false, false, false)
			r.idx++
			return step.OnFailure != restfile.WorkflowOnFailureContinue, nil
		}
		if !ok {
			if err := r.emitStepStart(ctx, r.idx, step, req, branch, 0, 0); err != nil {
				return false, err
			}
			if err := r.emitStepDone(
				ctx,
				r.idx,
				step,
				req,
				branch,
				0,
				0,
				engine.RequestResult{Skipped: true, SkipReason: reason},
			); err != nil {
				return false, err
			}
			r.note(false, true, false)
			r.idx++
			return false, nil
		}
	}

	spec, err := workflowForEach(step, req)
	if err != nil {
		if ctx.Err() != nil {
			r.canceled = true
			r.idx++
			return true, nil
		}
		wrap := errdef.Wrap(errdef.CodeScript, err, "@for-each")
		if err := r.emitStepStart(ctx, r.idx, step, req, branch, 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(
			ctx,
			r.idx,
			step,
			req,
			branch,
			0,
			0,
			engine.RequestResult{Err: wrap},
		); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return step.OnFailure != restfile.WorkflowOnFailureContinue, nil
	}
	if spec == nil {
		if err := r.emitStepStart(ctx, r.idx, step, req, branch, 0, 0); err != nil {
			return false, err
		}
		out, err := r.execReq(ctx, r.idx, step, req, branch, 0, 0, xv, nil)
		if err != nil {
			return false, err
		}
		if err := r.emitStepDone(ctx, r.idx, step, req, branch, 0, 0, out); err != nil {
			return false, err
		}
		ok, skip, cancel := evalReq(step, out)
		r.note(ok, skip, cancel)
		r.idx++
		if cancel {
			r.canceled = true
			return true, nil
		}
		return !skip && !ok && step.OnFailure != restfile.WorkflowOnFailureContinue, nil
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
		wrap := errdef.Wrap(errdef.CodeScript, err, "@for-each")
		if err := r.emitStepStart(ctx, r.idx, step, req, branch, 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(
			ctx,
			r.idx,
			step,
			req,
			branch,
			0,
			0,
			engine.RequestResult{Err: wrap},
		); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return step.OnFailure != restfile.WorkflowOnFailureContinue, nil
	}
	if len(items) == 0 {
		if err := r.emitStepStart(ctx, r.idx, step, req, branch, 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(
			ctx,
			r.idx,
			step,
			req,
			branch,
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: "for-each produced no items"},
		); err != nil {
			return false, err
		}
		r.note(false, true, false)
		r.idx++
		return false, nil
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
			wrap := errdef.Wrap(errdef.CodeScript, err, "@for-each")
			if err := r.emitStepStart(ctx, r.idx, step, req, branch, i+1, len(items)); err != nil {
				return false, err
			}
			if err := r.emitStepDone(
				ctx,
				r.idx,
				step,
				req,
				branch,
				i+1,
				len(items),
				engine.RequestResult{Err: wrap},
			); err != nil {
				return false, err
			}
			r.note(false, false, false)
			if step.OnFailure != restfile.WorkflowOnFailureContinue {
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
				wrap := errdef.Wrap(errdef.CodeScript, err, "@when")
				if err := r.emitStepStart(
					ctx,
					r.idx,
					step,
					req,
					branch,
					i+1,
					len(items),
				); err != nil {
					return false, err
				}
				if err := r.emitStepDone(
					ctx,
					r.idx,
					step,
					req,
					branch,
					i+1,
					len(items),
					engine.RequestResult{Err: wrap},
				); err != nil {
					return false, err
				}
				r.note(false, false, false)
				if step.OnFailure != restfile.WorkflowOnFailureContinue {
					r.idx++
					return true, nil
				}
				continue
			}
			if !ok {
				if err := r.emitStepStart(
					ctx,
					r.idx,
					step,
					req,
					branch,
					i+1,
					len(items),
				); err != nil {
					return false, err
				}
				if err := r.emitStepDone(
					ctx,
					r.idx,
					step,
					req,
					branch,
					i+1,
					len(items),
					engine.RequestResult{Skipped: true, SkipReason: reason},
				); err != nil {
					return false, err
				}
				r.note(false, true, false)
				continue
			}
		}

		if err := r.emitStepStart(ctx, r.idx, step, req, branch, i+1, len(items)); err != nil {
			return false, err
		}
		out, err := r.execReq(ctx, r.idx, step, req, branch, i+1, len(items), loopVars, ev)
		if err != nil {
			return false, err
		}
		if err := r.emitStepDone(ctx, r.idx, step, req, branch, i+1, len(items), out); err != nil {
			return false, err
		}
		ok, skip, cancel := evalReq(step, out)
		r.note(ok, skip, cancel)
		if cancel {
			r.canceled = true
			r.idx++
			return true, nil
		}
		if !skip && !ok && step.OnFailure != restfile.WorkflowOnFailureContinue {
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
		res := engine.RequestResult{
			Err: errdef.New(errdef.CodeUI, "workflow @if missing definition"),
		}
		if err := r.emitStepStart(ctx, r.idx, step, nil, "", 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(ctx, r.idx, step, nil, "", 0, 0, res); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return true, nil
	}

	vals := stepVars(step)
	applyVars(r.vars, vals)
	xv := stepExtras(r.vars, vals, nil)
	vv := r.dep.CollectVariables(r.pl.Doc, nil, r.pl.Run.Env, xv)
	eval := func(expr string, line int, tag string) (bool, error) {
		if strings.TrimSpace(expr) == "" {
			return false, fmt.Errorf("%s expression missing", tag)
		}
		val, err := r.dep.EvalValue(
			ctx,
			r.pl.Doc,
			nil,
			r.pl.Run.Env,
			baseDir(r.pl.Doc),
			expr,
			tag+" "+expr,
			r.dep.PosForLine(r.pl.Doc, nil, line),
			vv,
			nil,
		)
		if err != nil {
			return false, err
		}
		return val.IsTruthy(), nil
	}

	var br *restfile.WorkflowIfBranch
	ok, err := eval(step.If.Then.Cond, step.If.Then.Line, "@if")
	if err != nil {
		if ctx.Err() != nil {
			r.canceled = true
			return true, nil
		}
		return r.failBranch(ctx, step, "", errdef.Wrap(errdef.CodeScript, err, "@if"))
	}
	if ok {
		br = &step.If.Then
	} else {
		for i := range step.If.Elifs {
			item := &step.If.Elifs[i]
			ok, err = eval(item.Cond, item.Line, "@elif")
			if err != nil {
				if ctx.Err() != nil {
					r.canceled = true
					return true, nil
				}
				return r.failBranch(ctx, step, "", errdef.Wrap(errdef.CodeScript, err, "@elif"))
			}
			if ok {
				br = item
				break
			}
		}
	}
	if br == nil && step.If.Else != nil {
		br = step.If.Else
	}
	if br == nil {
		if err := r.emitStepStart(ctx, r.idx, step, nil, "", 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(
			ctx,
			r.idx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: "no @if branch matched"},
		); err != nil {
			return false, err
		}
		r.note(false, true, false)
		r.idx++
		return false, nil
	}
	if msg := strings.TrimSpace(br.Fail); msg != "" {
		res := engine.RequestResult{Err: fmt.Errorf("%s", msg)}
		if err := r.emitStepStart(ctx, r.idx, step, nil, "", 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(ctx, r.idx, step, nil, "", 0, 0, res); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return true, nil
	}
	run := strings.ToLower(strings.TrimSpace(br.Run))
	if run == "" {
		if err := r.emitStepStart(ctx, r.idx, step, nil, "", 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(
			ctx,
			r.idx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: "no @if run target"},
		); err != nil {
			return false, err
		}
		r.note(false, true, false)
		r.idx++
		return false, nil
	}
	req := r.pl.Reqs[run]
	if req == nil {
		res := engine.RequestResult{
			Err: fmt.Errorf("request %s not found", strings.TrimSpace(br.Run)),
		}
		if err := r.emitStepStart(
			ctx,
			r.idx,
			step,
			nil,
			strings.TrimSpace(br.Run),
			0,
			0,
		); err != nil {
			return false, err
		}
		if err := r.emitStepDone(
			ctx,
			r.idx,
			step,
			nil,
			strings.TrimSpace(br.Run),
			0,
			0,
			res,
		); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return true, nil
	}

	if err := r.emitStepStart(ctx, r.idx, step, req, strings.TrimSpace(br.Run), 0, 0); err != nil {
		return false, err
	}
	out, err := r.execReq(ctx, r.idx, step, req, strings.TrimSpace(br.Run), 0, 0, xv, nil)
	if err != nil {
		return false, err
	}
	if err := r.emitStepDone(
		ctx,
		r.idx,
		step,
		req,
		strings.TrimSpace(br.Run),
		0,
		0,
		out,
	); err != nil {
		return false, err
	}
	ok, skip, cancel := evalReq(step, out)
	r.note(ok, skip, cancel)
	r.idx++
	if cancel {
		r.canceled = true
		return true, nil
	}
	return !skip && !ok && step.OnFailure != restfile.WorkflowOnFailureContinue, nil
}

func (r *wfRun) runSwitch(ctx context.Context, step restfile.WorkflowStep) (bool, error) {
	if ctx.Err() != nil {
		r.canceled = true
		return true, nil
	}
	if step.Switch == nil {
		res := engine.RequestResult{
			Err: errdef.New(errdef.CodeUI, "workflow @switch missing definition"),
		}
		if err := r.emitStepStart(ctx, r.idx, step, nil, "", 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(ctx, r.idx, step, nil, "", 0, 0, res); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return true, nil
	}

	vals := stepVars(step)
	applyVars(r.vars, vals)
	xv := stepExtras(r.vars, vals, nil)
	vv := r.dep.CollectVariables(r.pl.Doc, nil, r.pl.Run.Env, xv)
	expr := strings.TrimSpace(step.Switch.Expr)
	if expr == "" {
		res := engine.RequestResult{
			Err: errdef.New(errdef.CodeUI, "@switch expression missing"),
		}
		if err := r.emitStepDone(ctx, r.idx, step, nil, "", 0, 0, res); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return true, nil
	}

	base, err := r.dep.EvalValue(
		ctx,
		r.pl.Doc,
		nil,
		r.pl.Run.Env,
		baseDir(r.pl.Doc),
		expr,
		"@switch "+expr,
		r.dep.PosForLine(r.pl.Doc, nil, step.Switch.Line),
		vv,
		nil,
	)
	if err != nil {
		if ctx.Err() != nil {
			r.canceled = true
			return true, nil
		}
		return r.failBranch(ctx, step, "", errdef.Wrap(errdef.CodeScript, err, "@switch"))
	}

	var sel *restfile.WorkflowSwitchCase
	for i := range step.Switch.Cases {
		item := &step.Switch.Cases[i]
		expr := strings.TrimSpace(item.Expr)
		if expr == "" {
			continue
		}
		val, err := r.dep.EvalValue(
			ctx,
			r.pl.Doc,
			nil,
			r.pl.Run.Env,
			baseDir(r.pl.Doc),
			expr,
			"@case "+expr,
			r.dep.PosForLine(r.pl.Doc, nil, item.Line),
			vv,
			nil,
		)
		if err != nil {
			if ctx.Err() != nil {
				r.canceled = true
				return true, nil
			}
			return r.failBranch(ctx, step, "", errdef.Wrap(errdef.CodeScript, err, "@case"))
		}
		if rts.ValueEqual(base, val) {
			sel = item
			break
		}
	}
	if sel == nil {
		sel = step.Switch.Default
	}
	if sel == nil {
		if err := r.emitStepStart(ctx, r.idx, step, nil, "", 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(
			ctx,
			r.idx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: "no @switch case matched"},
		); err != nil {
			return false, err
		}
		r.note(false, true, false)
		r.idx++
		return false, nil
	}
	if msg := strings.TrimSpace(sel.Fail); msg != "" {
		res := engine.RequestResult{Err: fmt.Errorf("%s", msg)}
		if err := r.emitStepStart(ctx, r.idx, step, nil, "", 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(ctx, r.idx, step, nil, "", 0, 0, res); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return true, nil
	}
	run := strings.ToLower(strings.TrimSpace(sel.Run))
	if run == "" {
		if err := r.emitStepStart(ctx, r.idx, step, nil, "", 0, 0); err != nil {
			return false, err
		}
		if err := r.emitStepDone(
			ctx,
			r.idx,
			step,
			nil,
			"",
			0,
			0,
			engine.RequestResult{Skipped: true, SkipReason: "no @switch run target"},
		); err != nil {
			return false, err
		}
		r.note(false, true, false)
		r.idx++
		return false, nil
	}
	req := r.pl.Reqs[run]
	if req == nil {
		res := engine.RequestResult{
			Err: fmt.Errorf("request %s not found", strings.TrimSpace(sel.Run)),
		}
		if err := r.emitStepStart(
			ctx,
			r.idx,
			step,
			nil,
			strings.TrimSpace(sel.Run),
			0,
			0,
		); err != nil {
			return false, err
		}
		if err := r.emitStepDone(
			ctx,
			r.idx,
			step,
			nil,
			strings.TrimSpace(sel.Run),
			0,
			0,
			res,
		); err != nil {
			return false, err
		}
		r.note(false, false, false)
		r.idx++
		return true, nil
	}

	if err := r.emitStepStart(ctx, r.idx, step, req, strings.TrimSpace(sel.Run), 0, 0); err != nil {
		return false, err
	}
	out, err := r.execReq(ctx, r.idx, step, req, strings.TrimSpace(sel.Run), 0, 0, xv, nil)
	if err != nil {
		return false, err
	}
	if err := r.emitStepDone(
		ctx,
		r.idx,
		step,
		req,
		strings.TrimSpace(sel.Run),
		0,
		0,
		out,
	); err != nil {
		return false, err
	}
	ok, skip, cancel := evalReq(step, out)
	r.note(ok, skip, cancel)
	r.idx++
	if cancel {
		r.canceled = true
		return true, nil
	}
	return !skip && !ok && step.OnFailure != restfile.WorkflowOnFailureContinue, nil
}

func (r *wfRun) failBranch(
	ctx context.Context,
	step restfile.WorkflowStep,
	branch string,
	err error,
) (bool, error) {
	if e := r.emitStepStart(ctx, r.idx, step, nil, branch, 0, 0); e != nil {
		return false, e
	}
	if e := r.emitStepDone(
		ctx,
		r.idx,
		step,
		nil,
		branch,
		0,
		0,
		engine.RequestResult{Err: err},
	); e != nil {
		return false, e
	}
	r.note(false, false, false)
	r.idx++
	return true, nil
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

func (r *wfRun) note(ok, skip, cancel bool) {
	r.done = true
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
	lbl := strings.TrimSpace(step.Name)
	if lbl == "" {
		switch step.Kind {
		case restfile.WorkflowStepKindIf:
			lbl = "@if"
		case restfile.WorkflowStepKindSwitch:
			lbl = "@switch"
		case restfile.WorkflowStepKindForEach:
			lbl = strings.TrimSpace(step.Using)
			if lbl == "" {
				lbl = "@for-each"
			}
		default:
			lbl = strings.TrimSpace(step.Using)
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
	target := strings.TrimSpace(step.Using)
	if req != nil {
		target = requestTarget(req)
	}
	return StepMeta{
		Index:  i,
		Name:   strings.TrimSpace(step.Name),
		Kind:   step.Kind,
		Target: target,
		Branch: branch,
		Iter:   iter,
		Total:  total,
	}
}

func requestTarget(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if req.GRPC != nil {
		if m := strings.TrimSpace(req.GRPC.FullMethod); m != "" {
			return m
		}
		if t := strings.TrimSpace(req.GRPC.Target); t != "" {
			return t
		}
	}
	return strings.TrimSpace(req.URL)
}

func requestBaseTitle(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	name := strings.TrimSpace(req.Metadata.Name)
	if name == "" {
		name = requestTarget(req)
		if len(name) > 60 {
			name = name[:57] + "..."
		}
	}
	method := "REQ"
	switch {
	case req.GRPC != nil:
		method = "GRPC"
	case req.WebSocket != nil:
		method = "WS"
	case req.SSE != nil:
		method = "SSE"
	default:
		if m := strings.ToUpper(strings.TrimSpace(req.Method)); m != "" {
			method = m
		}
	}
	return fmt.Sprintf("%s %s", method, name)
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
