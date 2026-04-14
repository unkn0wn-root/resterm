package ui

import (
	"context"
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
		FilePath:        path,
		Client:          m.client,
		EnvironmentSet:  m.cfg.EnvironmentSet,
		EnvironmentName: m.cfg.EnvironmentName,
		EnvironmentFile: m.cfg.EnvironmentFile,
		HTTPOptions:     m.resolveHTTPOptions(opts),
		GRPCOptions:     m.grpcOptions,
		History:         m.historyStore(),
		SSHManager:      m.sshManager(),
		K8sManager:      m.k8sManager(),
		WorkspaceRoot:   m.workspaceRoot,
		Recursive:       m.workspaceRecursive,
		CompareTargets:  append([]string(nil), m.cfg.CompareTargets...),
		CompareBase:     strings.TrimSpace(m.cfg.CompareBase),
		Registry:        m.registryIndex(),
		Bindings:        m.bindingsMap,
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
	return m.rq
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
	rq := m.requestSvc(opts)
	if rq == nil {
		return nil
	}
	x := mergeRunExtras(xs...)
	return m.runMsg(func(ctx context.Context) tea.Msg {
		res, err := rq.ExecuteWith(doc, req, env, rqeng.ExecOptions{
			Extra:      x,
			Values:     copyRunValues(vals),
			Record:     true,
			Ctx:        ctx,
			AttachSSE:  m.attachSSEHandle,
			AttachWS:   m.attachWebSocketHandle,
			AttachGRPC: m.attachGRPCSession,
		})
		return runReqMsg{res: res, err: err}
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
		for k, v := range x {
			out[k] = v
		}
	}
	return out
}

func copyRunValues(src map[string]rts.Value) map[string]rts.Value {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]rts.Value, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
