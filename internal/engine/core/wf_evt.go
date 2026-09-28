package core

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (r *wfRun) emitRunStart() error {
	return Emit(r.ectx, r.sink, RunStart{Meta: r.meta(time.Now())})
}

func (r *wfRun) emitRunDone(err error) error {
	return Emit(r.ectx, r.sink, RunDone{
		Meta:     r.meta(time.Now()),
		Success:  r.done && !r.skip && !r.fail && !r.canceled,
		Skipped:  r.seen && r.skip,
		Canceled: r.canceled,
		Err:      err,
	})
}

func (r *wfRun) emitStepStart(
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
) error {
	return Emit(r.ectx, r.sink, WfStepStart{
		Meta:    r.meta(time.Now()),
		Step:    stepMeta(i, step, req, branch, iter, total),
		Doc:     r.pl.Doc,
		Request: req,
	})
}

func (r *wfRun) emitStepDone(
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
	res engine.RequestResult,
) error {
	return Emit(r.ectx, r.sink, WfStepDone{
		Meta:   r.meta(time.Now()),
		Step:   stepMeta(i, step, req, branch, iter, total),
		Result: res,
	})
}

func (r *wfRun) emitReqStart(
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
) error {
	r.seq++
	return Emit(r.ectx, r.sink, ReqStart{
		Meta: r.meta(time.Now()),
		Req: ReqMeta{
			Index: r.seq,
			Label: StepLabel(step, branch, iter, total),
			Env:   r.pl.Run.Env.Label(),
		},
		Doc:     r.pl.Doc,
		Request: req,
	})
}

func (r *wfRun) emitReqDone(
	i int,
	step restfile.WorkflowStep,
	req *restfile.Request,
	branch string,
	iter int,
	total int,
	res engine.RequestResult,
) error {
	return Emit(r.ectx, r.sink, ReqDone{
		Meta: r.meta(time.Now()),
		Req: ReqMeta{
			Index: r.seq,
			Label: StepLabel(step, branch, iter, total),
			Env:   r.pl.Run.Env.Label(),
		},
		Result: res,
	})
}
