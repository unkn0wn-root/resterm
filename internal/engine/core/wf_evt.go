package core

import (
	"context"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (r *wfRun) emitRunStart(ctx context.Context) error {
	return Emit(ctx, r.sink, RunStart{Meta: r.meta(time.Now())})
}

func (r *wfRun) emitRunDone(ctx context.Context, err error) error {
	return Emit(ctx, r.sink, RunDone{
		Meta:     r.meta(time.Now()),
		Success:  r.done && !r.skip && !r.fail && !r.canceled,
		Skipped:  r.seen && r.skip,
		Canceled: r.canceled,
		Err:      err,
	})
}

func (r *wfRun) emitStepStart(
	ctx context.Context,
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
) error {
	return Emit(ctx, r.sink, WfStepStart{
		Meta:    r.meta(time.Now()),
		Step:    stepMeta(i, step, req, branch, iter, total),
		Doc:     r.pl.Doc,
		Request: req,
	})
}

func (r *wfRun) emitStepDone(
	ctx context.Context,
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
	res engine.RequestResult,
) error {
	return Emit(ctx, r.sink, WfStepDone{
		Meta:   r.meta(time.Now()),
		Step:   stepMeta(i, step, req, branch, iter, total),
		Result: res,
	})
}

func (r *wfRun) emitReqStart(
	ctx context.Context,
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
) error {
	r.seq++
	return Emit(ctx, r.sink, ReqStart{
		Meta: r.meta(time.Now()),
		Req: ReqMeta{
			Index: r.seq,
			Label: stepLabel(step, branch, iter, total),
			Env:   r.pl.Run.Env,
		},
		Doc:     r.pl.Doc,
		Request: req,
	})
}

func (r *wfRun) emitReqDone(
	ctx context.Context,
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
	res engine.RequestResult,
) error {
	return Emit(ctx, r.sink, ReqDone{
		Meta: r.meta(time.Now()),
		Req: ReqMeta{
			Index: r.seq,
			Label: stepLabel(step, branch, iter, total),
			Env:   r.pl.Run.Env,
		},
		Result: res,
	})
}
