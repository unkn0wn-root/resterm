package rts

import (
	"context"
	"fmt"
	"os"
	"sync"
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
	Pos   Pos
}

// EvalConfig describes language-level inputs to one evaluation. Bindings are
// additive layers, locals are lexical overlays, and overrides are the final
// language bindings used by specialized callers such as assertion evaluators.
// Host concepts do not belong here; hosts translate them to RTS values first.
type EvalConfig struct {
	Bindings    []Extensions
	Locals      Locals
	Overrides   Locals
	Uses        []Use
	BaseDir     string
	ReadFile    func(string) ([]byte, error)
	AllowRandom bool
	Site        string
	MaxSteps    int
}

// EngOption configures an evaluator.
type EngOption func(*Eng)

// WithModuleBindings adds immutable bindings visible while modules compile.
// Objects may still resolve evaluation-local state through Ctx when called.
func WithModuleBindings(values Extensions) EngOption {
	return func(e *Eng) {
		e.mod = append(e.mod, values)
	}
}

// Eng evaluates RTS expressions and modules. Evaluations are serialized because
// cached module environments intentionally retain mutable top-level state.
type Eng struct {
	C   *ModCache
	Lim Limits
	// uni is the universe. It is never written once built, so every evaluation
	// shares it.
	uni *Env
	mod []Extensions
	mu  sync.Mutex
}

// NewEng creates an engine that predeclares the names in pre.
func NewEng(pre map[string]Value, opts ...EngOption) *Eng {
	e := &Eng{
		Lim: defaultLimits(),
		uni: newUniverse(pre),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	e.C = NewCache(nil, e.moduleEnv)
	return e
}

func (e *Eng) Eval(ctx context.Context, cfg EvalConfig, src string, pos Pos) (Value, error) {
	if e == nil {
		return Null(), fmt.Errorf("nil engine")
	}
	cx, env, err := e.prepare(ctx, cfg, pos)
	if err != nil {
		return Null(), err
	}

	ex, err := ParseExpr(src, pos)
	if err != nil {
		return Null(), err
	}

	if cfg.Site != "" {
		cx.push(Frame{Kind: FrameExpr, Pos: pos, Name: cfg.Site})
		defer cx.pop()
	}

	vm := &VM{ctx: cx}
	v, err := vm.eval(env, ex)
	if err != nil {
		return Null(), err
	}
	return v, nil
}

// Load loads cfg.Uses as Eval would, without evaluating anything.
func (e *Eng) Load(ctx context.Context, cfg EvalConfig, pos Pos) error {
	if e == nil {
		return fmt.Errorf("nil engine")
	}
	_, _, err := e.prepare(ctx, cfg, pos)
	return err
}

func (e *Eng) EvalStr(ctx context.Context, cfg EvalConfig, src string, pos Pos) (string, error) {
	v, err := e.Eval(ctx, cfg, src, pos)
	if err != nil {
		return "", err
	}
	cx := NewCtx(ctx, e.Lim)
	return ToStr(cx, pos, v)
}

func (e *Eng) ExecModule(ctx context.Context, cfg EvalConfig, src string, pos Pos) (*Comp, error) {
	if e == nil {
		return nil, fmt.Errorf("nil engine")
	}
	cx, env, err := e.prepare(ctx, cfg, pos)
	if err != nil {
		return nil, err
	}
	mod, err := parseModuleAt([]byte(src), pos)
	if err != nil {
		return nil, err
	}
	if cfg.Site != "" {
		cx.push(Frame{Kind: FrameExpr, Pos: pos, Name: cfg.Site})
		defer cx.pop()
	}
	return exec(cx, mod, env)
}

// prepare builds what one evaluation needs. Only this setup reads shared
// engine state, so the lock is released before code runs. A host function can
// then evaluate again without waiting on itself.
func (e *Eng) prepare(ctx context.Context, cfg EvalConfig, pos Pos) (*Ctx, *Env, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cx := e.newCtx(ctx, cfg)
	env, err := e.buildEnv(cx, cfg, pos)
	// Keep module loading on the engine budget; its results are cached.
	if cfg.MaxSteps > 0 {
		cx.Lim.MaxSteps = cfg.MaxSteps
	}
	return cx, env, err
}

func (e *Eng) newCtx(ctx context.Context, cfg EvalConfig) *Ctx {
	cx := NewCtx(ctx, e.Lim)
	if cfg.ReadFile != nil {
		cx.ReadFile = cfg.ReadFile
	} else {
		cx.ReadFile = os.ReadFile
	}
	cx.BaseDir = cfg.BaseDir
	cx.AllowRandom = cfg.AllowRandom
	return cx
}

// buildEnv assembles the top-level scope one evaluation runs in. That scope is
// the only source of truth for what a local may shadow, so binding a new host
// object here is all it takes to make it shadowable.
func (e *Eng) buildEnv(cx *Ctx, cfg EvalConfig, pos Pos) (*Env, error) {
	env := e.moduleEnv()
	for _, bindings := range cfg.Bindings {
		if err := addExtensions(cx, pos, env, bindings); err != nil {
			return nil, err
		}
	}
	if err := e.addUses(cx, cfg, env); err != nil {
		return nil, err
	}
	if err := overlayLocals(cx, pos, env, cfg.Locals); err != nil {
		return nil, err
	}
	if err := overlayLocals(cx, pos, env, cfg.Overrides); err != nil {
		return nil, err
	}
	return env, nil
}

func (e *Eng) addUses(cx *Ctx, cfg EvalConfig, env *Env) error {
	uses, err := e.resolveUses(cx, cfg, env)
	if err != nil {
		return err
	}
	for _, u := range uses {
		cp, err := e.C.Load(cx, cfg.BaseDir, u.Path)
		if err != nil {
			return err
		}
		env.DefConst(u.Alias, Obj(NewModObj(u.Alias, cp.Exp)))
	}
	return nil
}

// moduleEnv returns a new top-level scope with the names every module sees:
// the predeclared names and the module bindings.
func (e *Eng) moduleEnv() *Env {
	env := newTopEnv(e.uni)
	for _, bindings := range e.mod {
		for name, value := range bindings.all() {
			if name != "" {
				env.DefConst(name, value)
			}
		}
	}
	return env
}
