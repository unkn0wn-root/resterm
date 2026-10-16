package rtshost

import (
	"context"
	"errors"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

var errNilEngine = errors.New("nil RTS host engine")

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
		return rts.Null(), errNilEngine
	}
	ctx, st := withRuntime(ctx, rt)
	defer st.end()
	value, err := e.core.Eval(ctx, evalConfig(st), src, pos)
	return value, diagnose(err)
}

// Load loads rt.Uses as an evaluation against rt would.
func (e *Engine) Load(ctx context.Context, rt Runtime, pos rts.Pos) error {
	if e == nil || e.core == nil {
		return errNilEngine
	}
	ctx, st := withRuntime(ctx, rt)
	defer st.end()
	return diagnose(e.core.Load(ctx, evalConfig(st), pos))
}

// EvalAssertion evaluates an assertion with response shorthand overrides.
func (e *Engine) EvalAssertion(
	ctx context.Context,
	rt Runtime,
	src string,
	pos rts.Pos,
) (rts.Value, error) {
	if e == nil || e.core == nil {
		return rts.Null(), errNilEngine
	}
	ctx, st := withRuntime(ctx, rt)
	defer st.end()
	cfg := evalConfig(st)
	cfg.Overrides = assertionOverrides(st, rt.Response)
	value, err := e.core.Eval(ctx, cfg, src, pos)
	return value, diagnose(err)
}

// EvalStr evaluates an expression and applies RTS's explicit result-to-string
// boundary conversion.
func (e *Engine) EvalStr(ctx context.Context, rt Runtime, src string, pos rts.Pos) (string, error) {
	if e == nil || e.core == nil {
		return "", errNilEngine
	}
	ctx, st := withRuntime(ctx, rt)
	defer st.end()
	value, err := e.core.EvalStr(ctx, evalConfig(st), src, pos)
	return value, diagnose(err)
}

// ExecModule executes one module body against rt.
func (e *Engine) ExecModule(
	ctx context.Context,
	rt Runtime,
	src string,
	pos rts.Pos,
) (*rts.Comp, error) {
	if e == nil || e.core == nil {
		return nil, errNilEngine
	}
	ctx, st := withRuntime(ctx, rt)
	defer st.end()
	comp, err := e.core.ExecModule(ctx, evalConfig(st), src, pos)
	return comp, diagnose(err)
}

func evalConfig(st *evalState) rts.EvalConfig {
	rt := st.rt
	bindings := map[string]rts.Value{
		"env":    rts.Obj(newEnvObj(st, rt.Scope)),
		"vars":   rts.Obj(newVarsObj(st, rt.Scope)),
		"last":   rts.Obj(newResponseObj(st, "last", rt.Last)),
		"trace":  rts.Obj(newTraceObj(st, rt.Trace)),
		"stream": rts.Obj(newStreamObj(st, rt.Stream)),
	}
	if rt.Response == nil {
		bindings["response"] = rts.Obj(newUnboundResponseObj(st, "response"))
	} else {
		bindings["response"] = rts.Obj(newResponseObj(st, "response", rt.Response))
	}
	cfg := rts.EvalConfig{
		Bindings:    []rts.Extensions{rts.NewExtensions(bindings), rt.Extensions},
		Locals:      rt.Locals,
		Uses:        rt.Uses,
		BaseDir:     rt.BaseDir,
		ReadFile:    rt.ReadFile,
		AllowRandom: rt.AllowRandom,
		Site:        rt.Site,
	}
	return cfg
}
