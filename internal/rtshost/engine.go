package rtshost

import (
	"context"
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

// Engine translates host runtime state into language-level bindings and
// delegates parsing and execution to the RTS evaluator.
type Engine struct {
	core *rts.Eng
}

// NewEngine creates a host evaluator using std as its standard library.
func NewEngine(std func() map[string]rts.Value) *Engine {
	req := rts.Obj(&requestObj{})
	core := rts.NewEng(std, rts.WithModuleBindings(rts.Extension("request", req)))
	return &Engine{core: core}
}

// Core returns the language evaluator for low-level configuration and tests.
func (e *Engine) Core() *rts.Eng {
	if e == nil {
		return nil
	}
	return e.core
}

// Limits returns the active language resource limits.
func (e *Engine) Limits() rts.Limits {
	if e == nil || e.core == nil {
		return rts.Limits{}
	}
	return e.core.Lim
}

// Eval evaluates one expression against rt.
func (e *Engine) Eval(ctx context.Context, rt Runtime, src string, pos rts.Pos) (rts.Value, error) {
	if e == nil || e.core == nil {
		return rts.Null(), fmt.Errorf("nil RTS host engine")
	}
	return e.core.Eval(withRuntime(ctx, rt), evalConfig(rt, false), src, pos)
}

// EvalAssertion evaluates an assertion with response shorthand overrides.
func (e *Engine) EvalAssertion(
	ctx context.Context,
	rt Runtime,
	src string,
	pos rts.Pos,
) (rts.Value, error) {
	if e == nil || e.core == nil {
		return rts.Null(), fmt.Errorf("nil RTS host engine")
	}
	return e.core.Eval(withRuntime(ctx, rt), evalConfig(rt, true), src, pos)
}

// EvalStr evaluates an expression and applies RTS's explicit result-to-string
// boundary conversion.
func (e *Engine) EvalStr(ctx context.Context, rt Runtime, src string, pos rts.Pos) (string, error) {
	v, err := e.Eval(ctx, rt, src, pos)
	if err != nil {
		return "", err
	}
	cx := rts.NewCtx(ctx, e.Limits())
	return rts.ToStr(cx, pos, v)
}

// ExecModule executes one module body against rt.
func (e *Engine) ExecModule(
	ctx context.Context,
	rt Runtime,
	src string,
	pos rts.Pos,
) (*rts.Comp, error) {
	if e == nil || e.core == nil {
		return nil, fmt.Errorf("nil RTS host engine")
	}
	return e.core.ExecModule(withRuntime(ctx, rt), evalConfig(rt, false), src, pos)
}

func evalConfig(rt Runtime, assertion bool) rts.EvalConfig {
	bindings := map[string]rts.Value{
		"env":    rts.Obj(newEnvObj(rt.Scope)),
		"vars":   rts.Obj(newVarsObj(rt.Scope, rt.VarsMut, rt.GlobalMut)),
		"last":   rts.Obj(newResponseObj("last", rt.Last)),
		"trace":  rts.Obj(newTraceObj(rt.Trace)),
		"stream": rts.Obj(&streamObj{stream: rt.Stream}),
	}
	if rt.Response == nil {
		bindings["response"] = rts.Obj(newUnboundResponseObj("response"))
	} else {
		bindings["response"] = rts.Obj(newResponseObj("response", rt.Response))
	}
	cfg := rts.EvalConfig{
		Bindings:    []rts.Extensions{rts.NewExtensions(bindings), rt.Extensions},
		Locals:      rt.Locals,
		Uses:        rt.Uses,
		BaseDir:     rt.BaseDir,
		ReadFile:    rt.readFile(),
		AllowRandom: rt.AllowRandom,
		Site:        rt.Site,
	}
	if assertion {
		cfg.Overrides = assertionOverrides(rt.Response)
	}
	return cfg
}
