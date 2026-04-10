package core

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (r *proRun) emitRunStart() error {
	return Emit(r.ectx, r.sink, RunStart{Meta: NewMeta(r.pl.Run, time.Now())})
}

func (r *proRun) emitRunDone(err error) error {
	return Emit(r.ectx, r.sink, RunDone{
		Meta:     NewMeta(r.pl.Run, time.Now()),
		Success:  r.done && !r.skip && !r.fail && !r.canceled && r.ok == r.pl.Spec.Count,
		Skipped:  r.seen && r.skip,
		Canceled: r.canceled,
		Err:      err,
	})
}

func (r *proRun) emitIterStart(it IterMeta, req *restfile.Request) error {
	return Emit(r.ectx, r.sink, ProIterStart{
		Meta:    NewMeta(r.pl.Run, time.Now()),
		Iter:    it,
		Doc:     r.pl.Doc,
		Request: req,
	})
}

func (r *proRun) emitIterDone(it IterMeta, res engine.RequestResult) error {
	return Emit(r.ectx, r.sink, ProIterDone{
		Meta:   NewMeta(r.pl.Run, time.Now()),
		Iter:   it,
		Result: res,
	})
}
