package ui

import (
	"context"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine"
	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) runCfg(opts httpclient.Options) engine.Config {
	path := strings.TrimSpace(m.currentFile)
	if path == "" {
		path = strings.TrimSpace(m.cfg.FilePath)
	}
	return engine.Config{
		FilePath:              path,
		Client:                m.client,
		Catalog:               m.ws.cat,
		Selection:             m.ws.sel,
		EnvironmentFile:       m.ws.envFile,
		AllowInteractiveOAuth: true,
		HTTPOptions:           opts,
		GRPCOptions:           m.grpcOptions,
		History:               m.historyStore(),
		SSHManager:            m.sshManager(),
		K8sManager:            m.k8sManager(),
		WorkspaceRoot:         m.ws.root,
		Recursive:             m.ws.recursive,
		Compare:               m.cfg.Compare.Clone(),
		Registry:              m.registryIndex(),
		Bindings:              m.bindingsMap,
		SourceDiagnostics:     true,
		MockInspector:         m.mockInspector(),
	}
}

// runOptions points BaseDir at the file being run. The launch file's directory
// stops meaning anything once another file or workspace is active.
func (m Model) runOptions() httpclient.Options {
	opts := m.cfg.HTTPOptions
	if m.currentFile != "" {
		opts.BaseDir = filepath.Dir(m.currentFile)
	}
	return opts
}

func (m *Model) requestSvc(opts httpclient.Options) *rqeng.Engine {
	rt := m.runtimeSvc()
	if rt == nil {
		return nil
	}
	cfg := m.runCfg(opts)
	if m.rq == nil {
		m.rq = rqeng.New(cfg, rt)
	}
	// m.rq only holds the shared collaborators. Each run takes a view, so a
	// workspace change cannot rewrite the config a request in flight is reading.
	return m.rq.ForRun(cfg, m.lastResponse, m.lastGRPC)
}

type requestRunner interface {
	ExecuteWith(
		doc *restfile.Document,
		req *restfile.Request,
		env vars.Environment,
		opt rqeng.ExecOptions,
	) (engine.RequestResult, error)
}

// This wrapper turns engine callbacks into UI updates. A profile, compare, or
// workflow run shares one wrapper so repeated warnings are only shown once.
type uiRequestEngine struct {
	*rqeng.Engine
	model *Model
	seen  map[string]struct{}
}

func (e *uiRequestEngine) ExecuteWith(
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
	opt rqeng.ExecOptions,
) (engine.RequestResult, error) {
	nextWarning := opt.OnWarning
	opt.OnWarning = func(warning rqeng.Warning) {
		if nextWarning != nil {
			nextWarning(warning)
		}
		e.queueWarning(string(warning))
	}
	opt.AttachSSE = e.model.attachSSEHandle
	opt.AttachWS = e.model.attachWebSocketHandle
	opt.AttachGRPC = e.model.attachGRPCSession
	return e.Engine.ExecuteWith(doc, req, env, opt)
}

// Engine warnings only. Parse warnings have their own status bar segment, and
// sending them here as well would show them twice and lose the progress text.
func (e *uiRequestEngine) queueWarning(text string) {
	if text == "" {
		return
	}
	if e.seen == nil {
		e.seen = make(map[string]struct{})
	}
	if _, seen := e.seen[text]; seen {
		return
	}
	e.seen[text] = struct{}{}

	emitQueuedMsg(e.model.runMsgChan, runWarningMsg{text: text})
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

// runBlocked refuses to execute while no environment is selected. Every path
// onto the wire passes this one rule.
func (m *Model) runBlocked() tea.Cmd {
	if !m.ws.unselected {
		return nil
	}
	return statusCmd(statusWarn, noEnvSelected+". Choose one with Ctrl+E")
}

// runSpec describes one request execution. Preview mode bypasses the UI
// wrapper because it must not attach live streams to the model.
type runSpec struct {
	doc    *restfile.Document
	req    *restfile.Request
	opts   httpclient.Options
	sel    vars.Selection
	mode   rqeng.ExecMode
	record bool
}

// startRun is the single path from a prepared request to a running command.
// The second result reports whether a run actually started, so callers never
// begin progress indicators for a request that will produce no response.
func (m *Model) startRun(sp runSpec) (tea.Cmd, bool) {
	if cmd := m.runBlocked(); cmd != nil {
		return cmd, false
	}

	env, err := m.environment(sp.sel)
	if err != nil {
		return responseErrCmd(err, sp.req, ""), false
	}
	if err := docErr(sp.doc); err != nil {
		return responseErrCmd(err, sp.req, env.Label()), false
	}

	svc := m.requestSvc(sp.opts)
	if svc == nil {
		return nil, false
	}
	var rq requestRunner = svc
	if sp.mode == rqeng.ExecModeSend {
		rq = &uiRequestEngine{Engine: svc, model: m}
	}
	gen := m.latencySeries.generation()
	return m.runMsg(func(ctx context.Context) tea.Msg {
		res, err := rq.ExecuteWith(sp.doc, sp.req, env, rqeng.ExecOptions{
			Record: sp.record,
			Ctx:    ctx,
			Mode:   sp.mode,
		})
		if sp.record {
			return runReqMsg{res: res, err: err, latGen: gen}
		}
		if err != nil {
			return responseMsg{
				err:         err,
				executed:    sp.req.Clone(),
				environment: env.Label(),
			}
		}
		msg := m.responseMsgFromRunState(res, false)
		msg.latGen = gen
		return msg
	}), true
}

func responseErrCmd(err error, req *restfile.Request, env string) tea.Cmd {
	return func() tea.Msg {
		return responseMsg{
			err:         err,
			executed:    req.Clone(),
			environment: env,
		}
	}
}
