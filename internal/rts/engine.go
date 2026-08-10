package rts

import (
	"context"
	"fmt"
	"os"
)

const (
	defaultMaxSteps = 10000
	defaultMaxCall  = 64
	defaultMaxStr   = 65536
	defaultMaxList  = 2000
	defaultMaxDict  = 2000
)

func defaultLimits() Limits {
	return Limits{
		MaxSteps: defaultMaxSteps,
		MaxCall:  defaultMaxCall,
		MaxStr:   defaultMaxStr,
		MaxList:  defaultMaxList,
		MaxDict:  defaultMaxDict,
	}
}

type Use struct {
	Path  string
	Alias string
}

// RT is the host-provided runtime surface visible to an evaluation.
type RT struct {
	Env         map[string]string
	EnvGroups   map[string]string
	Vars        map[string]string
	Globals     map[string]string
	Resp        *Resp
	Res         *Resp
	Trace       *Trace
	Stream      *Stream
	Req         *Req
	ReqMut      ReqMut
	VarsMut     VarsMut
	GlobalMut   GlobalMut
	Uses        []Use
	BaseDir     string
	ReadFile    func(string) ([]byte, error)
	AllowRandom bool
	Site        string
	Extensions  Extensions
	Locals      Locals
}

// evalKind selects which binding layers an evaluation gets.
type evalKind uint8

const (
	evalBase evalKind = iota
	evalAssert
)

// Eng evaluates RTS expressions and modules.
// It is not safe for concurrent use because evaluation updates shared request
// state that cached modules resolve dynamically.
type Eng struct {
	C      *ModCache
	Lim    Limits
	Stdlib func() map[string]Value
	reqObj *requestObj
}

// NewEng creates an engine with the provided standard-library prelude.
func NewEng(std func() map[string]Value) *Eng {
	e := &Eng{
		Lim:    defaultLimits(),
		Stdlib: prelude(std),
		reqObj: newRequestObj("request"),
	}
	e.C = NewCache(nil, e.modulePre)
	return e
}

func (e *Eng) Eval(ctx context.Context, rt RT, src string, pos Pos) (Value, error) {
	return e.eval(ctx, rt, src, pos, evalBase)
}

// EvalAssertion evaluates an @assert expression. Assertions add response
// shorthands such as status and header(name) that are not visible anywhere
// else, which is why the host cannot supply them itself.
func (e *Eng) EvalAssertion(ctx context.Context, rt RT, src string, pos Pos) (Value, error) {
	return e.eval(ctx, rt, src, pos, evalAssert)
}

func (e *Eng) eval(ctx context.Context, rt RT, src string, pos Pos, kind evalKind) (Value, error) {
	if e == nil {
		return Null(), fmt.Errorf("nil engine")
	}
	e.ensure()
	if e.reqObj != nil {
		e.reqObj.set(rt.Req)
		e.reqObj.setMut(rt.ReqMut)
	}

	cx := e.newCtx(ctx, rt)
	pre, err := e.buildPre(cx, rt, pos, kind)
	if err != nil {
		return Null(), err
	}

	env := NewEnv(nil)
	for k, v := range pre {
		env.DefConst(k, v)
	}

	ex, err := ParseExpr(pos.Path, pos.Line, pos.Col, src)
	if err != nil {
		return Null(), err
	}

	if rt.Site != "" {
		cx.push(Frame{Kind: FrameExpr, Pos: pos, Name: rt.Site})
		defer cx.pop()
	}

	vm := &VM{ctx: cx}
	v, err := vm.eval(env, ex)
	if err != nil {
		return Null(), err
	}
	return v, nil
}

func (e *Eng) EvalStr(ctx context.Context, rt RT, src string, pos Pos) (string, error) {
	v, err := e.Eval(ctx, rt, src, pos)
	if err != nil {
		return "", err
	}
	cx := NewCtx(ctx, e.Lim)
	return ToStr(cx, pos, v)
}

func (e *Eng) ExecModule(ctx context.Context, rt RT, src string, pos Pos) (*Comp, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	e.ensure()
	if e.reqObj != nil {
		e.reqObj.set(rt.Req)
		e.reqObj.setMut(rt.ReqMut)
		defer e.reqObj.setMut(nil)
	}

	cx := e.newCtx(ctx, rt)
	pre, err := e.buildPre(cx, rt, pos, evalBase)
	if err != nil {
		return nil, err
	}
	mod, err := ParseModuleAt(pos.Path, []byte(src), pos)
	if err != nil {
		return nil, err
	}
	if rt.Site != "" {
		cx.push(Frame{Kind: FrameExpr, Pos: pos, Name: rt.Site})
		defer cx.pop()
	}
	return Exec(cx, mod, pre)
}

func (e *Eng) ensure() {
	if e.reqObj == nil {
		e.reqObj = newRequestObj("request")
	}
	if e.Stdlib == nil {
		e.Stdlib = prelude(nil)
	}
	if e.C == nil {
		e.C = NewCache(nil, e.modulePre)
	}
	e.C.SetStdlib(e.modulePre)
}

func (e *Eng) newCtx(ctx context.Context, rt RT) *Ctx {
	cx := NewCtx(ctx, e.Lim)
	if rt.ReadFile != nil {
		cx.ReadFile = rt.ReadFile
	} else {
		cx.ReadFile = os.ReadFile
	}
	cx.BaseDir = rt.BaseDir
	cx.AllowRandom = rt.AllowRandom
	return cx
}

const unboundResponse = "response is not available until the request has run; use last for the previous response"

func responseObj(r *Resp) *respObj {
	if r == nil {
		return newUnboundRespObj("response", unboundResponse)
	}
	return newRespObj("response", r)
}

// buildPre assembles the bindings one evaluation sees. The prelude is the only
// source of truth for what a local may shadow, so binding a new host object
// here is all it takes to make it shadowable.
func (e *Eng) buildPre(cx *Ctx, rt RT, pos Pos, kind evalKind) (map[string]Value, error) {
	p := e.basePre(rt)
	if err := p.addExtensions(cx, pos, rt.Extensions); err != nil {
		return nil, err
	}
	if err := e.addUses(cx, rt, p, pos); err != nil {
		return nil, err
	}
	if err := p.overlayLocals(cx, pos, rt.Locals); err != nil {
		return nil, err
	}
	if kind == evalAssert {
		p.overlayAsserts(rt.Res)
	}
	return p.values, nil
}

func (e *Eng) basePre(rt RT) pre {
	p := pre{values: e.modulePre()}
	p.values["env"] = Obj(newEnvObj(rt.Env, rt.EnvGroups))
	p.values["vars"] = Obj(newVarsObj("vars", rt.Vars, rt.Globals, rt.VarsMut, rt.GlobalMut))
	p.values["last"] = Obj(newRespObj("last", rt.Resp))

	p.values["response"] = Obj(responseObj(rt.Res))
	p.values["trace"] = Obj(newTraceObj(rt.Trace))
	p.values["stream"] = Obj(newStreamObj(rt.Stream))
	return p
}

func (e *Eng) addUses(cx *Ctx, rt RT, p pre, pos Pos) error {
	uses, err := e.resolveUses(cx, rt, p.values, pos)
	if err != nil {
		return err
	}
	for _, u := range uses {
		cp, _, err := e.C.Load(cx, rt.BaseDir, u.Path)
		if err != nil {
			return err
		}
		p.values[u.Alias] = Obj(NewModObj(u.Alias, cp.Exp))
	}
	return nil
}

func (e *Eng) modulePre() map[string]Value {
	pre := cloneVals(e.Stdlib())
	if e.reqObj != nil {
		pre["request"] = Obj(e.reqObj)
	}
	return pre
}

func cloneVals(src map[string]Value) map[string]Value {
	return CloneDict(src)
}
