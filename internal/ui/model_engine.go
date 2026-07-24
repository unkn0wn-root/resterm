package ui

import (
	"context"
	"maps"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine"
	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func (m *Model) runCfg(opts httpclient.Options) engine.Config {
	path := strings.TrimSpace(m.currentFile)
	if path == "" {
		path = strings.TrimSpace(m.cfg.FilePath)
	}
	return engine.Config{
		FilePath:              path,
		Client:                m.client,
		EnvironmentSet:        m.cfg.EnvironmentSet,
		EnvironmentName:       m.cfg.EnvironmentName,
		EnvironmentFile:       m.cfg.EnvironmentFile,
		AllowInteractiveOAuth: true,
		HTTPOptions:           opts,
		GRPCOptions:           m.grpcOptions,
		History:               m.historyStore(),
		SSHManager:            m.sshManager(),
		K8sManager:            m.k8sManager(),
		WorkspaceRoot:         m.workspaceRoot,
		Recursive:             m.workspaceRecursive,
		CompareTargets:        append([]string(nil), m.cfg.CompareTargets...),
		CompareBase:           strings.TrimSpace(m.cfg.CompareBase),
		Registry:              m.registryIndex(),
		Bindings:              m.bindingsMap,
		SourceDiagnostics:     true,
		MockInspector:         m.mockInspector(),
	}
}

func (m *Model) requestSvc(opts httpclient.Options) *rqeng.Engine {
	rt := m.runtimeSvc()
	if rt == nil {
		return nil
	}
	cfg := m.runCfg(opts)
	if m.rq == nil {
		m.rq = rqeng.New(cfg, rt)
	} else {
		m.rq.SetConfig(cfg)
	}
	m.rq.SeedLast(m.lastResponse, m.lastGRPC)
	return m.rq
}

type requestRunner interface {
	ExecuteWith(
		doc *restfile.Document,
		req *restfile.Request,
		env string,
		opt rqeng.ExecOptions,
	) (engine.RequestResult, error)
}

// This wrapper turns engine callbacks into UI updates. A profile, compare, or
// workflow run shares one wrapper so repeated warnings are only shown once.
type uiRequestEngine struct {
	*rqeng.Engine
	model        *Model
	seenWarnings map[rqeng.Warning]struct{}
}

func (e *uiRequestEngine) ExecuteWith(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	opt rqeng.ExecOptions,
) (engine.RequestResult, error) {
	nextWarning := opt.OnWarning
	opt.OnWarning = func(warning rqeng.Warning) {
		if nextWarning != nil {
			nextWarning(warning)
		}
		e.queueWarning(warning)
	}
	opt.AttachSSE = e.model.attachSSEHandle
	opt.AttachWS = e.model.attachWebSocketHandle
	opt.AttachGRPC = e.model.attachGRPCSession
	return e.Engine.ExecuteWith(doc, req, env, opt)
}

func (e *uiRequestEngine) queueWarning(warning rqeng.Warning) {
	if e.seenWarnings == nil {
		e.seenWarnings = make(map[rqeng.Warning]struct{})
	}
	if _, seen := e.seenWarnings[warning]; seen {
		return
	}
	e.seenWarnings[warning] = struct{}{}

	emitQueuedMsg(e.model.runMsgChan, requestWarningMsg{warning: warning})
}

func (m *Model) runRequestSvc(opts httpclient.Options) *uiRequestEngine {
	rq := m.requestSvc(opts)
	if rq == nil {
		return nil
	}
	return &uiRequestEngine{Engine: rq, model: m}
}

func (m *Model) runMsg(fn func(context.Context) tea.Msg) tea.Cmd {
	if fn == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.sendCancel = cancel
	return func() tea.Msg {
		defer cancel()
		return fn(ctx)
	}
}

func (m *Model) execRunReq(
	doc *restfile.Document,
	req *restfile.Request,
	opts httpclient.Options,
	env string,
	vals map[string]rts.Value,
	xs ...map[string]string,
) tea.Cmd {
	if err := docErr(doc); err != nil {
		return responseErrCmd(err, req, env)
	}
	rq := m.runRequestSvc(opts)
	if rq == nil {
		return nil
	}
	x := mergeRunExtras(xs...)
	gen := m.latencySeries.generation()
	return m.runMsg(func(ctx context.Context) tea.Msg {
		res, err := rq.ExecuteWith(doc, req, env, rqeng.ExecOptions{
			Extra:  x,
			Values: copyRunValues(vals),
			Record: true,
			Ctx:    ctx,
		})
		return runReqMsg{res: res, err: err, latGen: gen}
	})
}

func mergeRunExtras(xs ...map[string]string) map[string]string {
	n := 0
	for _, x := range xs {
		n += len(x)
	}
	if n == 0 {
		return nil
	}
	out := make(map[string]string, n)
	for _, x := range xs {
		maps.Copy(out, x)
	}
	return out
}

func copyRunValues(src map[string]rts.Value) map[string]rts.Value {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]rts.Value, len(src))
	maps.Copy(out, src)
	return out
}
