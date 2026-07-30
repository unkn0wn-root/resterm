package headless

import (
	"context"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

var _ engine.Executor = (*Engine)(nil)

type repo[T any] struct {
	snap func() T
	load func(T)
}

func (r repo[T]) Snapshot() T {
	if r.snap == nil {
		var z T
		return z
	}
	return r.snap()
}

func (r repo[T]) Restore(v T) {
	if r.load == nil {
		return
	}
	r.load(v)
}

type Engine struct {
	rq  core.Dep
	cfg engine.Config
	rt  *rtrun.Runtime
	rs  repo[engine.RuntimeState]
	at  repo[engine.AuthState]
	cl  interface{ Close() error }
}

func New(cfg engine.Config) *Engine {
	rt := rtrun.New(rtrun.Config{
		Client:     cfg.Client,
		History:    cfg.History,
		SSHManager: cfg.SSHManager,
		K8sManager: cfg.K8sManager,
	})
	rq := request.New(cfg, rt)
	return newWithDeps(rq, rt, cfg)
}

func newWithDeps(rq core.Dep, rt *rtrun.Runtime, cfg engine.Config) *Engine {
	if rq == nil || rt == nil {
		return &Engine{}
	}
	return &Engine{
		rq:  rq,
		cfg: cfg,
		rt:  rt,
		rs: repo[engine.RuntimeState]{
			snap: rt.RuntimeState,
			load: rt.LoadRuntimeState,
		},
		at: repo[engine.AuthState]{
			snap: rt.AuthState,
			load: rt.LoadAuthState,
		},
		cl: rt,
	}
}

func (e *Engine) ExecuteRequest(
	doc *restfile.Document,
	req *restfile.Request,
	sel vars.Selection,
) (engine.RequestResult, error) {
	return e.ExecuteRequestContext(context.Background(), doc, req, sel)
}

func (e *Engine) ExecuteRequestContext(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	sel vars.Selection,
) (engine.RequestResult, error) {
	if e == nil || e.rq == nil {
		return engine.RequestResult{}, nil
	}
	if req == nil {
		return engine.RequestResult{}, errNilRequest
	}
	if req.WebSocket != nil && len(req.WebSocket.Steps) == 0 {
		return engine.RequestResult{}, errInteractiveWebSocket
	}
	env, err := e.environment(sel)
	if err != nil {
		return engine.RequestResult{}, err
	}

	spec := e.compareSpec(req)
	switch {
	case req.Metadata.ForEach != nil:
		if spec != nil {
			return engine.RequestResult{}, errCompareWithForEach
		}
		if req.Metadata.Profile != nil {
			return engine.RequestResult{}, errProfileWithForEach
		}
		out, err := e.executeForEach(runCtx(ctx), doc, req, env)
		if err != nil {
			return engine.RequestResult{}, err
		}
		return engine.RequestResult{
			Executed:    request.CloneRequest(req),
			Environment: out.Environment,
			Selection:   out.Selection,
			Skipped:     out.Skipped,
			Workflow:    out,
		}, nil
	case spec != nil:
		if req.Metadata.Profile != nil {
			return engine.RequestResult{}, errProfileDuringCompare
		}
		out, err := e.executeCompare(runCtx(ctx), doc, req, spec, env)
		if err != nil {
			return engine.RequestResult{}, err
		}
		return engine.RequestResult{
			Executed:    req,
			Environment: out.Environment,
			Selection:   out.Selection,
			Skipped:     out.Skipped,
			Compare:     out,
		}, nil
	case req.Metadata.Profile != nil && req.GRPC == nil:
		out, err := e.executeProfile(runCtx(ctx), doc, req, env)
		if err != nil {
			return engine.RequestResult{}, err
		}
		return engine.RequestResult{
			Executed:    req,
			Environment: out.Environment,
			Selection:   out.Selection,
			Skipped:     out.Skipped,
			SkipReason:  out.SkipReason,
			Profile:     out,
		}, nil
	default:
		return e.rq.ExecuteWith(doc, req, env, request.ExecOptions{
			Record: true,
			Ctx:    runCtx(ctx),
		})
	}
}

func (e *Engine) ExecuteWorkflow(
	doc *restfile.Document,
	wf *restfile.Workflow,
	sel vars.Selection,
) (*engine.WorkflowResult, error) {
	return e.ExecuteWorkflowContext(context.Background(), doc, wf, sel)
}

func (e *Engine) ExecuteWorkflowContext(
	ctx context.Context,
	doc *restfile.Document,
	wf *restfile.Workflow,
	sel vars.Selection,
) (*engine.WorkflowResult, error) {
	if e == nil || e.rq == nil {
		return nil, nil
	}
	env, err := e.environment(sel)
	if err != nil {
		return nil, err
	}
	return e.executeWorkflow(runCtx(ctx), doc, wf, env)
}

func (e *Engine) ExecuteCompare(
	doc *restfile.Document,
	req *restfile.Request,
	spec *restfile.CompareSpec,
	sel vars.Selection,
) (*engine.CompareResult, error) {
	return e.ExecuteCompareContext(context.Background(), doc, req, spec, sel)
}

func (e *Engine) ExecuteCompareContext(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	spec *restfile.CompareSpec,
	sel vars.Selection,
) (*engine.CompareResult, error) {
	if e == nil || e.rq == nil {
		return nil, nil
	}
	env, err := e.environment(sel)
	if err != nil {
		return nil, err
	}
	return e.executeCompare(runCtx(ctx), doc, req, spec, env)
}

func (e *Engine) ExecuteProfile(
	doc *restfile.Document,
	req *restfile.Request,
	sel vars.Selection,
) (*engine.ProfileResult, error) {
	return e.ExecuteProfileContext(context.Background(), doc, req, sel)
}

func (e *Engine) ExecuteProfileContext(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	sel vars.Selection,
) (*engine.ProfileResult, error) {
	if e == nil || e.rq == nil {
		return nil, nil
	}
	env, err := e.environment(sel)
	if err != nil {
		return nil, err
	}
	return e.executeProfile(runCtx(ctx), doc, req, env)
}

func (e *Engine) RuntimeState() engine.RuntimeState {
	if e == nil {
		return engine.RuntimeState{}
	}
	return e.rs.Snapshot()
}

func (e *Engine) LoadRuntimeState(st engine.RuntimeState) {
	if e == nil {
		return
	}
	e.rs.Restore(st)
}

func (e *Engine) AuthState() engine.AuthState {
	if e == nil {
		return engine.AuthState{}
	}
	return e.at.Snapshot()
}

func (e *Engine) LoadAuthState(st engine.AuthState) {
	if e == nil {
		return
	}
	e.at.Restore(st)
}

func (e *Engine) Close() error {
	if e == nil || e.cl == nil {
		return nil
	}
	return e.cl.Close()
}

func (e *Engine) environment(sel vars.Selection) (vars.Environment, error) {
	if sel.Empty() {
		sel = e.cfg.Selection
	}
	return e.cfg.Catalog.Resolve(sel)
}

func runCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (e *Engine) filePath(doc *restfile.Document) string {
	if doc != nil && strings.TrimSpace(doc.Path) != "" {
		return doc.Path
	}
	return strings.TrimSpace(e.cfg.FilePath)
}

func (e *Engine) history() history.Store {
	if e == nil || e.rt == nil {
		return nil
	}
	return e.rt.History()
}
