package ui

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/settings"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/tracebudget"
	"github.com/unkn0wn-root/resterm/internal/tunnel"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type execMode uint8

const (
	execModeSend execMode = iota
	execModeExplain
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
	preview        bool
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
	preview bool,
	extraVals map[string]rts.Value,
	extras []map[string]string,
) *execContext {
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
	explain := newExplainBuilder(m, req, envName, preview)
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
		preview:      preview,
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
	return m.executeWithMode(
		doc,
		req,
		options,
		envOverride,
		extraVals,
		execModeSend,
		extras...,
	)
}

func (m *Model) executeExplain(
	doc *restfile.Document,
	req *restfile.Request,
	options httpclient.Options,
	envOverride string,
	extraVals map[string]rts.Value,
	extras ...map[string]string,
) tea.Cmd {
	return m.executeWithMode(
		doc,
		req,
		options,
		envOverride,
		extraVals,
		execModeExplain,
		extras...,
	)
}

func (m *Model) executeWithMode(
	doc *restfile.Document,
	req *restfile.Request,
	options httpclient.Options,
	envOverride string,
	extraVals map[string]rts.Value,
	mode execMode,
	extras ...map[string]string,
) tea.Cmd {
	options = m.resolveHTTPOptions(options)
	envName := vars.SelectEnv(m.cfg.EnvironmentSet, envOverride, m.cfg.EnvironmentName)
	preview := mode == execModeExplain
	if req == nil {
		err := errdef.New(errdef.CodeUI, "request is nil")
		return func() tea.Msg {
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
		return func() tea.Msg {
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
	exec := newExecContext(m, doc, req, options, envName, preview, extraVals, extras)
	return exec.cmd()
}

func (e *execContext) cmd() tea.Cmd {
	return func() tea.Msg {
		return e.run()
	}
}

func (e *execContext) run() tea.Msg {
	if msg := e.pendingCancel(); msg != nil {
		return *msg
	}

	defer e.sendCancel()

	if msg := e.evaluateCondition(); msg != nil {
		return *msg
	}
	if msg := e.runPreRequestScripts(); msg != nil {
		return *msg
	}
	if msg := e.prepareRequest(); msg != nil {
		return *msg
	}
	if e.preview {
		return e.previewResponse()
	}
	if e.useGRPC {
		return e.executeGRPC()
	}
	return e.executeHTTP()
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

func (e *execContext) captureVariables() map[string]string {
	capVars := mergeVariableMaps(e.model.collectVariables(e.doc, e.req, e.envName), e.scriptVars)
	applyExtraVariables(capVars, e.extras)
	return capVars
}

func (e *execContext) applyRuntimeGlobals(changes map[string]scripts.GlobalValue) {
	if len(changes) == 0 {
		return
	}
	if e.preview {
		e.storeGlobals = mergeGlobalValues(e.storeGlobals, changes)
	} else {
		e.model.applyGlobalMutations(changes, e.envName)
		e.storeGlobals = e.model.collectStoredGlobalValues(e.envName)
	}
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

	if e.preview {
		e.resolver = e.model.buildResolverWithGlobals(
			e.sendCtx,
			e.doc,
			e.req,
			e.envName,
			e.options.BaseDir,
			e.extraVals,
			e.storeGlobals,
			resolverExtras...,
		)
	} else {
		e.resolver = e.model.buildResolver(
			e.sendCtx,
			e.doc,
			e.req,
			e.envName,
			e.options.BaseDir,
			e.extraVals,
			resolverExtras...,
		)
	}

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
	if e.preview {
		authPreview, err := e.model.prepareExplainAuthPreview(e.req, e.resolver, e.envName)
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
		e.explain.addSecrets(authPreview.extraSecrets...)
		e.explain.stage(
			explainStageAuth,
			authPreview.status,
			authPreview.summary,
			authBefore,
			e.req,
			authPreview.notes...,
		)
		return nil
	}

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

func (e *execContext) previewResponse() tea.Msg {
	e.explain.setPrepared(e.req)
	if e.req.GRPC == nil {
		if err := e.model.prepareExplainHTTPPreview(
			e.sendCtx,
			e.explain.report,
			e.req,
			e.resolver,
			e.options,
		); err != nil {
			e.explain.stage(
				explainStageHTTPPrepare,
				xplain.StageError,
				explainSummaryHTTPRequestBuildFailed,
				nil,
				nil,
				err.Error(),
			)

			msg := e.errorResponse(err, "HTTP preparation failed")
			return msg
		}
	}

	msg := e.baseResponse()
	msg.requestText = e.requestText()
	msg.preview = true
	msg.explain = e.explain.finish(
		xplain.StatusReady,
		"Explain preview ready. No request was sent.",
		nil,
	)
	return msg
}

func (e *execContext) executeGRPC() tea.Msg {
	grpcClient := e.model.grpcClient
	if grpcClient == nil {
		err := errdef.New(errdef.CodeHTTP, "gRPC client is not initialised")
		e.explain.setPrepared(e.req)

		msg := e.errorResponse(err, "gRPC request failed")
		msg.requestText = e.requestText()
		return msg
	}

	ctx, cancel := context.WithTimeout(e.sendCtx, e.effectiveTimeout)
	defer cancel()

	if e.grpcOpts.DialTimeout == 0 {
		e.grpcOpts.DialTimeout = e.effectiveTimeout
	}
	e.grpcOpts.SSH = e.sshPlan
	e.grpcOpts.K8s = e.k8sPlan

	hook := func(session *stream.Session) {
		e.model.attachGRPCSession(session, e.req)
	}
	grpcResp, grpcErr := grpcClient.Execute(ctx, e.req, e.req.GRPC, e.grpcOpts, hook)
	if grpcErr != nil {
		e.explain.setPrepared(e.req)

		msg := e.errorResponse(grpcErr, "gRPC request failed")
		msg.grpc = grpcResp
		msg.requestText = e.requestText()
		return msg
	}

	respForScripts := grpcScriptResponse(e.req, grpcResp)
	var captures captureResult
	if err := e.model.applyCaptures(captureRun{
		doc:  e.doc,
		req:  e.req,
		res:  e.resolver,
		resp: respForScripts,
		out:  &captures,
		env:  e.envName,
		v:    e.captureVariables(),
		x:    e.extraVals,
	}); err != nil {
		e.explain.stage(
			explainStageCaptures,
			xplain.StageError,
			explainSummaryCaptureEvaluationFailed,
			nil,
			nil,
			err.Error(),
		)
		e.explain.setPrepared(e.req)

		msg := e.errorResponse(err, "Capture evaluation failed")
		return msg
	}

	updatedVars := e.model.collectVariables(e.doc, e.req, e.envName)
	testVars := mergeVariableMaps(updatedVars, e.scriptVars)
	testGlobals := e.model.collectGlobalValues(e.doc, e.envName)
	asserts, assertErr := e.model.runAsserts(
		ctx,
		e.doc,
		e.req,
		e.envName,
		e.options.BaseDir,
		testVars,
		e.extraVals,
		rtsGRPC(grpcResp),
		nil,
		nil,
	)
	tests, globalChanges, testErr := e.runner.RunTests(
		e.req.Metadata.Scripts,
		scripts.TestInput{
			Response:  respForScripts,
			Variables: testVars,
			Globals:   testGlobals,
			BaseDir:   e.options.BaseDir,
		},
	)
	e.applyRuntimeGlobals(globalChanges)
	e.explain.setPrepared(e.req)
	e.explain.setGRPC(e.req)

	msg := e.baseResponse()
	msg.grpc = grpcResp
	msg.tests = append(asserts, tests...)
	msg.scriptErr = mergeErr(assertErr, testErr)
	msg.requestText = e.requestText()
	msg.explain = e.explain.finish(xplain.StatusReady, "gRPC request sent", nil)
	return msg
}

func (e *execContext) executeHTTP() tea.Msg {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		cancelActive = true
	)

	if e.req.WebSocket != nil && len(e.req.WebSocket.Steps) == 0 {
		ctx, cancel = context.WithCancel(e.sendCtx)
	} else {
		ctx, cancel = context.WithTimeout(e.sendCtx, e.effectiveTimeout)
	}
	defer func() {
		if cancelActive {
			cancel()
		}
	}()

	var (
		response *httpclient.Response
		err      error
	)

	switch {
	case e.req.WebSocket != nil:
		handle, fallback, startErr := e.client.StartWebSocket(ctx, e.req, e.resolver, e.options)
		if startErr != nil {
			msg := e.errorResponse(startErr, "WebSocket request failed")
			return msg
		}
		if fallback != nil {
			response = fallback
		} else {
			e.model.attachWebSocketHandle(handle, e.req)
			if len(e.req.WebSocket.Steps) == 0 {
				if handle != nil && handle.Session != nil {
					sessionDone := handle.Session.Done()
					go func() {
						<-sessionDone
						cancel()
					}()
					cancelActive = false
				}
				response = streamingPlaceholderResponse(handle.Meta)
			} else {
				response, err = e.client.CompleteWebSocket(ctx, handle, e.req, e.options)
			}
		}
	case e.req.SSE != nil:
		handle, fallback, startErr := e.client.StartSSE(ctx, e.req, e.resolver, e.options)
		if startErr != nil {
			msg := e.errorResponse(startErr, "SSE request failed")
			return msg
		}
		if fallback != nil {
			response = fallback
		} else {
			e.model.attachSSEHandle(handle, e.req)
			response, err = httpclient.CompleteSSE(handle)
		}
	default:
		response, err = e.client.Execute(ctx, e.req, e.resolver, e.options)
	}

	if response != nil {
		e.explain.sentHTTP(e.req, response)
	}
	if err != nil {
		e.explain.setPrepared(e.req)
		if response != nil {
			e.explain.setHTTP(response)
		}

		msg := e.errorResponse(err, "HTTP request failed")
		msg.response = response
		return msg
	}

	streamInfo, streamErr := streamInfoFromResponse(e.req, response)
	if streamErr != nil {
		e.explain.setPrepared(e.req)
		e.explain.setHTTP(response)

		msg := e.errorResponse(streamErr, "Stream decoding failed")
		msg.err = errdef.Wrap(errdef.CodeHTTP, streamErr, "decode stream transcript")
		return msg
	}

	respForScripts := httpScriptResponse(response)
	var captures captureResult
	if err := e.model.applyCaptures(captureRun{
		doc:    e.doc,
		req:    e.req,
		res:    e.resolver,
		resp:   respForScripts,
		stream: streamInfo,
		out:    &captures,
		env:    e.envName,
		v:      e.captureVariables(),
		x:      e.extraVals,
	}); err != nil {
		e.explain.stage(
			explainStageCaptures,
			xplain.StageError,
			explainSummaryCaptureEvaluationFailed,
			nil,
			nil,
			err.Error(),
		)
		e.explain.setPrepared(e.req)
		e.explain.setHTTP(response)

		msg := e.errorResponse(err, "Capture evaluation failed")
		return msg
	}

	updatedVars := e.model.collectVariables(e.doc, e.req, e.envName)
	testVars := mergeVariableMaps(updatedVars, e.scriptVars)
	testGlobals := e.model.collectGlobalValues(e.doc, e.envName)
	asserts, assertErr := e.model.runAsserts(
		ctx,
		e.doc,
		e.req,
		e.envName,
		e.options.BaseDir,
		testVars,
		e.extraVals,
		rtsHTTP(response),
		rtsTrace(response),
		rtsStream(streamInfo),
	)
	traceInput := scripts.NewTraceInput(response.Timeline, e.req.Metadata.Trace)
	tests, globalChanges, testErr := e.runner.RunTests(e.req.Metadata.Scripts, scripts.TestInput{
		Response:  respForScripts,
		Variables: testVars,
		Globals:   testGlobals,
		BaseDir:   e.options.BaseDir,
		Stream:    streamInfo,
		Trace:     traceInput,
	})
	e.applyRuntimeGlobals(globalChanges)
	e.explain.setPrepared(e.req)
	e.explain.setHTTP(response)

	msg := e.baseResponse()
	msg.response = response
	msg.tests = append(asserts, tests...)
	msg.scriptErr = mergeErr(assertErr, testErr)
	msg.requestText = e.requestText()
	msg.explain = e.explain.finish(xplain.StatusReady, "HTTP request sent", nil)
	return msg
}
