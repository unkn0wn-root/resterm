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
	C      *ModCache
	Lim    Limits
	Stdlib func() map[string]Value
	mod    []Extensions
	mu     sync.Mutex
}

// NewEng creates an engine with the provided standard-library prelude.
func NewEng(std func() map[string]Value, opts ...EngOption) *Eng {
	e := &Eng{
		Lim:    defaultLimits(),
		Stdlib: prelude(std),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(e)
		}
	}
	e.C = NewCache(nil, e.modulePre)
	return e
}

func (e *Eng) Eval(ctx context.Context, cfg EvalConfig, src string, pos Pos) (Value, error) {
	if e == nil {
		return Null(), fmt.Errorf("nil engine")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ensure()
	cx := e.newCtx(ctx, cfg)
	pre, err := e.buildPre(cx, cfg, pos)
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
	e.mu.Lock()
	defer e.mu.Unlock()

	e.ensure()
	cx := e.newCtx(ctx, cfg)
	pre, err := e.buildPre(cx, cfg, pos)
	if err != nil {
		return nil, err
	}
	mod, err := ParseModuleAt(pos.Path, []byte(src), pos)
	if err != nil {
		return nil, err
	}
	if cfg.Site != "" {
		cx.push(Frame{Kind: FrameExpr, Pos: pos, Name: cfg.Site})
		defer cx.pop()
	}
	return Exec(cx, mod, pre)
}

func (e *Eng) ensure() {
	if e.Stdlib == nil {
		e.Stdlib = prelude(nil)
	}
	if e.C == nil {
		e.C = NewCache(nil, e.modulePre)
	}
	e.C.SetStdlib(e.modulePre)
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

// buildPre assembles the bindings one evaluation sees. The prelude is the only
// source of truth for what a local may shadow, so binding a new host object
// here is all it takes to make it shadowable.
func (e *Eng) buildPre(cx *Ctx, cfg EvalConfig, pos Pos) (map[string]Value, error) {
	p := pre{values: e.modulePre()}
	for _, bindings := range cfg.Bindings {
		if err := p.addExtensions(cx, pos, bindings); err != nil {
			return nil, err
		}
	}
	if err := e.addUses(cx, cfg, p, pos); err != nil {
		return nil, err
	}
	if err := p.overlayLocals(cx, pos, cfg.Locals); err != nil {
		return nil, err
	}
	if err := p.overlayLocals(cx, pos, cfg.Overrides); err != nil {
		return nil, err
	}
	return p.values, nil
}

func (e *Eng) addUses(cx *Ctx, cfg EvalConfig, p pre, pos Pos) error {
	uses, err := e.resolveUses(cx, cfg, p.values, pos)
	if err != nil {
		return err
	}
	for _, u := range uses {
		cp, _, err := e.C.Load(cx, cfg.BaseDir, u.Path)
		if err != nil {
			return err
		}
		// Not p.bind: a module reports itself as module:<name>, while the alias
		// is the name the file wrote.
		p.values[u.Alias] = Obj(NewModObj(u.Alias, cp.Exp))
	}
	return nil
}

func (e *Eng) modulePre() map[string]Value {
	out := CloneDict(e.Stdlib())
	for _, bindings := range e.mod {
		for name, value := range bindings.all() {
			if name != "" {
				out[name] = value
			}
		}
	}
	return out
}
