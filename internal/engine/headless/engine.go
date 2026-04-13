package headless

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

var _ engine.Executor = (*Engine)(nil)

type reqExec interface {
	Execute(*restfile.Document, *restfile.Request, string) (engine.RequestResult, error)
	CollectVariables(
		*restfile.Document,
		*restfile.Request,
		string,
		...map[string]string,
	) map[string]string
	ExecuteWith(
		*restfile.Document,
		*restfile.Request,
		string,
		request.ExecOptions,
	) (engine.RequestResult, error)
	EvalCondition(
		context.Context,
		*restfile.Document,
		*restfile.Request,
		string,
		string,
		*restfile.ConditionSpec,
		map[string]string,
		map[string]rts.Value,
	) (bool, string, error)
	EvalForEachItems(
		context.Context,
		*restfile.Document,
		*restfile.Request,
		string,
		string,
		request.ForEachSpec,
		map[string]string,
		map[string]rts.Value,
	) ([]rts.Value, error)
	EvalValue(
		context.Context,
		*restfile.Document,
		*restfile.Request,
		string,
		string,
		string,
		string,
		rts.Pos,
		map[string]string,
		map[string]rts.Value,
	) (rts.Value, error)
	PosForLine(*restfile.Document, *restfile.Request, int) rts.Pos
	ValueString(context.Context, rts.Pos, rts.Value) (string, error)
}

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
	rq  reqExec
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

func newWithDeps(rq reqExec, rt *rtrun.Runtime, cfg engine.Config) *Engine {
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
	env string,
) (engine.RequestResult, error) {
	return e.ExecuteRequestContext(context.Background(), doc, req, env)
}

func (e *Engine) ExecuteRequestContext(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env string,
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
			Skipped:     out.Skipped,
			Workflow:    out,
		}, nil
	case spec != nil:
		if req.Metadata.Profile != nil {
			return engine.RequestResult{}, errProfileDuringCompare
		}
		out, err := e.ExecuteCompareContext(runCtx(ctx), doc, req, spec, env)
		if err != nil {
			return engine.RequestResult{}, err
		}
		return engine.RequestResult{
			Executed:    req,
			Environment: out.Environment,
			Skipped:     out.Skipped,
			Compare:     out,
		}, nil
	case req.Metadata.Profile != nil && req.GRPC == nil:
		out, err := e.ExecuteProfileContext(runCtx(ctx), doc, req, env)
		if err != nil {
			return engine.RequestResult{}, err
		}
		return engine.RequestResult{
			Executed:    req,
			Environment: out.Environment,
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
	env string,
) (*engine.WorkflowResult, error) {
	return e.ExecuteWorkflowContext(context.Background(), doc, wf, env)
}

func (e *Engine) ExecuteWorkflowContext(
	ctx context.Context,
	doc *restfile.Document,
	wf *restfile.Workflow,
	env string,
) (*engine.WorkflowResult, error) {
	if e == nil || e.rq == nil {
		return nil, nil
	}
	return e.executeWorkflow(runCtx(ctx), doc, wf, env)
}

func (e *Engine) ExecuteCompare(
	doc *restfile.Document,
	req *restfile.Request,
	spec *restfile.CompareSpec,
	env string,
) (*engine.CompareResult, error) {
	return e.ExecuteCompareContext(context.Background(), doc, req, spec, env)
}

func (e *Engine) ExecuteCompareContext(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	spec *restfile.CompareSpec,
	env string,
) (*engine.CompareResult, error) {
	if e == nil || e.rq == nil {
		return nil, nil
	}
	return e.executeCompare(runCtx(ctx), doc, req, spec, env)
}

func (e *Engine) ExecuteProfile(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
) (*engine.ProfileResult, error) {
	return e.ExecuteProfileContext(context.Background(), doc, req, env)
}

func (e *Engine) ExecuteProfileContext(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env string,
) (*engine.ProfileResult, error) {
	if e == nil || e.rq == nil {
		return nil, nil
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

func (e *Engine) env(name string) string {
	return vars.SelectEnv(e.cfg.EnvironmentSet, name, e.cfg.EnvironmentName)
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

func (e *Engine) baseDir(doc *restfile.Document) string {
	if dir := strings.TrimSpace(e.cfg.HTTPOptions.BaseDir); dir != "" {
		return dir
	}
	if p := e.filePath(doc); p != "" {
		return filepath.Dir(p)
	}
	return ""
}

func (e *Engine) history() history.Store {
	if e == nil || e.rt == nil {
		return nil
	}
	return e.rt.History()
}
