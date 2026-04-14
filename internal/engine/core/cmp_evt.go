package core

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (r *cmpRun) emitRunStart() error {
	return Emit(r.ectx, r.sink, RunStart{Meta: NewMeta(r.pl.Run, time.Now())})
}

func (r *cmpRun) emitRunDone(err error) error {
	return Emit(r.ectx, r.sink, RunDone{
		Meta:     NewMeta(r.pl.Run, time.Now()),
		Success:  r.done && !r.skip && !r.fail && !r.canceled,
		Skipped:  r.seen && r.skip,
		Canceled: r.canceled,
		Err:      err,
	})
}

func (r *cmpRun) emitRowStart(
	i int,
	env string,
	total int,
	req *restfile.Request,
) error {
	return Emit(r.ectx, r.sink, CmpRowStart{
		Meta:    NewMeta(r.pl.Run, time.Now()),
		Row:     r.row(i, env, total),
		Doc:     r.pl.Doc,
		Request: req,
	})
}

func (r *cmpRun) emitRowDone(
	i int,
	env string,
	total int,
	res engine.RequestResult,
) error {
	return Emit(r.ectx, r.sink, CmpRowDone{
		Meta:   NewMeta(r.pl.Run, time.Now()),
		Row:    r.row(i, env, total),
		Result: res,
	})
}
