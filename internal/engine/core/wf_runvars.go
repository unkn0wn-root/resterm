package core

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Keep failed results too, so later steps report the same error without retrying.
type runVarSet struct {
	vals vars.NameMap[string]
	err  error
}

func (r *wfRun) evalWorkflowRunVars(ctx context.Context) {
	sc := request.RunScope{Overlay: r.vars}
	r.wfRunVars.vals, r.wfRunVars.err = r.dep.EvalRunVars(
		ctx,
		r.pl.Doc,
		nil,
		r.pl.Run.Env,
		sc,
		r.pl.Workflow.RunVars,
	)
}

// The first step to use a request fixes its values for the rest of the run.
func (r *wfRun) runVarsFor(
	ctx context.Context,
	req *restfile.Request,
	overlay vars.NameMap[string],
) (vars.NameMap[string], error) {
	wf := r.wfRunVars
	if wf.err != nil || req == nil {
		return wf.vals, wf.err
	}
	set, ok := r.reqRunVars[req]
	if !ok {
		sc := request.RunScope{Overlay: overlay, RunVars: wf.vals}
		set.vals, set.err = r.dep.EvalRunVars(ctx, r.pl.Doc, req, r.pl.Run.Env, sc, req.RunVars)
		r.reqRunVars[req] = set
	}
	return set.vals, set.err
}
