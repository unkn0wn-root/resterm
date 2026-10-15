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
	value, err := e.core.Eval(withRuntime(ctx, rt), evalConfig(rt), src, pos)
	return value, diagnose(err)
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
	cfg := evalConfig(rt)
	cfg.Overrides = assertionOverrides(rt.Response)
	value, err := e.core.Eval(withRuntime(ctx, rt), cfg, src, pos)
	return value, diagnose(err)
}

// EvalStr evaluates an expression and applies RTS's explicit result-to-string
// boundary conversion.
func (e *Engine) EvalStr(ctx context.Context, rt Runtime, src string, pos rts.Pos) (string, error) {
	if e == nil || e.core == nil {
		return "", errNilEngine
	}
	value, err := e.core.EvalStr(withRuntime(ctx, rt), evalConfig(rt), src, pos)
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
	comp, err := e.core.ExecModule(withRuntime(ctx, rt), evalConfig(rt), src, pos)
	return comp, diagnose(err)
}

func evalConfig(rt Runtime) rts.EvalConfig {
	// Assign the interfaces only when Mutator is set. A nil pointer stored in an
	// interface is non-nil and could allow a read-only write to panic.
	var varsMut VarsMutator
	var globalMut GlobalMutator
	if rt.Mutator != nil {
		varsMut, globalMut = rt.Mutator, rt.Mutator
	}
	bindings := map[string]rts.Value{
		"env":    rts.Obj(newEnvObj(rt.Scope)),
		"vars":   rts.Obj(newVarsObj(rt.Scope, varsMut, globalMut)),
		"last":   rts.Obj(newResponseObj("last", rt.Last)),
		"trace":  rts.Obj(newTraceObj(rt.Trace)),
		"stream": rts.Obj(newStreamObj(rt.Stream)),
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
		ReadFile:    rt.ReadFile,
		AllowRandom: rt.AllowRandom,
		Site:        rt.Site,
	}
	return cfg
}
