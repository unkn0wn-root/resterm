package rts

import (
	"context"
	"fmt"
	"os"
)

type Use struct {
	Path  string
	Alias string
}

type RT struct {
	Env         map[string]string
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
	Extra       map[string]Value
}

type Eng struct {
	C      *ModCache
	Lim    Limits
	Stdlib func() map[string]Value
	reqObj *requestObj
}

func NewEng() *Eng {
	e := &Eng{
		C:      NewCache(nil),
		Lim:    Limits{MaxSteps: 10000, MaxCall: 64, MaxStr: 65536, MaxList: 2000, MaxDict: 2000},
		reqObj: newRequestObj("request"),
	}
	e.Stdlib = func() map[string]Value {
		return buildStdlib(e.reqObj)
	}
	e.C.SetStdlib(e.Stdlib)
	return e
}

func (e *Eng) ensure() {
	if e.reqObj == nil {
		e.reqObj = newRequestObj("request")
	}
	if e.Stdlib == nil {
		e.Stdlib = func() map[string]Value {
			return buildStdlib(e.reqObj)
		}
	}
	if e.C == nil {
		e.C = NewCache(nil)
	}
	e.C.SetStdlib(e.Stdlib)
}

func (e *Eng) Eval(ctx context.Context, rt RT, src string, pos Pos) (Value, error) {
	if e == nil {
		return Null(), fmt.Errorf("nil engine")
	}
	e.ensure()
	if e.reqObj != nil {
		e.reqObj.set(rt.Req)
		e.reqObj.setMut(rt.ReqMut)
	}

	cx := e.newCtx(ctx, rt)
	pre, err := e.buildPre(cx, rt, pos)
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
	return toStr(cx, pos, v)
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
	pre, err := e.buildPre(cx, rt, pos)
	if err != nil {
		return nil, err
	}
	mod, err := ParseModule(pos.Path, []byte(src))
	if err != nil {
		return nil, err
	}
	if rt.Site != "" {
		cx.push(Frame{Kind: FrameExpr, Pos: pos, Name: rt.Site})
		defer cx.pop()
	}
	return Exec(cx, mod, pre)
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

func (e *Eng) buildPre(cx *Ctx, rt RT, pos Pos) (map[string]Value, error) {
	pre := cloneVals(e.Stdlib())
	pre["env"] = Obj(newMapObj("env", rt.Env))
	pre["vars"] = Obj(newVarsObj("vars", rt.Vars, rt.Globals, rt.VarsMut, rt.GlobalMut))
	pre["last"] = Obj(newRespObj("last", rt.Resp))

	res := rt.Res
	if res == nil {
		res = rt.Resp
	}
	pre["response"] = Obj(newRespObj("response", res))
	pre["trace"] = Obj(newTraceObj(rt.Trace))
	pre["stream"] = Obj(newStreamObj(rt.Stream))

	for k, v := range rt.Extra {
		if k == "" {
			continue
		}
		if _, ok := pre[k]; ok {
			return nil, rtErr(cx, pos, "name already defined: %s", k)
		}
		pre[k] = v
	}

	for _, u := range rt.Uses {
		if u.Alias == "" {
			return nil, rtErr(cx, pos, "missing module alias")
		}
		if _, ok := pre[u.Alias]; ok {
			return nil, rtErr(cx, pos, "alias already defined: %s", u.Alias)
		}
		comp, _, err := e.C.Load(cx, rt.BaseDir, u.Path)
		if err != nil {
			return nil, err
		}
		pre[u.Alias] = Obj(NewModObj(u.Alias, comp.Exp))
	}
	return pre, nil
}

func cloneVals(src map[string]Value) map[string]Value {
	return cloneMap(src)
}
