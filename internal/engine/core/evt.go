package core

import (
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Evt interface {
	evt()
	meta() EvtMeta
}

func MetaOf(e Evt) EvtMeta {
	if e == nil {
		return EvtMeta{}
	}
	return e.meta()
}

type RunStart struct {
	Meta EvtMeta
}

func (RunStart) evt()            {}
func (e RunStart) meta() EvtMeta { return e.Meta }

type RunDone struct {
	Meta     EvtMeta
	Success  bool
	Skipped  bool
	Canceled bool
	Err      error
}

func (RunDone) evt()            {}
func (e RunDone) meta() EvtMeta { return e.Meta }

type ReqStart struct {
	Meta    EvtMeta
	Req     ReqMeta
	Doc     *restfile.Document
	Request *restfile.Request
}

func (ReqStart) evt()            {}
func (e ReqStart) meta() EvtMeta { return e.Meta }

type ReqDone struct {
	Meta   EvtMeta
	Req    ReqMeta
	Result engine.RequestResult
}

func (ReqDone) evt()            {}
func (e ReqDone) meta() EvtMeta { return e.Meta }

type WfStepStart struct {
	Meta    EvtMeta
	Step    StepMeta
	Doc     *restfile.Document
	Request *restfile.Request
}

func (WfStepStart) evt()            {}
func (e WfStepStart) meta() EvtMeta { return e.Meta }

type WfStepDone struct {
	Meta   EvtMeta
	Step   StepMeta
	Result engine.RequestResult
}

func (WfStepDone) evt()            {}
func (e WfStepDone) meta() EvtMeta { return e.Meta }

type CmpRowStart struct {
	Meta    EvtMeta
	Row     RowMeta
	Doc     *restfile.Document
	Request *restfile.Request
}

func (CmpRowStart) evt()            {}
func (e CmpRowStart) meta() EvtMeta { return e.Meta }

type CmpRowDone struct {
	Meta   EvtMeta
	Row    RowMeta
	Result engine.RequestResult
}

func (CmpRowDone) evt()            {}
func (e CmpRowDone) meta() EvtMeta { return e.Meta }

type ProIterStart struct {
	Meta    EvtMeta
	Iter    IterMeta
	Doc     *restfile.Document
	Request *restfile.Request
}

func (ProIterStart) evt()            {}
func (e ProIterStart) meta() EvtMeta { return e.Meta }

type ProIterDone struct {
	Meta   EvtMeta
	Iter   IterMeta
	Result engine.RequestResult
}

func (ProIterDone) evt()            {}
func (e ProIterDone) meta() EvtMeta { return e.Meta }
