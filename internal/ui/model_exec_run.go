package ui

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	xexec "github.com/unkn0wn-root/resterm/internal/exec"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/settings"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/tracebudget"
	"github.com/unkn0wn-root/resterm/internal/tunnel"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type execContext struct {
	// Immutable execution inputs.
	model          *Model
	doc            *restfile.Document
	req            *restfile.Request
	envName        string
	options        httpclient.Options
	extraVals      map[string]rts.Value
	extras         []map[string]string
	runtimeSecrets []string

	// Execution services and lifetime control.
	client     *httpclient.Client
	runner     *scripts.Runner
	sendCtx    context.Context
	sendCancel context.CancelFunc

	// State derived before request preparation.
	baseVars     map[string]string
	storeGlobals map[string]scripts.GlobalValue
	hasRTSPre    bool
	hasJSPre     bool
	scriptVars   map[string]string

	// State populated during request preparation.
	resolver       *vars.Resolver
	trace          *vars.Trace
	mergedSettings map[string]string
	sshPlan        *ssh.Plan
	k8sPlan        *k8s.Plan

	// Execution mode selected from the prepared request.
	useGRPC          bool
	grpcOpts         grpcclient.Options
	effectiveTimeout time.Duration

	// Explain/report state shared across phases.
	explain *explainBuilder
}

func newExecContext(
	m *Model,
	doc *restfile.Document,
	req *restfile.Request,
	options httpclient.Options,
	envName string,
	extraVals map[string]rts.Value,
	extras []map[string]string,
) *execContext {
	m.syncRegistry(doc)
	client := m.client
	if client == nil {
		client = httpclient.NewClient(nil)
	}
	runner := m.scriptRunner
	if runner == nil {
		runner = scripts.NewRunner(nil)
	}
	sendCtx, sendCancel := context.WithCancel(context.Background())
	m.sendCancel = sendCancel

	baseVars := m.collectVariables(doc, req, envName)
	applyExtraVariables(baseVars, extras)

	hasRTSPre, hasJSPre := detectPreRequestScripts(req)
	explain := newExplainBuilder(m, req, envName, false)
	if req != nil &&
		(req.Metadata.When != nil || len(req.Metadata.Applies) > 0 || hasRTSPre || hasJSPre) {
		explain.warn(
			"Variable trace covers template resolution only; RTS/JS script internals are not traced",
		)
	}

	storeGlobals := m.collectStoredGlobalValues(envName)
	explain.globals = effectiveGlobalValues(doc, storeGlobals)

	return &execContext{
		model:        m,
		doc:          doc,
		req:          req,
		options:      options,
		envName:      envName,
		extraVals:    extraVals,
		extras:       extras,
		client:       client,
		runner:       runner,
		sendCtx:      sendCtx,
		sendCancel:   sendCancel,
		baseVars:     baseVars,
		storeGlobals: storeGlobals,
		hasRTSPre:    hasRTSPre,
		hasJSPre:     hasJSPre,
		explain:      explain,
	}
}

func detectPreRequestScripts(req *restfile.Request) (bool, bool) {
	if req == nil {
		return false, false
	}
	hasRTSPre := false
	hasJSPre := false
	for _, block := range req.Metadata.Scripts {
		if isRTSPre(block) {
			hasRTSPre = true
		}
		if strings.ToLower(block.Kind) == "pre-request" && scriptLang(block.Lang) == "js" {
			hasJSPre = true
		}
	}
	return hasRTSPre, hasJSPre
}

func applyExtraVariables(base map[string]string, extras []map[string]string) {
	if base == nil || len(extras) == 0 {
		return
	}
	for _, extra := range extras {
		for key, value := range extra {
			if key == "" {
				continue
			}
			base[key] = value
		}
	}
}

// Accept an environment override so compare sweeps can force a per-iteration
// scope without mutating the global environment selection.
func (m *Model) executeRequest(
	doc *restfile.Document,
	req *restfile.Request,
	options httpclient.Options,
	envOverride string,
	extraVals map[string]rts.Value,
	extras ...map[string]string,
) tea.Cmd {
	options, envName, cmd := m.requestSetup(req, options, envOverride, false)
	if cmd != nil {
		return cmd
	}
	if req != nil && req.WebSocket != nil && len(req.WebSocket.Steps) == 0 {
		exec := newExecContext(m, doc, req, options, envName, extraVals, extras)
		return exec.cmdInteractive()
	}

	rq := m.requestSvc(options)
	if rq == nil {
		return nil
	}
	x := mergeRunExtras(extras...)
	return m.runMsg(func(ctx context.Context) tea.Msg {
		res, err := rq.ExecuteWith(doc, req, envName, rqeng.ExecOptions{
			Extra:      x,
			Values:     copyRunValues(extraVals),
			Record:     false,
			Ctx:        ctx,
			AttachSSE:  m.attachSSEHandle,
			AttachWS:   m.attachWebSocketHandle,
			AttachGRPC: m.attachGRPCSession,
		})
		if err != nil {
			return responseMsg{
				err:         err,
				executed:    cloneRequest(req),
				environment: envName,
			}
		}
		return m.responseMsgFromRunState(res, false)
	})
}

func (m *Model) executeExplain(
	doc *restfile.Document,
	req *restfile.Request,
	options httpclient.Options,
	envOverride string,
	extraVals map[string]rts.Value,
	extras ...map[string]string,
) tea.Cmd {
	options, envName, cmd := m.requestSetup(req, options, envOverride, true)
	if cmd != nil {
		return cmd
	}
	rq := m.requestSvc(options)
	if rq == nil {
		return nil
	}
	x := mergeRunExtras(extras...)
	return m.runMsg(func(ctx context.Context) tea.Msg {
		res, err := rq.ExecuteWith(doc, req, envName, rqeng.ExecOptions{
			Extra:  x,
			Values: copyRunValues(extraVals),
			Record: false,
			Ctx:    ctx,
			Mode:   rqeng.ExecModePreview,
		})
		if err != nil {
			return responseMsg{
				err:         err,
				executed:    cloneRequest(req),
				environment: envName,
			}
		}
		return m.responseMsgFromRunState(res, false)
	})
}

func (m *Model) requestSetup(
	req *restfile.Request,
	options httpclient.Options,
	envOverride string,
	preview bool,
) (httpclient.Options, string, tea.Cmd) {
	options = m.resolveHTTPOptions(options)
	envName := vars.SelectEnv(m.cfg.EnvironmentSet, envOverride, m.cfg.EnvironmentName)
	if options.CookieJar == nil {
		if cs := m.cookieStore(); cs != nil {
			options.CookieJar = cs.Jar(envName)
		}
	}
	if req == nil {
		err := errdef.New(errdef.CodeUI, "request is nil")
		return options, envName, func() tea.Msg {
			return responseMsg{
				err:         err,
				environment: envName,
			}
		}
	}
	if tunnel.HasConflict(req.SSH != nil, req.K8s != nil) {
		err := errdef.New(errdef.CodeHTTP, "@ssh cannot be combined with @k8s")
		explain := newExplainBuilder(m, req, envName, preview)
		explain.stage(
			explainStageRoute,
			xplain.StageError,
			explainSummaryRouteConfigInvalid,
			nil,
			nil,
			err.Error(),
		)
		rep := explain.finish(xplain.StatusError, "Route resolution failed", err)
		return options, envName, func() tea.Msg {
			return responseMsg{
				err:         err,
				executed:    req,
				environment: envName,
				explain:     rep,
			}
		}
	}

	if req.Metadata.Trace != nil && req.Metadata.Trace.Enabled {
		options.Trace = true
		if budget, ok := tracebudget.FromSpec(req.Metadata.Trace); ok {
			options.TraceBudget = &budget
		}
	}
	return options, envName, nil
}

func (e *execContext) cmdInteractive() tea.Cmd {
	return func() tea.Msg {
		return e.runInteractive()
	}
}

func (e *execContext) runInteractive() tea.Msg {
	return responseMsgFromExecResult(xexec.RunRequest(interactiveExecFlow{ctx: e}))
}

func (e *execContext) finish() {
	if e.sendCancel != nil {
		e.sendCancel()
	}
}

func (e *execContext) baseResponse() responseMsg {
	return responseMsg{
		executed:       e.req,
		requestText:    "",
		runtimeSecrets: append([]string(nil), e.runtimeSecrets...),
		environment:    e.envName,
	}
}

func (e *execContext) requestText() string {
	return renderRequestText(e.req)
}

func (e *execContext) errorResponse(cause error, decision string) responseMsg {
	msg := e.baseResponse()
	msg.err = cause
	msg.explain = e.explain.finish(xplain.StatusError, decision, cause)
	return msg
}

func (e *execContext) pendingCancel() *responseMsg {
	select {
	case <-e.sendCtx.Done():
		msg := e.errorResponse(context.Canceled, "Request canceled")
		return &msg
	default:
		return nil
	}
}

func (e *execContext) canceledResponse(err error) *responseMsg {
	msg := e.errorResponse(err, "Request canceled")
	return &msg
}

func (e *execContext) currentVariables() map[string]string {
	current := e.model.collectVariablesWithStoreGlobals(
		e.doc,
		e.req,
		e.envName,
		e.storeGlobals,
	)
	applyExtraVariables(current, e.extras)
	return current
}

func (e *execContext) currentGlobalValues() map[string]scripts.GlobalValue {
	return effectiveGlobalValues(e.doc, e.storeGlobals)
}

func (e *execContext) applyRuntimeGlobals(changes map[string]scripts.GlobalValue) {
	if len(changes) == 0 {
		return
	}
	e.model.applyGlobalMutations(changes, e.envName)
	e.storeGlobals = e.model.collectStoredGlobalValues(e.envName)
	e.explain.globals = e.currentGlobalValues()
}

func (e *execContext) evaluateCondition() *responseMsg {
	if e.req.Metadata.When == nil {
		return nil
	}

	shouldRun, reason, err := e.model.evalCondition(
		e.sendCtx,
		e.doc,
		e.req,
		e.envName,
		e.options.BaseDir,
		e.req.Metadata.When,
		e.baseVars,
		e.extraVals,
	)
	if err != nil {
		tag := "@when"
		if e.req.Metadata.When.Negate {
			tag = "@skip-if"
		}
		e.explain.stage(
			tag,
			xplain.StageError,
			explainSummaryConditionEvaluationFailed,
			nil,
			nil,
			err.Error(),
		)

		msg := e.errorResponse(err, "Condition evaluation failed")
		msg.err = errdef.Wrap(errdef.CodeScript, err, "%s", tag)
		return &msg
	}

	stageStatus := xplain.StageOK
	summary := explainSummaryConditionPassed
	if !shouldRun {
		stageStatus = xplain.StageSkipped
		summary = explainSummaryConditionBlockedRequest
	}
	e.explain.stage(explainStageCondition, stageStatus, summary, nil, nil, reason)
	if shouldRun {
		return nil
	}

	msg := e.baseResponse()
	msg.requestText = e.requestText()
	msg.skipped = true
	msg.skipReason = reason
	msg.explain = e.explain.finish(xplain.StatusSkipped, reason, nil)
	return &msg
}

func (e *execContext) runPreRequestScripts() *responseMsg {
	e.model.resolveInheritedAuth(e.doc, e.req)
	preVars := cloneStringMap(e.baseVars)
	applyBefore := cloneRequestIf(e.req, len(e.req.Metadata.Applies) > 0)
	if err := e.model.runRTSApply(
		e.sendCtx,
		e.doc,
		e.req,
		e.envName,
		e.options.BaseDir,
		preVars,
		e.extraVals,
	); err != nil {
		e.explain.stage(
			explainStageApply,
			xplain.StageError,
			explainSummaryApplyFailed,
			applyBefore,
			e.req,
			err.Error(),
		)

		msg := e.errorResponse(err, "Apply failed")
		msg.err = errdef.Wrap(errdef.CodeScript, err, "@apply")
		return &msg
	}
	if len(e.req.Metadata.Applies) > 0 {
		e.explain.stage(
			explainStageApply,
			xplain.StageOK,
			explainSummaryApplyComplete,
			applyBefore,
			e.req,
		)
	}

	rtsBefore := cloneRequestIf(e.req, e.hasRTSPre)
	rtsResult, err := e.model.runRTSPreRequest(
		e.sendCtx,
		e.doc,
		e.req,
		e.envName,
		e.options.BaseDir,
		preVars,
		cloneGlobalValues(e.currentGlobalValues()),
	)
	if err != nil {
		e.explain.stage(
			explainStageRTSPreRequest,
			xplain.StageError,
			explainSummaryRTSPreRequestFailed,
			rtsBefore,
			e.req,
			err.Error(),
		)

		msg := e.errorResponse(err, "RTS pre-request failed")
		msg.err = errdef.Wrap(errdef.CodeScript, err, "pre-request rts script")
		return &msg
	}
	if err := applyPreRequestOutput(e.req, rtsResult); err != nil {
		e.explain.stage(
			explainStageRTSPreRequest,
			xplain.StageError,
			explainSummaryRTSPreRequestOutputBad,
			rtsBefore,
			e.req,
			err.Error(),
		)

		msg := e.errorResponse(err, "RTS pre-request failed")
		return &msg
	}
	if e.hasRTSPre {
		e.explain.stage(
			explainStageRTSPreRequest,
			xplain.StageOK,
			explainSummaryRTSPreRequestComplete,
			rtsBefore,
			e.req,
		)
	}
	if err := e.sendCtx.Err(); err != nil {
		return e.canceledResponse(err)
	}

	e.applyRuntimeGlobals(rtsResult.Globals)
	if len(rtsResult.Globals) > 0 || len(rtsResult.Variables) > 0 {
		preVars = e.currentVariables()
	}

	jsBefore := cloneRequestIf(e.req, e.hasJSPre)
	preResult, err := e.runner.RunPreRequest(e.req.Metadata.Scripts, scripts.PreRequestInput{
		Request:   e.req,
		Variables: preVars,
		Globals:   cloneGlobalValues(e.currentGlobalValues()),
		BaseDir:   e.options.BaseDir,
		Context:   e.sendCtx,
	})
	if err != nil {
		e.explain.stage(
			explainStageJSPreRequest,
			xplain.StageError,
			explainSummaryJSPreRequestFailed,
			jsBefore,
			e.req,
			err.Error(),
		)

		msg := e.errorResponse(err, "JS pre-request failed")
		msg.err = errdef.Wrap(errdef.CodeScript, err, "pre-request script")
		return &msg
	}
	if err := applyPreRequestOutput(e.req, preResult); err != nil {
		e.explain.stage(
			explainStageJSPreRequest,
			xplain.StageError,
			explainSummaryJSPreRequestOutputBad,
			jsBefore,
			e.req,
			err.Error(),
		)

		msg := e.errorResponse(err, "JS pre-request failed")
		return &msg
	}
	if e.hasJSPre {
		e.explain.stage(
			explainStageJSPreRequest,
			xplain.StageOK,
			explainSummaryJSPreRequestComplete,
			jsBefore,
			e.req,
		)
	}
	if err := e.sendCtx.Err(); err != nil {
		return e.canceledResponse(err)
	}

	e.applyRuntimeGlobals(preResult.Globals)
	e.scriptVars = mergeVariableMaps(rtsResult.Variables, preResult.Variables)
	return nil
}

func (e *execContext) buildResolver() {
	resolverExtras := make([]map[string]string, 0, len(e.extras)+1)
	if len(e.scriptVars) > 0 {
		resolverExtras = append(resolverExtras, e.scriptVars)
	}
	for _, extra := range e.extras {
		if len(extra) > 0 {
			resolverExtras = append(resolverExtras, extra)
		}
	}

	e.resolver = e.model.buildResolver(
		e.sendCtx,
		e.doc,
		e.req,
		e.envName,
		e.options.BaseDir,
		e.extraVals,
		resolverExtras...,
	)

	e.trace = vars.NewTrace()
	e.resolver.SetTrace(e.trace)
	e.explain.trace = e.trace
}

func (e *execContext) resolveRoute() *responseMsg {
	var err error

	e.sshPlan, err = e.model.resolveSSH(e.doc, e.req, e.resolver, e.envName)
	if err != nil {
		e.explain.stage(
			explainStageRoute,
			xplain.StageError,
			explainSummaryRouteSSHResolutionFailed,
			nil,
			nil,
			err.Error(),
		)

		msg := e.errorResponse(err, "Route resolution failed")
		msg.err = errdef.Wrap(errdef.CodeHTTP, err, "resolve ssh")
		return &msg
	}

	e.k8sPlan, err = e.model.resolveK8s(e.doc, e.req, e.resolver, e.envName)
	if err != nil {
		e.explain.stage(
			explainStageRoute,
			xplain.StageError,
			explainSummaryRouteK8sResolutionFailed,
			nil,
			nil,
			err.Error(),
		)

		msg := e.errorResponse(err, "Route resolution failed")
		msg.err = errdef.Wrap(errdef.CodeHTTP, err, "resolve k8s")
		return &msg
	}

	e.options.SSH = e.sshPlan
	e.options.K8s = e.k8sPlan
	e.explain.setRoute(e.sshPlan, e.k8sPlan)

	if route := explainRoute(e.sshPlan, e.k8sPlan); route != nil {
		notes := append([]string{route.Summary}, route.Notes...)
		e.explain.stage(explainStageRoute, xplain.StageOK, route.Kind, nil, nil, notes...)
		if e.sshPlan != nil && e.sshPlan.Active() && e.sshPlan.Config != nil &&
			!e.sshPlan.Config.Strict {
			e.explain.warn("@ssh strict_hostkey=false (insecure)")
		}
	}

	return nil
}

func (e *execContext) configureGRPCOptions() {
	e.useGRPC = e.req.GRPC != nil
	if !e.useGRPC {
		return
	}

	e.grpcOpts = e.model.grpcOptions
	if e.grpcOpts.BaseDir != "" {
		return
	}
	e.grpcOpts.BaseDir = e.options.BaseDir
	if e.grpcOpts.BaseDir == "" && e.model.currentFile != "" {
		e.grpcOpts.BaseDir = filepath.Dir(e.model.currentFile)
	}
}

func (e *execContext) applySettings() *responseMsg {
	e.configureGRPCOptions()

	globalSettings := settings.FromEnv(e.model.cfg.EnvironmentSet, e.envName)
	fileSettings := map[string]string{}
	if e.doc != nil && e.doc.Settings != nil {
		fileSettings = e.doc.Settings
	}
	requestSettings := e.req.Settings

	settingsBefore := cloneRequest(e.req)
	e.mergedSettings = settings.Merge(globalSettings, fileSettings, requestSettings)
	e.req.Settings = e.mergedSettings
	e.explain.setSettings(e.mergedSettings)
	e.explain.stage(
		explainStageSettings,
		xplain.StageOK,
		explainSummarySettingsMerged,
		settingsBefore,
		e.req,
	)

	handlers := []settings.Handler{
		settings.HTTPHandler(&e.options, e.resolver),
	}
	if e.useGRPC {
		handlers = append(handlers, settings.GRPCHandler(&e.grpcOpts, e.resolver))
	}

	if _, err := settings.New(handlers...).ApplyAll(e.mergedSettings); err != nil {
		e.explain.stage(
			explainStageSettings,
			xplain.StageError,
			explainSummarySettingsApplyFailed,
			nil,
			nil,
			err.Error(),
		)

		msg := e.errorResponse(err, "Settings application failed")
		return &msg
	}

	e.effectiveTimeout = defaultTimeout(resolveRequestTimeout(e.req, e.options.Timeout))
	return nil
}

func (e *execContext) prepareAuthentication() *responseMsg {
	if e.req.Metadata.Auth == nil {
		return nil
	}

	e.explain.addSecrets(explainAuthSecretValues(e.req.Metadata.Auth, e.resolver)...)

	authBefore := cloneRequest(e.req)

	var extraSecrets []string
	switch strings.ToLower(strings.TrimSpace(e.req.Metadata.Auth.Type)) {
	case "command":
		res, err := e.model.ensureCommandAuth(
			e.sendCtx,
			e.req,
			e.resolver,
			e.envName,
			e.effectiveTimeout,
		)
		if err != nil {
			e.explain.stage(
				explainStageAuth,
				xplain.StageError,
				explainSummaryAuthInjectionFailed,
				authBefore,
				e.req,
				err.Error(),
			)

			msg := e.errorResponse(err, "Auth preparation failed")
			return &msg
		}
		extraSecrets = commandAuthSecrets(res)
	default:
		if err := e.model.ensureOAuth(
			e.sendCtx,
			e.req,
			e.resolver,
			e.options,
			e.envName,
			e.effectiveTimeout,
		); err != nil {
			e.explain.stage(
				explainStageAuth,
				xplain.StageError,
				explainSummaryAuthInjectionFailed,
				authBefore,
				e.req,
				err.Error(),
			)

			msg := e.errorResponse(err, "Auth preparation failed")
			return &msg
		}
		extraSecrets = explainInjectedAuthSecrets(e.req.Metadata.Auth, authBefore, e.req)
	}

	e.explain.addSecrets(extraSecrets...)
	e.runtimeSecrets = append(e.runtimeSecrets, extraSecrets...)
	e.explain.stage(explainStageAuth, xplain.StageOK, explainSummaryAuthPrepared, authBefore, e.req)
	return nil
}

func (e *execContext) prepareProtocolRequests() *responseMsg {
	if e.req.GRPC != nil {
		grpcBefore := cloneRequest(e.req)
		if err := e.model.prepareGRPCRequest(e.req, e.resolver, e.grpcOpts.BaseDir); err != nil {
			e.explain.stage(
				explainStageGRPCPrepare,
				xplain.StageError,
				explainSummaryGRPCPrepareFailed,
				grpcBefore,
				e.req,
				err.Error(),
			)

			msg := e.errorResponse(err, "gRPC preparation failed")
			return &msg
		}
		e.explain.stage(
			explainStageGRPCPrepare,
			xplain.StageOK,
			explainSummaryGRPCRequestPrepared,
			grpcBefore,
			e.req,
		)
	}

	if e.req.WebSocket != nil {
		wsBefore := cloneRequest(e.req)
		if err := e.model.expandWebSocketSteps(e.req, e.resolver); err != nil {
			e.explain.stage(
				explainStageWebSocketPrepare,
				xplain.StageError,
				explainSummaryWebSocketPrepareFailed,
				wsBefore,
				e.req,
				err.Error(),
			)

			msg := e.errorResponse(err, "WebSocket preparation failed")
			return &msg
		}
		e.explain.stage(
			explainStageWebSocketPrepare,
			xplain.StageOK,
			explainSummaryWebSocketRequestPrepared,
			wsBefore,
			e.req,
		)
	}

	return nil
}

func (e *execContext) prepareRequest() *responseMsg {
	e.buildResolver()
	if msg := e.resolveRoute(); msg != nil {
		return msg
	}
	if msg := e.applySettings(); msg != nil {
		return msg
	}
	if msg := e.prepareAuthentication(); msg != nil {
		return msg
	}
	if msg := e.prepareProtocolRequests(); msg != nil {
		return msg
	}
	return nil
}

func (e *execContext) executeInteractiveWebSocket() responseMsg {
	ctx, cancel := context.WithCancel(e.sendCtx)
	cancelActive := true
	defer func() {
		if cancelActive {
			cancel()
		}
	}()

	handle, fallback, startErr := e.client.StartWebSocket(ctx, e.req, e.resolver, e.options)
	if startErr != nil {
		msg := e.errorResponse(startErr, "WebSocket request failed")
		msg.requestText = e.requestText()
		return msg
	}
	if fallback != nil {
		return e.successHTTPResponse(fallback, "HTTP request sent")
	}

	e.model.attachWebSocketHandle(handle, e.req)
	if handle != nil && handle.Session != nil {
		sessionDone := handle.Session.Done()
		releaseSend := e.sendCancel
		go func() {
			<-sessionDone
			cancel()
			if releaseSend != nil {
				releaseSend()
			}
		}()
		// Interactive websocket sessions must outlive the initial request
		// command so the Stream tab and console stay attached.
		e.sendCancel = nil
		cancelActive = false
	}

	return e.successHTTPResponse(streamingPlaceholderResponse(handle.Meta), "HTTP request sent")
}

func (e *execContext) successHTTPResponse(
	resp *httpclient.Response,
	decision string,
) responseMsg {
	if resp != nil {
		e.explain.sentHTTP(e.req, resp)
		e.explain.setHTTP(resp)
	}
	e.explain.setPrepared(e.req)

	msg := e.baseResponse()
	msg.response = resp
	msg.requestText = e.requestText()
	msg.explain = e.explain.finish(xplain.StatusReady, strings.TrimSpace(decision), nil)
	return msg
}

type interactiveExecFlow struct {
	ctx *execContext
}

func (f interactiveExecFlow) PendingCancel() *xexec.RequestResult {
	if f.ctx == nil {
		return nil
	}
	return execResultFromResponseMsgPtr(f.ctx.pendingCancel())
}

func (f interactiveExecFlow) Finish() {
	if f.ctx != nil {
		f.ctx.finish()
	}
}

func (f interactiveExecFlow) EvaluateCondition() *xexec.RequestResult {
	if f.ctx == nil {
		return nil
	}
	return execResultFromResponseMsgPtr(f.ctx.evaluateCondition())
}

func (f interactiveExecFlow) RunPreRequest() *xexec.RequestResult {
	if f.ctx == nil {
		return nil
	}
	return execResultFromResponseMsgPtr(f.ctx.runPreRequestScripts())
}

func (f interactiveExecFlow) PrepareRequest() *xexec.RequestResult {
	if f.ctx == nil {
		return nil
	}
	return execResultFromResponseMsgPtr(f.ctx.prepareRequest())
}

func (interactiveExecFlow) PreviewResult() xexec.RequestResult {
	return xexec.RequestResult{}
}

func (f interactiveExecFlow) UseGRPC() bool {
	return f.ctx != nil && f.ctx.req != nil && f.ctx.req.GRPC != nil
}

func (f interactiveExecFlow) IsInteractiveWebSocket() bool {
	return f.ctx != nil &&
		f.ctx.req != nil &&
		f.ctx.req.WebSocket != nil &&
		len(f.ctx.req.WebSocket.Steps) == 0
}

func (f interactiveExecFlow) ExecuteInteractiveWebSocket() xexec.RequestResult {
	if f.ctx == nil {
		return xexec.RequestResult{}
	}
	return execResultFromResponseMsg(f.ctx.executeInteractiveWebSocket())
}

func (interactiveExecFlow) ExecuteGRPC() xexec.RequestResult {
	return xexec.RequestResult{}
}

func (interactiveExecFlow) ExecuteHTTP() xexec.RequestResult {
	return xexec.RequestResult{}
}

func execResultFromResponseMsgPtr(msg *responseMsg) *xexec.RequestResult {
	if msg == nil {
		return nil
	}
	out := execResultFromResponseMsg(*msg)
	return &out
}

func execResultFromResponseMsg(msg responseMsg) xexec.RequestResult {
	return xexec.RequestResult{
		Response:       msg.response,
		GRPC:           msg.grpc,
		Stream:         msg.stream,
		Transcript:     append([]byte(nil), msg.transcript...),
		Err:            msg.err,
		Tests:          append([]scripts.TestResult(nil), msg.tests...),
		ScriptErr:      msg.scriptErr,
		Executed:       msg.executed,
		RequestText:    msg.requestText,
		RuntimeSecrets: append([]string(nil), msg.runtimeSecrets...),
		Environment:    msg.environment,
		Skipped:        msg.skipped,
		SkipReason:     msg.skipReason,
		Preview:        msg.preview,
		Explain:        msg.explain,
	}
}

func responseMsgFromExecResult(res xexec.RequestResult) responseMsg {
	return responseMsg{
		response:       res.Response,
		grpc:           res.GRPC,
		stream:         res.Stream,
		transcript:     append([]byte(nil), res.Transcript...),
		err:            res.Err,
		tests:          append([]scripts.TestResult(nil), res.Tests...),
		scriptErr:      res.ScriptErr,
		executed:       res.Executed,
		requestText:    res.RequestText,
		runtimeSecrets: append([]string(nil), res.RuntimeSecrets...),
		environment:    res.Environment,
		skipped:        res.Skipped,
		skipReason:     res.SkipReason,
		preview:        res.Preview,
		explain:        res.Explain,
	}
}
