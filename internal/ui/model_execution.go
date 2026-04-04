package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/curl"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/settings"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/tracebudget"
	"github.com/unkn0wn-root/resterm/internal/tunnel"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) cancelInFlightSend(status string) {
	if m.sendCancel != nil {
		m.sendCancel()
	}
	if strings.TrimSpace(status) != "" {
		m.setStatusMessage(statusMsg{text: status, level: statusInfo})
	}
}

func isCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

func batchCmds(cmds []tea.Cmd) tea.Cmd {
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}

func (m *Model) cancelStatus() string {
	if state := m.profileRun; state != nil {
		return "Canceling profile run..."
	}
	if state := m.workflowRun; state != nil {
		name := strings.TrimSpace(state.workflow.Name)
		if name == "" {
			name = "workflow"
		}
		return fmt.Sprintf("Canceling %s...", name)
	}
	if m.compareRun != nil {
		return "Canceling compare run..."
	}
	if m.sending {
		return "Canceling in-progress request..."
	}
	if m.responseLoading {
		return "Canceling response formatting..."
	}
	if m.hasReflowPending() {
		return "Canceling response reflow..."
	}
	return "Canceling..."
}

func (m *Model) hasActiveRun() bool {
	return m.sending || m.profileRun != nil || m.workflowRun != nil || m.compareRun != nil
}

func (m Model) hasReflowPending() bool {
	for i := range m.responsePanes {
		pane := &m.responsePanes[i]
		if pane.reflow == nil {
			continue
		}
		for _, state := range pane.reflow {
			if reflowStateLive(pane, state) {
				return true
			}
		}
	}
	return false
}

func (m Model) spinnerActive() bool {
	return m.sending || m.responseLoading || m.hasReflowPending()
}

func (m *Model) cancelActiveRuns() tea.Cmd {
	if !m.hasActiveRun() && !m.responseLoading && !m.hasReflowPending() {
		return nil
	}
	return m.cancelRuns(m.cancelStatus())
}

func (m *Model) cancelRuns(status string) tea.Cmd {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "Canceling..."
	}

	m.stopSending()
	m.stopStatusPulse()

	var cmds []tea.Cmd
	if cmd := m.cancelProfileRun(status); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.cancelWorkflowRun(status); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.cancelCompareRun(status); cmd != nil {
		cmds = append(cmds, cmd)
	}
	m.cancelInFlightSend(status)
	if m.responseLoading {
		if cmd := m.cancelResponseFormatting(""); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if cmd := m.cancelResponseReflow(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return batchCmds(cmds)
}

func (m *Model) cancelResponseReflow() tea.Cmd {
	canceled := false
	for i := range m.responsePanes {
		pane := &m.responsePanes[i]
		if len(pane.reflow) == 0 {
			continue
		}
		wasActive := m.reflowActiveForPane(pane)
		for key, state := range pane.reflow {
			markReflowCanceled(pane, key, state.snapshotID)
		}
		clearReflowAll(pane)
		canceled = true

		if wasActive {
			m.showReflowCanceled(pane)
		}
	}
	if !canceled {
		return nil
	}
	m.respSpinStop()
	if !m.hasActiveRun() && !m.responseLoading && !m.hasReflowPending() {
		m.setStatusMessage(statusMsg{})
	}
	return nil
}

func (m *Model) showReflowCanceled(pane *responsePaneState) {
	if pane == nil {
		return
	}
	tab := pane.activeTab
	if tab == responseTabHistory {
		return
	}

	_, ww, h := paneDims(pane, tab)
	sr, sid := paneSnap(pane)
	m.applyReflowCanceled(pane, tab, ww, h, sr, sid)
}

func (m *Model) cancelProfileRun(reason string) tea.Cmd {
	state := m.profileRun
	if state == nil {
		return nil
	}
	state.canceled = true
	if strings.TrimSpace(state.cancelReason) == "" {
		state.cancelReason = reason
	}
	if state.current == nil {
		return m.finalizeProfileRun(responseMsg{}, state)
	}
	return nil
}

func (m *Model) cancelWorkflowRun(reason string) tea.Cmd {
	state := m.workflowRun
	if state == nil {
		return nil
	}
	state.canceled = true
	if strings.TrimSpace(state.cancelReason) == "" {
		state.cancelReason = reason
	}
	if state.current == nil {
		return m.finalizeWorkflowRun(state)
	}
	return nil
}

func (m *Model) cancelCompareRun(reason string) tea.Cmd {
	state := m.compareRun
	if state == nil {
		return nil
	}
	state.canceled = true
	if strings.TrimSpace(state.cancelReason) == "" {
		state.cancelReason = reason
	}
	if state.current == nil {
		return m.finalizeCompareRun(state)
	}
	return nil
}

type activeReqExec struct {
	doc  *restfile.Document
	req  *restfile.Request
	opts httpclient.Options
	wrap func(tea.Cmd) tea.Cmd
}

func (m *Model) prepareActiveRequestExec() (*activeReqExec, tea.Cmd) {
	content := m.editor.Value()
	doc := parser.Parse(m.currentFile, []byte(content))
	cursorLine := currentCursorLine(m.editor)
	req, _ := m.requestAtCursor(doc, content, cursorLine)
	if req == nil {
		return nil, func() tea.Msg {
			return statusMsg{text: "No request at cursor", level: statusWarn}
		}
	}

	rc := m.restorePane(paneRegionResponse)
	wrap := func(cmd tea.Cmd) tea.Cmd {
		return batchCommands(rc, cmd)
	}

	m.doc = doc
	m.syncRequestList(doc)
	m.setActiveRequest(req)
	m.syncAllGlobals(doc)

	cloned := cloneRequest(req)
	m.currentRequest = cloned
	m.testResults = nil
	m.scriptError = nil

	opts := m.cfg.HTTPOptions
	if opts.BaseDir == "" && m.currentFile != "" {
		opts.BaseDir = filepath.Dir(m.currentFile)
	}

	return &activeReqExec{doc: doc, req: cloned, opts: opts, wrap: wrap}, nil
}

func (m *Model) sendActiveRequest() tea.Cmd {
	if cmd := m.cancelActiveRuns(); cmd != nil {
		return cmd
	}
	st, cmd := m.prepareActiveRequestExec()
	if cmd != nil {
		return cmd
	}

	if st.req.Metadata.ForEach != nil {
		if spec := m.compareSpecForRequest(st.req); spec != nil {
			m.setStatusMessage(
				statusMsg{level: statusWarn, text: "@compare cannot run alongside @for-each"},
			)
			return st.wrap(nil)
		}
		if st.req.Metadata.Profile != nil {
			m.setStatusMessage(
				statusMsg{level: statusWarn, text: "@profile cannot run alongside @for-each"},
			)
			return st.wrap(nil)
		}
		if st.req.Metadata.Trace != nil && st.req.Metadata.Trace.Enabled {
			st.opts.Trace = true
			if budget, ok := tracebudget.FromSpec(st.req.Metadata.Trace); ok {
				st.opts.TraceBudget = &budget
			}
		}
		return st.wrap(m.startForEachRun(st.doc, st.req, st.opts))
	}

	if spec := m.compareSpecForRequest(st.req); spec != nil {
		if st.req.Metadata.Profile != nil {
			m.setStatusMessage(
				statusMsg{level: statusWarn, text: "@compare cannot run alongside @profile"},
			)
			return st.wrap(nil)
		}
		return st.wrap(m.startCompareRun(st.doc, st.req, spec, st.opts))
	}

	if st.req.Metadata.Trace != nil && st.req.Metadata.Trace.Enabled {
		st.opts.Trace = true
		if budget, ok := tracebudget.FromSpec(st.req.Metadata.Trace); ok {
			st.opts.TraceBudget = &budget
		}
	}

	if st.req.Metadata.Profile != nil {
		return st.wrap(m.startProfileRun(st.doc, st.req, st.opts))
	}

	spin := m.startSending()
	target := m.statusRequestTarget(st.doc, st.req, "")
	base := "Sending"
	if trimmed := strings.TrimSpace(target); trimmed != "" {
		base = fmt.Sprintf("Sending %s", trimmed)
	}
	m.statusPulseBase = base
	m.statusPulseFrame = -1
	m.setStatusMessage(statusMsg{text: base, level: statusInfo})

	execCmd := m.executeRequest(st.doc, st.req, st.opts, "", nil)
	pulse := m.startStatusPulse()
	return st.wrap(batchCmds([]tea.Cmd{execCmd, pulse, spin}))
}

func (m *Model) explainActiveRequest() tea.Cmd {
	if cmd := m.cancelActiveRuns(); cmd != nil {
		return cmd
	}

	st, cmd := m.prepareActiveRequestExec()
	if cmd != nil {
		return cmd
	}

	spin := m.startSending()
	target := m.statusRequestTarget(st.doc, st.req, "")
	base := "Preparing explain preview"
	if trimmed := strings.TrimSpace(target); trimmed != "" {
		base = fmt.Sprintf("Preparing explain for %s", trimmed)
	}
	m.statusPulseBase = base
	m.statusPulseFrame = -1
	m.setStatusMessage(statusMsg{text: base, level: statusInfo})

	execCmd := m.executeExplain(st.doc, st.req, st.opts, "", nil)
	pulse := m.startStatusPulse()
	return st.wrap(batchCmds([]tea.Cmd{execCmd, pulse, spin}))
}

// Allow CLI-level compare flags to kick off a sweep even when the request lacks
// @compare metadata so users can reuse the same editor workflow while honoring
// --compare selections.
func (m *Model) startConfigCompareFromEditor() tea.Cmd {
	content := m.editor.Value()
	doc := parser.Parse(m.currentFile, []byte(content))
	cursorLine := currentCursorLine(m.editor)
	req, _ := m.requestAtCursor(doc, content, cursorLine)
	if req == nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "No request at cursor"})
		return nil
	}

	if req.Metadata.ForEach != nil {
		m.setStatusMessage(
			statusMsg{level: statusWarn, text: "@compare cannot run alongside @for-each"},
		)
		return nil
	}
	if req.Metadata.Profile != nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "@profile cannot run during compare"})
		return nil
	}

	spec := buildConfigCompareSpec(m.cfg.CompareTargets, m.cfg.CompareBase)
	if spec == nil && req.Metadata.Compare != nil {
		spec = cloneCompareSpec(req.Metadata.Compare)
	}
	if spec == nil {
		m.setStatusMessage(statusMsg{
			level: statusWarn,
			text:  "No compare targets configured. Use --compare or add @compare.",
		})
		return nil
	}

	m.doc = doc
	m.syncRequestList(doc)
	m.setActiveRequest(req)
	m.syncAllGlobals(doc)

	cloned := cloneRequest(req)
	m.currentRequest = cloned
	m.testResults = nil
	m.scriptError = nil

	options := m.cfg.HTTPOptions
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}
	if cloned.Metadata.Trace != nil && cloned.Metadata.Trace.Enabled {
		options.Trace = true
		if budget, ok := tracebudget.FromSpec(cloned.Metadata.Trace); ok {
			options.TraceBudget = &budget
		}
	}

	return m.startCompareRun(doc, cloned, spec, options)
}

type execMode uint8

const (
	execModeSend execMode = iota
	execModeExplain
)

type explainFinalizeInput struct {
	report         *xplain.Report
	request        *restfile.Request
	envName        string
	preview        bool
	status         xplain.Status
	decision       string
	err            error
	trace          *vars.Trace
	mergedSettings map[string]string
	sshPlan        *ssh.Plan
	k8sPlan        *k8s.Plan
	globals        map[string]scripts.GlobalValue
	extraSecrets   []string
}

type explainAuthPreviewResult struct {
	status       xplain.StageStatus
	summary      string
	notes        []string
	extraSecrets []string
}

type explainBuilder struct {
	model          *Model
	request        *restfile.Request
	envName        string
	preview        bool
	report         *xplain.Report
	trace          *vars.Trace
	mergedSettings map[string]string
	sshPlan        *ssh.Plan
	k8sPlan        *k8s.Plan
	globals        map[string]scripts.GlobalValue
	extraSecrets   []string
}

type execContext struct {
	// Immutable execution inputs.
	model     *Model
	doc       *restfile.Document
	req       *restfile.Request
	envName   string
	options   httpclient.Options
	extraVals map[string]rts.Value
	extras    []map[string]string
	preview   bool

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

func newExplainBuilder(
	m *Model,
	req *restfile.Request,
	envName string,
	preview bool,
) *explainBuilder {
	b := &explainBuilder{
		model:   m,
		request: req,
		envName: envName,
		preview: preview,
		report:  newExplainReport(req, envName),
	}
	if !preview || req == nil {
		return b
	}
	if req.Metadata.ForEach != nil {
		b.warn("@for-each iterations are not expanded in explain preview")
	}
	if req.Metadata.Compare != nil {
		b.warn("@compare sweep is not executed in explain preview")
	}
	if req.Metadata.Profile != nil {
		b.warn("@profile run is not executed in explain preview")
	}
	return b
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

func (b *explainBuilder) warn(msg string) {
	addExplainWarn(b.report, msg)
}

func (b *explainBuilder) stage(
	name string,
	st xplain.StageStatus,
	sum string,
	before *restfile.Request,
	after *restfile.Request,
	notes ...string,
) {
	addExplainStage(b.report, name, st, sum, before, after, notes...)
}

func (b *explainBuilder) sentHTTP(
	req *restfile.Request,
	resp *httpclient.Response,
	notes ...string,
) {
	addExplainSentHTTPStage(b.report, req, resp, notes...)
}

func (b *explainBuilder) setSettings(mergedSettings map[string]string) {
	b.mergedSettings = mergedSettings
}

func (b *explainBuilder) setRoute(sshPlan *ssh.Plan, k8sPlan *k8s.Plan) {
	b.sshPlan = sshPlan
	b.k8sPlan = k8sPlan
}

func (b *explainBuilder) addSecrets(values ...string) {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		b.extraSecrets = append(b.extraSecrets, value)
	}
}

func (b *explainBuilder) setPrepared(req *restfile.Request) {
	setExplainPrepared(b.report, req, b.mergedSettings, b.sshPlan, b.k8sPlan)
}

func (b *explainBuilder) setGRPC(req *restfile.Request) {
	setExplainGRPC(b.report, req)
}

func (b *explainBuilder) setHTTP(resp *httpclient.Response) {
	setExplainHTTP(b.report, resp)
}

func (b *explainBuilder) finish(
	status xplain.Status,
	decision string,
	err error,
) *xplain.Report {
	return b.model.finalizeExplainReport(explainFinalizeInput{
		report:         b.report,
		request:        b.request,
		envName:        b.envName,
		preview:        b.preview,
		status:         status,
		decision:       decision,
		err:            err,
		trace:          b.trace,
		mergedSettings: b.mergedSettings,
		sshPlan:        b.sshPlan,
		k8sPlan:        b.k8sPlan,
		globals:        b.globals,
		extraSecrets:   b.extraSecrets,
	})
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

func (m *Model) finalizeExplainReport(in explainFinalizeInput) *xplain.Report {
	if in.report == nil {
		return nil
	}
	if in.trace != nil {
		finalizeExplainVars(in.report, in.trace)
	}
	if in.report.Final == nil {
		setExplainPrepared(in.report, in.request, in.mergedSettings, in.sshPlan, in.k8sPlan)
	}
	fail := in.report.Failure
	if in.err != nil && strings.TrimSpace(fail) == "" {
		fail = in.err.Error()
	}
	decision := in.decision
	if strings.TrimSpace(decision) == "" {
		decision = in.report.Decision
	}
	if strings.TrimSpace(decision) == "" {
		switch in.status {
		case xplain.StatusSkipped:
			decision = "Request skipped"
		case xplain.StatusError:
			decision = "Request preparation failed"
		default:
			if in.preview {
				decision = "Explain preview ready"
			} else {
				decision = "Request prepared"
			}
		}
	}
	setExplainDecision(in.report, in.status, decision, fail)
	return m.redactExplainReportWithState(
		in.report,
		in.envName,
		in.request,
		in.globals,
		in.extraSecrets...,
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
		executed:    e.req,
		environment: e.envName,
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

	e.explain.addSecrets(explainInjectedAuthSecrets(e.req.Metadata.Auth, authBefore, e.req)...)
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

func (m *Model) prepareExplainHTTPPreview(
	ctx context.Context,
	rep *xplain.Report,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts httpclient.Options,
) error {
	if rep == nil || req == nil {
		return nil
	}
	c := m.client
	if c == nil {
		c = httpclient.NewClient(nil)
	}
	httpReq, _, body, err := c.BuildHTTPRequest(ctx, req, resolver, opts)
	if err != nil {
		return err
	}
	if req.SSE != nil && httpReq.Header.Get("Accept") == "" {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	addExplainPreparedHTTPStage(rep, req, httpReq, body)
	setExplainHTTPPrepared(rep, req, httpReq, body)
	return nil
}

func streamingPlaceholderResponse(meta httpclient.StreamMeta) *httpclient.Response {
	headers := meta.Headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}

	headers.Set(streamHeaderType, "websocket")
	headers.Set(streamHeaderSummary, "streaming")
	status := meta.Status
	if strings.TrimSpace(status) == "" {
		status = "101 Switching Protocols"
	}

	statusCode := meta.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusSwitchingProtocols
	}

	return &httpclient.Response{
		Status:         status,
		StatusCode:     statusCode,
		Proto:          meta.Proto,
		Headers:        headers,
		ReqMethod:      meta.RequestMethod,
		RequestHeaders: cloneHeader(meta.RequestHeaders),
		ReqHost:        meta.RequestHost,
		ReqLen:         meta.RequestLength,
		ReqTE:          append([]string(nil), meta.RequestTE...),
		EffectiveURL:   meta.EffectiveURL,
		Request:        meta.Request,
	}
}

func (m *Model) expandWebSocketSteps(req *restfile.Request, resolver *vars.Resolver) error {
	if req == nil || req.WebSocket == nil || resolver == nil {
		return nil
	}

	steps := req.WebSocket.Steps
	if len(steps) == 0 {
		return nil
	}

	for i := range steps {
		step := &steps[i]
		if trimmed := strings.TrimSpace(step.Value); trimmed != "" {
			expanded, err := resolver.ExpandTemplates(trimmed)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand websocket step value")
			}
			step.Value = expanded
		}
		if trimmed := strings.TrimSpace(step.File); trimmed != "" {
			expanded, err := resolver.ExpandTemplates(trimmed)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand websocket file path")
			}
			step.File = expanded
		}
		if trimmed := strings.TrimSpace(step.Reason); trimmed != "" {
			expanded, err := resolver.ExpandTemplates(trimmed)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand websocket close reason")
			}
			step.Reason = expanded
		}
	}

	req.WebSocket.Steps = steps
	return nil
}

func httpScriptResponse(resp *httpclient.Response) *scripts.Response {
	if resp == nil {
		return nil
	}
	return &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: resp.Status,
		Code:   resp.StatusCode,
		URL:    resp.EffectiveURL,
		Time:   resp.Duration,
		Header: cloneHeader(resp.Headers),
		Body:   append([]byte(nil), resp.Body...),
	}
}

func grpcScriptResponse(req *restfile.Request, resp *grpcclient.Response) *scripts.Response {
	if resp == nil {
		return nil
	}

	body := append([]byte(nil), resp.Body...)
	if len(body) == 0 && strings.TrimSpace(resp.Message) != "" {
		body = []byte(resp.Message)
	}
	wire := append([]byte(nil), resp.Wire...)
	wireCT := strings.TrimSpace(resp.WireContentType)
	ct := strings.TrimSpace(resp.ContentType)
	if ct == "" {
		ct = "application/json"
	}

	headers := make(http.Header)
	for name, values := range resp.Headers {
		for _, value := range values {
			headers.Add(name, value)
		}
	}
	for name, values := range resp.Trailers {
		key := "Grpc-Trailer-" + name
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	if headers.Get("Content-Type") == "" && ct != "" {
		headers.Set("Content-Type", ct)
	}

	status := resp.StatusCode.String()
	if msg := strings.TrimSpace(resp.StatusMessage); msg != "" && !strings.EqualFold(msg, status) {
		status = fmt.Sprintf("%s (%s)", status, msg)
	}

	target := ""
	if req != nil && req.GRPC != nil {
		target = strings.TrimSpace(req.GRPC.Target)
	}

	return &scripts.Response{
		Kind:            scripts.ResponseKindGRPC,
		Status:          status,
		Code:            int(resp.StatusCode),
		URL:             target,
		Time:            resp.Duration,
		Header:          headers,
		Body:            body,
		Wire:            wire,
		WireContentType: wireCT,
		ContentType:     ct,
	}
}

const (
	statusPulseInterval = 1 * time.Second
	tabSpinInterval     = 100 * time.Millisecond
)
const (
	streamHeaderType    = "X-Resterm-Stream-Type"
	streamHeaderSummary = "X-Resterm-Stream-Summary"
)

func (m *Model) startSending() tea.Cmd {
	m.sending = true
	return m.startTabSpin()
}

func (m *Model) stopSending() {
	m.sending = false
	m.stopTabSpinIfIdle()
}

func (m *Model) stopStatusPulse() {
	m.statusPulseOn = false
	m.statusPulseBase = ""
	m.statusPulseFrame = 0
}

func (m *Model) stopStatusPulseIfIdle() {
	if m.hasActiveRun() {
		return
	}
	m.stopStatusPulse()
}

func (m *Model) scheduleStatusPulse() tea.Cmd {
	if !m.statusPulseOn || !m.hasActiveRun() {
		return nil
	}
	seq := m.statusPulseSeq
	return tea.Tick(statusPulseInterval, func(time.Time) tea.Msg {
		return statusPulseMsg{seq: seq}
	})
}

func (m *Model) startStatusPulse() tea.Cmd {
	if m.statusPulseOn {
		return nil
	}
	m.statusPulseOn = true
	m.statusPulseSeq++
	m.statusPulseFrame = 0
	return m.scheduleStatusPulse()
}

func (m *Model) stopTabSpin() {
	m.tabSpinOn = false
	m.tabSpinIdx = 0
}

func (m *Model) stopTabSpinIfIdle() {
	if m.spinnerActive() {
		return
	}
	m.stopTabSpin()
}

func (m *Model) scheduleTabSpin() tea.Cmd {
	if !m.tabSpinOn || !m.spinnerActive() || len(tabSpinFrames) == 0 {
		return nil
	}
	seq := m.tabSpinSeq
	return tea.Tick(tabSpinInterval, func(time.Time) tea.Msg {
		return tabSpinMsg{seq: seq}
	})
}

func (m *Model) startTabSpin() tea.Cmd {
	if m.tabSpinOn || !m.spinnerActive() || len(tabSpinFrames) == 0 {
		return nil
	}
	m.tabSpinOn = true
	m.tabSpinSeq++
	m.tabSpinIdx = 0
	return m.scheduleTabSpin()
}

func (m *Model) handleTabSpin(msg tabSpinMsg) tea.Cmd {
	if msg.seq != m.tabSpinSeq {
		return nil
	}
	if !m.tabSpinOn || !m.spinnerActive() || len(tabSpinFrames) == 0 {
		m.stopTabSpin()
		return nil
	}
	m.tabSpinIdx++
	if m.tabSpinIdx >= len(tabSpinFrames) {
		m.tabSpinIdx = 0
	}
	return m.scheduleTabSpin()
}

func (m *Model) handleStatusPulse(msg statusPulseMsg) tea.Cmd {
	if msg.seq != m.statusPulseSeq {
		return nil
	}
	if !m.statusPulseOn || !m.hasActiveRun() {
		m.stopStatusPulse()
		return nil
	}

	m.statusPulseFrame++
	if m.statusPulseFrame >= 3 {
		m.statusPulseFrame = 0
	}

	if m.profileRun != nil {
		m.showProfileProgress(m.profileRun)
		return m.scheduleStatusPulse()
	}

	base := strings.TrimSpace(m.statusPulseBase)
	if base == "" {
		base = "Sending"
	}

	dots := strings.Repeat(".", m.statusPulseFrame+1)
	m.setStatusMessage(statusMsg{text: base + dots, level: statusInfo})
	return m.scheduleStatusPulse()
}

func defaultTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return 30 * time.Second
}

func resolveRequestTimeout(req *restfile.Request, base time.Duration) time.Duration {
	if req != nil {
		if raw, ok := req.Settings["timeout"]; ok {
			if dur, err := time.ParseDuration(raw); err == nil && dur > 0 {
				return dur
			}
		}
	}
	return base
}

func (m *Model) buildResolver(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	extraVals map[string]rts.Value,
	extras ...map[string]string,
) *vars.Resolver {
	return m.buildResolverWithGlobals(ctx, doc, req, envName, base, extraVals, nil, extras...)
}

func (m *Model) buildResolverWithGlobals(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	extraVals map[string]rts.Value,
	globals map[string]scripts.GlobalValue,
	extras ...map[string]string,
) *vars.Resolver {
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	providers := make([]vars.Provider, 0, 9)

	if doc != nil && len(doc.Constants) > 0 {
		constValues := make(map[string]string, len(doc.Constants))
		for _, c := range doc.Constants {
			constValues[c.Name] = c.Value
		}
		providers = append(providers, vars.NewMapProvider("const", constValues))
	}

	for _, extra := range extras {
		if len(extra) > 0 {
			providers = append(providers, vars.NewMapProvider("script", extra))
		}
	}

	if req != nil {
		reqVars := make(map[string]string)
		for _, v := range req.Variables {
			reqVars[v.Name] = v.Value
		}
		if len(reqVars) > 0 {
			providers = append(providers, vars.NewMapProvider("request", reqVars))
		}
	}

	if globals != nil {
		if values := globalValueMap(globals); len(values) > 0 {
			providers = append(providers, vars.NewMapProvider("global", values))
		}
	} else if m.globals != nil {
		if snapshot := m.globals.snapshot(resolvedEnv); len(snapshot) > 0 {
			values := make(map[string]string, len(snapshot))
			for key, entry := range snapshot {
				name := entry.Name
				if strings.TrimSpace(name) == "" {
					name = key
				}
				values[name] = entry.Value
			}
			providers = append(providers, vars.NewMapProvider("global", values))
		}
	}

	if doc != nil {
		globalVars := make(map[string]string)
		for _, v := range doc.Globals {
			globalVars[v.Name] = v.Value
		}
		if len(globalVars) > 0 {
			providers = append(providers, vars.NewMapProvider("document-global", globalVars))
		}
	}

	fileVars := make(map[string]string)
	if doc != nil {
		for _, v := range doc.Variables {
			fileVars[v.Name] = v.Value
		}
	}
	m.mergeFileRuntimeVars(fileVars, doc, resolvedEnv)
	if len(fileVars) > 0 {
		providers = append(providers, vars.NewMapProvider("file", fileVars))
	}

	if envValues := vars.EnvValues(m.cfg.EnvironmentSet, resolvedEnv); len(envValues) > 0 {
		providers = append(providers, vars.NewMapProvider("environment", envValues))
	}

	providers = append(providers, vars.EnvProvider{})
	res := vars.NewResolver(providers...)
	res.AddRefResolver(vars.EnvRefResolver)
	res.SetExprEval(m.rtsEval(ctx, doc, req, resolvedEnv, base, false, extraVals, extras...))
	res.SetExprPos(m.rtsPos(doc, req))
	return res
}

// buildDisplayResolver is a best-effort resolver for UI/status rendering that
// avoids expanding secret values.
func (m *Model) buildDisplayResolver(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	extraVals map[string]rts.Value,
	extras ...map[string]string,
) *vars.Resolver {
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	providers := make([]vars.Provider, 0, 9)

	if doc != nil && len(doc.Constants) > 0 {
		constValues := make(map[string]string, len(doc.Constants))
		for _, c := range doc.Constants {
			constValues[c.Name] = c.Value
		}
		providers = append(providers, vars.NewMapProvider("const", constValues))
	}

	for _, extra := range extras {
		if len(extra) > 0 {
			providers = append(providers, vars.NewMapProvider("script", extra))
		}
	}

	if req != nil {
		reqVars := make(map[string]string)
		for _, v := range req.Variables {
			if v.Secret {
				continue
			}
			reqVars[v.Name] = v.Value
		}
		if len(reqVars) > 0 {
			providers = append(providers, vars.NewMapProvider("request", reqVars))
		}
	}

	if m.globals != nil {
		if snapshot := m.globals.snapshot(resolvedEnv); len(snapshot) > 0 {
			values := make(map[string]string, len(snapshot))
			for key, entry := range snapshot {
				if entry.Secret {
					continue
				}
				name := entry.Name
				if strings.TrimSpace(name) == "" {
					name = key
				}
				values[name] = entry.Value
			}
			if len(values) > 0 {
				providers = append(providers, vars.NewMapProvider("global", values))
			}
		}
	}

	if doc != nil {
		globalVars := make(map[string]string)
		for _, v := range doc.Globals {
			if v.Secret {
				continue
			}
			globalVars[v.Name] = v.Value
		}
		if len(globalVars) > 0 {
			providers = append(providers, vars.NewMapProvider("document-global", globalVars))
		}
	}

	fileVars := make(map[string]string)
	if doc != nil {
		for _, v := range doc.Variables {
			if v.Secret {
				continue
			}
			fileVars[v.Name] = v.Value
		}
	}
	m.mergeFileRuntimeVarsSafe(fileVars, doc, resolvedEnv)
	if len(fileVars) > 0 {
		providers = append(providers, vars.NewMapProvider("file", fileVars))
	}

	if envValues := vars.EnvValues(m.cfg.EnvironmentSet, resolvedEnv); len(envValues) > 0 {
		providers = append(providers, vars.NewMapProvider("environment", envValues))
	}

	providers = append(providers, vars.EnvProvider{})
	res := vars.NewResolver(providers...)
	res.AddRefResolver(vars.EnvRefResolver)
	res.SetExprEval(m.rtsEval(ctx, doc, req, resolvedEnv, base, true, extraVals, extras...))
	res.SetExprPos(m.rtsPos(doc, req))
	return res
}

func (m *Model) resolveSSH(
	doc *restfile.Document,
	req *restfile.Request,
	resolver *vars.Resolver,
	envName string,
) (*ssh.Plan, error) {
	if req == nil || req.SSH == nil {
		return nil, nil
	}
	manager := m.ensureSSHManager()
	fileProfiles := docSSHProfiles(doc)
	globalProfiles := []restfile.SSHProfile(nil)
	if m.sshGlobals != nil {
		globalProfiles = m.sshGlobals.all()
	}
	cfg, err := ssh.Resolve(req.SSH, fileProfiles, globalProfiles, resolver, envName)
	if err != nil {
		return nil, err
	}
	if cfg != nil && !cfg.Strict {
		m.setStatusMessage(statusMsg{
			text:  "@ssh strict_hostkey=false (insecure)",
			level: statusWarn,
		})
	}
	return &ssh.Plan{Manager: manager, Config: cfg}, nil
}

func (m *Model) resolveK8s(
	doc *restfile.Document,
	req *restfile.Request,
	resolver *vars.Resolver,
	envName string,
) (*k8s.Plan, error) {
	if req == nil || req.K8s == nil {
		return nil, nil
	}
	manager := m.ensureK8sManager()
	fileProfiles := docK8sProfiles(doc)
	globalProfiles := []restfile.K8sProfile(nil)
	if m.k8sGlobals != nil {
		globalProfiles = m.k8sGlobals.all()
	}
	cfg, err := k8s.Resolve(req.K8s, fileProfiles, globalProfiles, resolver, envName)
	if err != nil {
		return nil, err
	}
	return &k8s.Plan{Manager: manager, Config: cfg}, nil
}

func (m *Model) documentRuntimePath(doc *restfile.Document) string {
	if doc != nil && strings.TrimSpace(doc.Path) != "" {
		return doc.Path
	}
	return m.currentFile
}

func (m *Model) ensureSSHManager() *ssh.Manager {
	if m.sshMgr != nil {
		return m.sshMgr
	}
	// Defensive for zero-value models used in tests and non-UI helpers.
	m.sshMgr = ssh.NewManager()
	return m.sshMgr
}

func (m *Model) ensureK8sManager() *k8s.Manager {
	if m.k8sMgr != nil {
		return m.k8sMgr
	}
	// Defensive for zero-value models used in tests and non-UI helpers.
	m.k8sMgr = k8s.NewManager()
	return m.k8sMgr
}

func (m *Model) syncSSHGlobals(doc *restfile.Document) {
	if m.sshGlobals == nil {
		return
	}
	path := m.documentRuntimePath(doc)
	m.sshGlobals.set(path, docSSHProfiles(doc))
}

func (m *Model) syncK8sGlobals(doc *restfile.Document) {
	if m.k8sGlobals == nil {
		return
	}
	path := m.documentRuntimePath(doc)
	m.k8sGlobals.set(path, docK8sProfiles(doc))
}

func docSSHProfiles(doc *restfile.Document) []restfile.SSHProfile {
	if doc == nil {
		return nil
	}
	return doc.SSH
}

func docK8sProfiles(doc *restfile.Document) []restfile.K8sProfile {
	if doc == nil {
		return nil
	}
	return doc.K8s
}

func (m *Model) syncPatchGlobals(doc *restfile.Document) {
	if m.patchGlobals == nil {
		return
	}
	path := m.documentRuntimePath(doc)
	m.patchGlobals.set(path, docPatchProfiles(doc))
}

func (m *Model) syncAllGlobals(doc *restfile.Document) {
	m.syncSSHGlobals(doc)
	m.syncK8sGlobals(doc)
	m.syncPatchGlobals(doc)
}

func docPatchProfiles(doc *restfile.Document) []restfile.PatchProfile {
	if doc == nil {
		return nil
	}
	return doc.Patches
}

func (m *Model) mergeFileRuntimeVars(
	target map[string]string,
	doc *restfile.Document,
	envName string,
) {
	if target == nil || m.fileVars == nil {
		return
	}
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	path := m.documentRuntimePath(doc)
	if snapshot := m.fileVars.snapshot(resolvedEnv, path); len(snapshot) > 0 {
		for key, entry := range snapshot {
			name := strings.TrimSpace(entry.Name)
			if name == "" {
				name = key
			}
			target[name] = entry.Value
		}
	}
}

// mergeFileRuntimeVarsSafe merges runtime file vars while skipping secrets so UI
// previews do not leak them.
func (m *Model) mergeFileRuntimeVarsSafe(
	target map[string]string,
	doc *restfile.Document,
	envName string,
) {
	if target == nil || m.fileVars == nil {
		return
	}
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	path := m.documentRuntimePath(doc)
	if snapshot := m.fileVars.snapshot(resolvedEnv, path); len(snapshot) > 0 {
		for key, entry := range snapshot {
			if entry.Secret {
				continue
			}
			name := strings.TrimSpace(entry.Name)
			if name == "" {
				name = key
			}
			target[name] = entry.Value
		}
	}
}

func (m *Model) collectVariables(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
) map[string]string {
	return m.collectVariablesWithStoreGlobals(
		doc,
		req,
		envName,
		m.collectStoredGlobalValues(envName),
	)
}

func (m *Model) collectVariablesWithStoreGlobals(
	doc *restfile.Document,
	req *restfile.Request,
	envName string,
	storeGlobals map[string]scripts.GlobalValue,
) map[string]string {
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	result := make(map[string]string)
	if env := vars.EnvValues(m.cfg.EnvironmentSet, resolvedEnv); env != nil {
		for k, v := range env {
			result[k] = v
		}
	}

	if doc != nil {
		for _, v := range doc.Variables {
			result[v.Name] = v.Value
		}
		for _, v := range doc.Globals {
			result[v.Name] = v.Value
		}
	}

	m.mergeFileRuntimeVars(result, doc, resolvedEnv)
	for name, value := range globalValueMap(storeGlobals) {
		result[name] = value
	}

	if req != nil {
		for _, v := range req.Variables {
			result[v.Name] = v.Value
		}
	}
	return result
}

func (m *Model) collectGlobalValues(
	doc *restfile.Document,
	envName string,
) map[string]scripts.GlobalValue {
	return effectiveGlobalValues(doc, m.collectStoredGlobalValues(envName))
}

func collectDocumentGlobalValues(doc *restfile.Document) map[string]scripts.GlobalValue {
	globals := make(map[string]scripts.GlobalValue)
	if doc != nil {
		for _, v := range doc.Globals {
			name := strings.TrimSpace(v.Name)
			if name == "" {
				continue
			}
			globals[name] = scripts.GlobalValue{Name: name, Value: v.Value, Secret: v.Secret}
		}
	}
	if len(globals) == 0 {
		return nil
	}
	return globals
}

func (m *Model) collectStoredGlobalValues(envName string) map[string]scripts.GlobalValue {
	resolvedEnv := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	globals := make(map[string]scripts.GlobalValue)
	if m.globals != nil {
		if snapshot := m.globals.snapshot(resolvedEnv); len(snapshot) > 0 {
			for key, entry := range snapshot {
				name := strings.TrimSpace(entry.Name)
				if name == "" {
					name = key
				}
				globals[name] = scripts.GlobalValue{
					Name:   name,
					Value:  entry.Value,
					Secret: entry.Secret,
				}
			}
		}
	}
	if len(globals) == 0 {
		return nil
	}
	return globals
}

func effectiveGlobalValues(
	doc *restfile.Document,
	storeGlobals map[string]scripts.GlobalValue,
) map[string]scripts.GlobalValue {
	return mergeGlobalValues(collectDocumentGlobalValues(doc), storeGlobals)
}

func cloneGlobalValues(src map[string]scripts.GlobalValue) map[string]scripts.GlobalValue {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]scripts.GlobalValue, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}

func mergeGlobalValues(
	base map[string]scripts.GlobalValue,
	changes map[string]scripts.GlobalValue,
) map[string]scripts.GlobalValue {
	if len(base) == 0 && len(changes) == 0 {
		return nil
	}
	out := cloneGlobalValues(base)
	if out == nil {
		out = make(map[string]scripts.GlobalValue, len(changes))
	}
	for key, change := range changes {
		name := strings.TrimSpace(change.Name)
		if name == "" {
			name = strings.TrimSpace(key)
		}
		if name == "" {
			continue
		}
		for existing := range out {
			if strings.EqualFold(strings.TrimSpace(existing), name) {
				delete(out, existing)
			}
		}
		if change.Delete {
			continue
		}
		change.Name = name
		out[name] = change
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func globalValueMap(globals map[string]scripts.GlobalValue) map[string]string {
	if len(globals) == 0 {
		return nil
	}
	values := make(map[string]string, len(globals))
	for key, entry := range globals {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			name = strings.TrimSpace(key)
		}
		if name == "" || entry.Delete {
			continue
		}
		values[name] = entry.Value
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func (m *Model) applyGlobalMutations(changes map[string]scripts.GlobalValue, envName string) {
	if len(changes) == 0 || m.globals == nil {
		return
	}

	env := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	for _, change := range changes {
		name := strings.TrimSpace(change.Name)
		if name == "" {
			continue
		}
		if change.Delete {
			m.globals.delete(env, name)
			continue
		}
		m.globals.set(env, name, change.Value, change.Secret)
	}
}

func (m *Model) showGlobalSummary() tea.Cmd {
	text := m.buildGlobalSummary()
	if strings.TrimSpace(text) == "" {
		text = "Globals: (empty)"
	}
	m.setStatusMessage(statusMsg{level: statusInfo, text: text})
	return nil
}

func (m *Model) buildGlobalSummary() string {
	var segments []string

	if snapshot := m.globalsSnapshot(); len(snapshot) > 0 {
		entries := make([]summaryEntry, 0, len(snapshot))
		for key, value := range snapshot {
			name := strings.TrimSpace(value.Name)
			if name == "" {
				name = key
			}
			entries = append(
				entries,
				summaryEntry{name: name, value: value.Value, secret: value.Secret},
			)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			parts = append(
				parts,
				fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)),
			)
		}
		segments = append(segments, "Globals: "+strings.Join(parts, ", "))
	}

	if doc := m.doc; doc != nil {
		entries := make([]summaryEntry, 0, len(doc.Globals))
		for _, global := range doc.Globals {
			name := strings.TrimSpace(global.Name)
			if name == "" {
				continue
			}
			entries = append(
				entries,
				summaryEntry{name: name, value: global.Value, secret: global.Secret},
			)
		}
		if len(entries) > 0 {
			sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
			parts := make([]string, 0, len(entries))
			for _, entry := range entries {
				parts = append(
					parts,
					fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)),
				)
			}
			segments = append(segments, "Doc: "+strings.Join(parts, ", "))
		}
	}

	return strings.Join(segments, " | ")
}

func (m *Model) globalsSnapshot() map[string]globalValue {
	if m.globals == nil {
		return nil
	}
	return m.globals.snapshot(m.cfg.EnvironmentName)
}

func (m *Model) clearGlobalValues() tea.Cmd {
	if m.globals == nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "No global store available"})
		return nil
	}

	env := m.cfg.EnvironmentName
	m.globals.clear(env)
	label := env
	if strings.TrimSpace(label) == "" {
		label = "default"
	}

	m.setStatusMessage(
		statusMsg{level: statusInfo, text: fmt.Sprintf("Cleared globals for %s", label)},
	)
	return nil
}

type summaryEntry struct {
	name   string
	value  string
	secret bool
}

func maskSecret(value string, secret bool) string {
	if secret {
		return "•••"
	}
	return value
}

func explainAuthSecretValues(auth *restfile.AuthSpec, resolver *vars.Resolver) []string {
	if auth == nil || len(auth.Params) == 0 {
		return nil
	}

	expand := func(key string) string {
		value := strings.TrimSpace(auth.Params[key])
		if value == "" {
			return ""
		}
		if resolver == nil {
			return value
		}
		expanded, err := resolver.ExpandTemplates(value)
		if err != nil {
			return value
		}
		return strings.TrimSpace(expanded)
	}

	values := make(map[string]struct{})
	add := func(value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		values[value] = struct{}{}
	}

	switch strings.ToLower(strings.TrimSpace(auth.Type)) {
	case "basic":
		add(expand("password"))
	case "bearer":
		add(expand("token"))
	case "apikey", "api-key", "header":
		add(expand("value"))
	case "oauth2":
		for _, key := range []string{"client_secret", "password", "refresh_token", "access_token"} {
			add(expand(key))
		}
	}

	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	return out
}

func explainInjectedAuthSecrets(
	auth *restfile.AuthSpec,
	before *restfile.Request,
	after *restfile.Request,
) []string {
	if auth == nil || after == nil {
		return nil
	}
	header := "Authorization"
	if strings.EqualFold(strings.TrimSpace(auth.Type), "oauth2") {
		if name := strings.TrimSpace(auth.Params["header"]); name != "" {
			header = name
		}
	}
	beforeValue := headerValue(reqHeaders(before), header)
	afterValue := headerValue(reqHeaders(after), header)
	if strings.TrimSpace(afterValue) == "" || afterValue == beforeValue {
		return nil
	}

	values := []string{afterValue}
	if strings.EqualFold(header, "authorization") {
		_, token, ok := strings.Cut(afterValue, " ")
		if ok {
			token = strings.TrimSpace(token)
			if token != "" {
				values = append(values, token)
			}
		}
	}
	return values
}

func (m *Model) prepareExplainAuthPreview(
	req *restfile.Request,
	resolver *vars.Resolver,
	envName string,
) (explainAuthPreviewResult, error) {
	if req == nil || req.Metadata.Auth == nil {
		return explainAuthPreviewResult{}, nil
	}

	auth := req.Metadata.Auth
	kind := strings.ToLower(strings.TrimSpace(auth.Type))
	switch kind {
	case "", "basic", "bearer", "apikey", "api-key", "header":
		return explainAuthPreviewResult{
			status:  xplain.StageOK,
			summary: explainSummaryAuthPrepared,
			notes:   []string{"auth headers/query are applied during HTTP request build"},
		}, nil
	case "oauth2":
		if m.oauth == nil {
			return explainAuthPreviewResult{}, errdef.New(
				errdef.CodeHTTP,
				"oauth support is not initialised",
			)
		}

		cfg, err := m.buildOAuthConfig(auth, resolver)
		if err != nil {
			return explainAuthPreviewResult{}, err
		}

		envKey := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
		cfg = m.oauth.MergeCachedConfig(envKey, cfg)
		if cfg.TokenURL == "" {
			return explainAuthPreviewResult{}, errdef.New(
				errdef.CodeHTTP,
				"@auth oauth2 requires token_url (include it once per cache_key to seed the cache)",
			)
		}

		header := strings.TrimSpace(cfg.Header)
		if header == "" {
			header = "Authorization"
		}
		if req.Headers != nil && req.Headers.Get(header) != "" {
			return explainAuthPreviewResult{
				status:  xplain.StageOK,
				summary: explainSummaryAuthPrepared,
				notes:   []string{"auth header already set on request"},
			}, nil
		}

		token, ok := m.oauth.CachedToken(envKey, cfg)
		if !ok {
			return explainAuthPreviewResult{
				status:  xplain.StageSkipped,
				summary: explainSummaryOAuthTokenFetchSkipped,
				notes: []string{
					"OAuth token acquisition is skipped in explain preview",
					fmt.Sprintf("%s is omitted without a cached token", header),
				},
			}, nil
		}

		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		value := token.AccessToken
		if strings.EqualFold(header, "authorization") {
			typeValue := strings.TrimSpace(token.TokenType)
			if typeValue == "" {
				typeValue = "Bearer"
			}
			value = strings.TrimSpace(typeValue) + " " + token.AccessToken
		}
		req.Headers.Set(header, value)

		return explainAuthPreviewResult{
			status:  xplain.StageOK,
			summary: explainSummaryAuthPrepared,
			notes:   []string{"used cached OAuth token for explain preview"},
			extraSecrets: []string{
				token.AccessToken,
				value,
			},
		}, nil
	default:
		return explainAuthPreviewResult{
			status:  xplain.StageSkipped,
			summary: explainSummaryAuthTypeNotApplied,
			notes:   []string{fmt.Sprintf("unsupported auth type %q is not applied", auth.Type)},
		}, nil
	}
}

func (m *Model) ensureOAuth(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts httpclient.Options,
	envName string,
	timeout time.Duration,
) error {
	if req == nil || req.Metadata.Auth == nil {
		return nil
	}
	if !strings.EqualFold(req.Metadata.Auth.Type, "oauth2") {
		return nil
	}
	if m.oauth == nil {
		return errdef.New(errdef.CodeHTTP, "oauth support is not initialised")
	}

	cfg, err := m.buildOAuthConfig(req.Metadata.Auth, resolver)
	if err != nil {
		return err
	}

	envKey := vars.SelectEnv(m.cfg.EnvironmentSet, envName, m.cfg.EnvironmentName)
	cfg = m.oauth.MergeCachedConfig(envKey, cfg)
	if cfg.TokenURL == "" {
		return errdef.New(
			errdef.CodeHTTP,
			"@auth oauth2 requires token_url (include it once per cache_key to seed the cache)",
		)
	}

	grant := strings.ToLower(strings.TrimSpace(cfg.GrantType))
	header := cfg.Header
	if strings.TrimSpace(header) == "" {
		header = "Authorization"
	}
	if req.Headers != nil && req.Headers.Get(header) != "" {
		return nil
	}

	tokenTimeout := timeout
	if grant == "authorization_code" && tokenTimeout < 2*time.Minute {
		tokenTimeout = 2 * time.Minute
		m.setStatusMessage(
			statusMsg{
				level: statusInfo,
				text:  "Open browser to complete OAuth (auth code/PKCE). Press send again to cancel.",
			},
		)
	}

	ctx, cancel := context.WithTimeout(ctx, tokenTimeout)

	defer cancel()

	token, err := m.oauth.Token(ctx, envKey, cfg, opts)
	if err != nil {
		return errdef.Wrap(errdef.CodeHTTP, err, "fetch oauth token")
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	if req.Headers.Get(header) != "" {
		return nil
	}

	value := token.AccessToken
	if strings.EqualFold(header, "authorization") {
		typeValue := strings.TrimSpace(token.TokenType)
		if typeValue == "" {
			typeValue = "Bearer"
		}
		value = strings.TrimSpace(typeValue) + " " + token.AccessToken
	}

	req.Headers.Set(header, value)
	return nil
}

func (m *Model) buildOAuthConfig(
	auth *restfile.AuthSpec,
	resolver *vars.Resolver,
) (oauth.Config, error) {
	cfg := oauth.Config{Extra: make(map[string]string)}
	if auth == nil {
		return cfg, errdef.New(errdef.CodeHTTP, "missing oauth spec")
	}

	expand := func(key string) (string, error) {
		value := auth.Params[key]
		if strings.TrimSpace(value) == "" {
			return "", nil
		}
		if resolver == nil {
			return strings.TrimSpace(value), nil
		}
		expanded, err := resolver.ExpandTemplates(value)
		if err != nil {
			return "", errdef.Wrap(errdef.CodeHTTP, err, "expand oauth param %s", key)
		}
		return strings.TrimSpace(expanded), nil
	}

	var err error
	if cfg.TokenURL, err = expand("token_url"); err != nil {
		return cfg, err
	}
	if cfg.AuthURL, err = expand("auth_url"); err != nil {
		return cfg, err
	}
	if cfg.RedirectURL, err = expand("redirect_uri"); err != nil {
		return cfg, err
	}
	if cfg.ClientID, err = expand("client_id"); err != nil {
		return cfg, err
	}
	if cfg.ClientSecret, err = expand("client_secret"); err != nil {
		return cfg, err
	}
	if cfg.Scope, err = expand("scope"); err != nil {
		return cfg, err
	}
	if cfg.Audience, err = expand("audience"); err != nil {
		return cfg, err
	}
	if cfg.Resource, err = expand("resource"); err != nil {
		return cfg, err
	}
	if cfg.Username, err = expand("username"); err != nil {
		return cfg, err
	}
	if cfg.Password, err = expand("password"); err != nil {
		return cfg, err
	}
	if cfg.ClientAuth, err = expand("client_auth"); err != nil {
		return cfg, err
	}
	if cfg.GrantType, err = expand("grant"); err != nil {
		return cfg, err
	}
	if cfg.Header, err = expand("header"); err != nil {
		return cfg, err
	}
	if cfg.CacheKey, err = expand("cache_key"); err != nil {
		return cfg, err
	}
	if cfg.CodeVerifier, err = expand("code_verifier"); err != nil {
		return cfg, err
	}
	if cfg.CodeMethod, err = expand("code_challenge_method"); err != nil {
		return cfg, err
	}
	if cfg.State, err = expand("state"); err != nil {
		return cfg, err
	}

	known := map[string]struct{}{
		"token_url":             {},
		"auth_url":              {},
		"redirect_uri":          {},
		"client_id":             {},
		"client_secret":         {},
		"scope":                 {},
		"audience":              {},
		"resource":              {},
		"username":              {},
		"password":              {},
		"client_auth":           {},
		"grant":                 {},
		"header":                {},
		"cache_key":             {},
		"code_verifier":         {},
		"code_challenge_method": {},
		"state":                 {},
	}
	for key, raw := range auth.Params {
		if _, ok := known[strings.ToLower(key)]; ok {
			continue
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		value, err := expand(key)
		if err != nil {
			return cfg, err
		}
		if value != "" {
			cfg.Extra[strings.ToLower(strings.ReplaceAll(key, "-", "_"))] = value
		}
	}
	if len(cfg.Extra) == 0 {
		cfg.Extra = nil
	}
	return cfg, nil
}

func mergeVariableMaps(base map[string]string, additions map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(additions))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range additions {
		merged[k] = v
	}
	return merged
}

func (m *Model) resolveHTTPOptions(opts httpclient.Options) httpclient.Options {
	if opts.BaseDir == "" && m.currentFile != "" {
		opts.BaseDir = filepath.Dir(m.currentFile)
	}

	if fallbackEnabled() {
		fallbacks := make([]string, 0, len(opts.FallbackBaseDirs)+3)
		fallbacks = append(fallbacks, opts.FallbackBaseDirs...)
		fallbacks = append(fallbacks, opts.BaseDir)
		if m.workspaceRoot != "" {
			fallbacks = append(fallbacks, m.workspaceRoot)
		}
		if cwd, err := os.Getwd(); err == nil {
			fallbacks = append(fallbacks, cwd)
		}
		opts.FallbackBaseDirs = util.DedupeNonEmptyStrings(fallbacks)
		opts.NoFallback = false
	} else {
		opts.FallbackBaseDirs = nil
		opts.NoFallback = true
	}
	return opts
}

func fallbackEnabled() bool {
	val := strings.ToLower(strings.TrimSpace(os.Getenv("RESTERM_ENABLE_FALLBACK")))
	return val == "1" || val == "true" || val == "yes"
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	clone := make(map[string]string, len(input))
	for k, v := range input {
		clone[k] = v
	}
	return clone
}

func (m *Model) prepareGRPCRequest(
	req *restfile.Request,
	resolver *vars.Resolver,
	baseDir string,
) error {
	grpcReq := req.GRPC
	if grpcReq == nil {
		return nil
	}

	if strings.TrimSpace(grpcReq.FullMethod) == "" {
		service := strings.TrimSpace(grpcReq.Service)
		method := strings.TrimSpace(grpcReq.Method)
		if service != "" && method != "" {
			if grpcReq.Package != "" {
				grpcReq.FullMethod = "/" + grpcReq.Package + "." + service + "/" + method
			} else {
				grpcReq.FullMethod = "/" + service + "/" + method
			}
		} else {
			return errdef.New(errdef.CodeHTTP, "grpc method metadata is incomplete")
		}
	}

	if text := strings.TrimSpace(req.Body.Text); text != "" {
		grpcReq.Message = req.Body.Text
		grpcReq.MessageFile = ""
	} else if file := strings.TrimSpace(req.Body.FilePath); file != "" {
		grpcReq.MessageFile = req.Body.FilePath
		grpcReq.Message = ""
	}
	grpcReq.MessageExpanded = ""
	grpcReq.MessageExpandedSet = false

	if err := grpcclient.ValidateMetaPairs(grpcReq.Metadata); err != nil {
		return err
	}
	if err := grpcclient.ValidateHeaderPairs(req.Headers); err != nil {
		return err
	}

	if resolver != nil {
		target, err := resolver.ExpandTemplates(grpcReq.Target)
		if err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc target")
		}

		grpcReq.Target = strings.TrimSpace(target)
		if strings.TrimSpace(grpcReq.Message) != "" {
			expanded, err := resolver.ExpandTemplates(grpcReq.Message)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc message")
			}
			grpcReq.Message = expanded
		}
		if req.Body.Options.ExpandTemplates && strings.TrimSpace(grpcReq.MessageFile) != "" {
			expanded, err := expandGRPCMessageFile(grpcReq.MessageFile, baseDir, resolver)
			if err != nil {
				return err
			}
			grpcReq.MessageExpanded = expanded
			grpcReq.MessageExpandedSet = true
		}
		if len(grpcReq.Metadata) > 0 {
			for i := range grpcReq.Metadata {
				value := grpcReq.Metadata[i].Value
				expanded, err := resolver.ExpandTemplates(value)
				if err != nil {
					return errdef.Wrap(
						errdef.CodeHTTP,
						err,
						"expand grpc metadata %s",
						grpcReq.Metadata[i].Key,
					)
				}
				grpcReq.Metadata[i].Value = expanded
			}
		}
		if authority := strings.TrimSpace(grpcReq.Authority); authority != "" {
			expanded, err := resolver.ExpandTemplates(authority)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc authority")
			}
			grpcReq.Authority = strings.TrimSpace(expanded)
		}
		if descriptor := strings.TrimSpace(grpcReq.DescriptorSet); descriptor != "" {
			expanded, err := resolver.ExpandTemplates(descriptor)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc descriptor set")
			}
			grpcReq.DescriptorSet = strings.TrimSpace(expanded)
		}

		if req.Headers != nil {
			for key, values := range req.Headers {
				for i, value := range values {
					expanded, err := resolver.ExpandTemplates(value)
					if err != nil {
						return errdef.Wrap(errdef.CodeHTTP, err, "expand header %s", key)
					}
					req.Headers[key][i] = expanded
				}
			}
		}
	}

	grpcReq.Target = strings.TrimSpace(grpcReq.Target)
	grpcReq.Target = normalizeGRPCTarget(grpcReq.Target, grpcReq)
	if grpcReq.Target == "" {
		return errdef.New(errdef.CodeHTTP, "grpc target not specified")
	}
	req.URL = grpcReq.Target
	return nil
}

func expandGRPCMessageFile(
	path string,
	baseDir string,
	resolver *vars.Resolver,
) (string, error) {
	if resolver == nil {
		return "", nil
	}
	full := path
	if !filepath.IsAbs(full) && baseDir != "" {
		full = filepath.Join(baseDir, full)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", errdef.Wrap(errdef.CodeFilesystem, err, "read grpc message file %s", path)
	}
	expanded, err := resolver.ExpandTemplates(string(data))
	if err != nil {
		return "", errdef.Wrap(errdef.CodeHTTP, err, "expand grpc message file")
	}
	return expanded, nil
}

func normalizeGRPCTarget(target string, grpcReq *restfile.GRPCRequest) string {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return ""
	}

	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, "grpcs://"):
		if grpcReq != nil && !grpcReq.PlaintextSet {
			grpcReq.Plaintext = false
			grpcReq.PlaintextSet = true
		}
		return trimmed[len("grpcs://"):]
	case strings.HasPrefix(lower, "https://"):
		if grpcReq != nil && !grpcReq.PlaintextSet {
			grpcReq.Plaintext = false
			grpcReq.PlaintextSet = true
		}
		return trimmed[len("https://"):]
	case strings.HasPrefix(lower, "grpc://"):
		return trimmed[len("grpc://"):]
	case strings.HasPrefix(lower, "http://"):
		return trimmed[len("http://"):]
	default:
		return trimmed
	}
}

func applyPreRequestOutput(req *restfile.Request, out scripts.PreRequestOutput) error {
	if out.Method != nil {
		req.Method = strings.ToUpper(strings.TrimSpace(*out.Method))
	}

	if out.URL != nil {
		req.URL = strings.TrimSpace(*out.URL)
	}

	if len(out.Query) > 0 {
		if err := applyPreRequestQuery(req, out.Query); err != nil {
			return errdef.Wrap(errdef.CodeScript, err, "invalid url after script")
		}
	}
	if out.Headers != nil {
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		for name, values := range out.Headers {
			req.Headers.Del(name)
			for _, value := range values {
				req.Headers.Add(name, value)
			}
		}
	}
	if out.Body != nil {
		req.Body.FilePath = ""
		req.Body.Text = *out.Body
		req.Body.GraphQL = nil
	}
	setRequestVars(req, out.Variables)
	return nil
}

func applyPreRequestQuery(req *restfile.Request, q map[string]string) error {
	if req == nil || len(q) == 0 {
		return nil
	}
	raw := strings.TrimSpace(req.URL)
	patch := make(map[string]*string, len(q))
	for key, value := range q {
		val := value
		patch[key] = &val
	}
	updated, err := urltpl.PatchQuery(raw, patch)
	if err != nil {
		return err
	}
	if raw == "" && updated == "" {
		return nil
	}
	req.URL = updated
	return nil
}

func cloneRequest(req *restfile.Request) *restfile.Request {
	if req == nil {
		return nil
	}

	clone := *req
	clone.Headers = cloneHeader(req.Headers)
	if req.Settings != nil {
		clone.Settings = make(map[string]string, len(req.Settings))
		for k, v := range req.Settings {
			clone.Settings[k] = v
		}
	}

	clone.Variables = append([]restfile.Variable(nil), req.Variables...)
	clone.Metadata.Tags = append([]string(nil), req.Metadata.Tags...)
	clone.Metadata.Scripts = append([]restfile.ScriptBlock(nil), req.Metadata.Scripts...)
	clone.Metadata.Uses = append([]restfile.UseSpec(nil), req.Metadata.Uses...)
	if len(req.Metadata.Applies) > 0 {
		clone.Metadata.Applies = make([]restfile.ApplySpec, len(req.Metadata.Applies))
		copy(clone.Metadata.Applies, req.Metadata.Applies)
		for i := range clone.Metadata.Applies {
			clone.Metadata.Applies[i].Uses = append(
				[]string(nil),
				req.Metadata.Applies[i].Uses...,
			)
		}
	}
	clone.Metadata.Asserts = append([]restfile.AssertSpec(nil), req.Metadata.Asserts...)
	clone.Metadata.Captures = append([]restfile.CaptureSpec(nil), req.Metadata.Captures...)
	if req.Metadata.When != nil {
		when := *req.Metadata.When
		clone.Metadata.When = &when
	}
	if req.Metadata.ForEach != nil {
		forEach := *req.Metadata.ForEach
		clone.Metadata.ForEach = &forEach
	}
	if req.Metadata.Compare != nil {
		spec := *req.Metadata.Compare
		if len(spec.Environments) > 0 {
			spec.Environments = append([]string(nil), spec.Environments...)
		}
		clone.Metadata.Compare = &spec
	}
	if req.Body.GraphQL != nil {
		gql := *req.Body.GraphQL
		clone.Body.GraphQL = &gql
	}
	if req.GRPC != nil {
		grpcCopy := *req.GRPC
		if len(grpcCopy.Metadata) > 0 {
			meta := make([]restfile.MetadataPair, len(grpcCopy.Metadata))
			copy(meta, grpcCopy.Metadata)
			grpcCopy.Metadata = meta
		}
		clone.GRPC = &grpcCopy
	}
	if req.SSE != nil {
		sseCopy := *req.SSE
		clone.SSE = &sseCopy
	}
	if req.WebSocket != nil {
		wsCopy := *req.WebSocket
		if len(wsCopy.Options.Subprotocols) > 0 {
			protocols := make([]string, len(wsCopy.Options.Subprotocols))
			copy(protocols, wsCopy.Options.Subprotocols)
			wsCopy.Options.Subprotocols = protocols
		}
		if len(wsCopy.Steps) > 0 {
			steps := make([]restfile.WebSocketStep, len(wsCopy.Steps))
			copy(steps, wsCopy.Steps)
			wsCopy.Steps = steps
		}
		clone.WebSocket = &wsCopy
	}
	return &clone
}

func cloneRequestIf(req *restfile.Request, enabled bool) *restfile.Request {
	if !enabled {
		return nil
	}
	return cloneRequest(req)
}

func (m *Model) requestAtCursor(
	doc *restfile.Document,
	content string,
	cursorLine int,
) (*restfile.Request, bool) {
	if req, _ := requestAtLine(doc, cursorLine); req != nil {
		return req, false
	}
	if inline := buildInlineRequest(content, cursorLine); inline != nil {
		return inline, true
	}
	if doc != nil && len(doc.Requests) > 0 {
		last := doc.Requests[len(doc.Requests)-1]
		if last != nil && cursorLine > last.LineRange.End {
			return last, false
		}
	}
	return nil, false
}

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}

	cloned := make(http.Header, len(h))
	for k, values := range h {
		cloned[k] = append([]string(nil), values...)
	}
	return cloned
}

func renderRequestText(req *restfile.Request) string {
	if req == nil {
		return ""
	}

	builder := strings.Builder{}
	fmt.Fprintf(&builder, "%s %s\n", req.Method, req.URL)
	headerNames := make([]string, 0, len(req.Headers))
	for name := range req.Headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		for _, value := range req.Headers[name] {
			fmt.Fprintf(&builder, "%s: %s\n", name, value)
		}
	}

	builder.WriteString("\n")
	if req.WebSocket != nil {
		builder.WriteString(renderWebSocketSection(req.WebSocket))
	}
	if req.SSE != nil {
		builder.WriteString(renderSSESection(req.SSE))
	}
	if req.GRPC != nil {
		grpc := req.GRPC
		if grpc.FullMethod != "" {
			builder.WriteString("# @grpc ")
			builder.WriteString(strings.TrimPrefix(grpc.FullMethod, "/"))
			builder.WriteString("\n")
		}
		if grpc.DescriptorSet != "" {
			builder.WriteString("# @grpc-descriptor " + grpc.DescriptorSet + "\n")
		}
		if !grpc.UseReflection {
			builder.WriteString("# @grpc-reflection false\n")
		}
		if grpc.PlaintextSet {
			fmt.Fprintf(&builder, "# @grpc-plaintext %t\n", grpc.Plaintext)
		}
		if grpc.Authority != "" {
			builder.WriteString("# @grpc-authority " + grpc.Authority + "\n")
		}
		if len(grpc.Metadata) > 0 {
			for _, pair := range grpc.Metadata {
				fmt.Fprintf(&builder, "# @grpc-metadata %s: %s\n", pair.Key, pair.Value)
			}
		}
		builder.WriteString("\n")
		if strings.TrimSpace(grpc.Message) != "" {
			builder.WriteString(grpc.Message)
			if !strings.HasSuffix(grpc.Message, "\n") {
				builder.WriteString("\n")
			}
		} else if strings.TrimSpace(grpc.MessageFile) != "" {
			builder.WriteString("< " + strings.TrimSpace(grpc.MessageFile) + "\n")
		}
	} else if req.Body.GraphQL != nil {
		gql := req.Body.GraphQL
		builder.WriteString("# @graphql\n")
		if strings.TrimSpace(gql.OperationName) != "" {
			builder.WriteString("# @operation " + strings.TrimSpace(gql.OperationName) + "\n")
		}

		if strings.TrimSpace(gql.Query) != "" {
			builder.WriteString(gql.Query)
			if !strings.HasSuffix(gql.Query, "\n") {
				builder.WriteString("\n")
			}
		} else if strings.TrimSpace(gql.QueryFile) != "" {
			builder.WriteString("< " + strings.TrimSpace(gql.QueryFile) + "\n")
		}

		if strings.TrimSpace(gql.Variables) != "" || strings.TrimSpace(gql.VariablesFile) != "" {
			builder.WriteString("\n# @variables\n")
			if strings.TrimSpace(gql.Variables) != "" {
				builder.WriteString(gql.Variables)
				if !strings.HasSuffix(gql.Variables, "\n") {
					builder.WriteString("\n")
				}
			} else if strings.TrimSpace(gql.VariablesFile) != "" {
				builder.WriteString("< " + strings.TrimSpace(gql.VariablesFile) + "\n")
			}
		}
	} else if req.Body.FilePath != "" {
		builder.WriteString("< " + req.Body.FilePath + "\n")
	} else if strings.TrimSpace(req.Body.Text) != "" {
		builder.WriteString(req.Body.Text)
		if !strings.HasSuffix(req.Body.Text, "\n") {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func renderSSESection(sse *restfile.SSERequest) string {
	if sse == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if sse.Options.TotalTimeout > 0 {
		parts = append(parts, fmt.Sprintf("duration=%s", sse.Options.TotalTimeout))
	}
	if sse.Options.IdleTimeout > 0 {
		parts = append(parts, fmt.Sprintf("idle=%s", sse.Options.IdleTimeout))
	}
	if sse.Options.MaxEvents > 0 {
		parts = append(parts, fmt.Sprintf("max-events=%d", sse.Options.MaxEvents))
	}
	if sse.Options.MaxBytes > 0 {
		parts = append(parts, fmt.Sprintf("max-bytes=%d", sse.Options.MaxBytes))
	}
	line := "# @sse"
	if len(parts) > 0 {
		line += " " + strings.Join(parts, " ")
	}
	return line + "\n\n"
}

func renderWebSocketSection(ws *restfile.WebSocketRequest) string {
	if ws == nil {
		return ""
	}
	lines := []string{renderWebSocketDirectiveLine(ws.Options)}
	for _, step := range ws.Steps {
		if line := renderWebSocketStepLine(step); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n") + "\n\n"
}

func renderWebSocketDirectiveLine(opts restfile.WebSocketOptions) string {
	parts := make([]string, 0, 5)
	if opts.HandshakeTimeout > 0 {
		parts = append(parts, fmt.Sprintf("timeout=%s", opts.HandshakeTimeout))
	}
	if opts.IdleTimeout > 0 {
		parts = append(parts, fmt.Sprintf("idle=%s", opts.IdleTimeout))
	}
	if opts.MaxMessageBytes > 0 {
		parts = append(parts, fmt.Sprintf("max-message-bytes=%d", opts.MaxMessageBytes))
	}
	if len(opts.Subprotocols) > 0 {
		parts = append(parts, fmt.Sprintf("subprotocols=%s", strings.Join(opts.Subprotocols, ",")))
	}
	if opts.CompressionSet {
		parts = append(parts, fmt.Sprintf("compression=%t", opts.Compression))
	}
	line := "# @websocket"
	if len(parts) > 0 {
		line += " " + strings.Join(parts, " ")
	}
	return line
}

func renderWebSocketStepLine(step restfile.WebSocketStep) string {
	prefix := "# @ws "
	switch step.Type {
	case restfile.WebSocketStepSendText:
		return prefix + "send " + step.Value
	case restfile.WebSocketStepSendJSON:
		return prefix + "send-json " + step.Value
	case restfile.WebSocketStepSendBase64:
		return prefix + "send-base64 " + step.Value
	case restfile.WebSocketStepSendFile:
		if step.File == "" {
			return ""
		}
		return prefix + "send-file " + step.File
	case restfile.WebSocketStepPing:
		if strings.TrimSpace(step.Value) == "" {
			return prefix + "ping"
		}
		return prefix + "ping " + step.Value
	case restfile.WebSocketStepPong:
		if strings.TrimSpace(step.Value) == "" {
			return prefix + "pong"
		}
		return prefix + "pong " + step.Value
	case restfile.WebSocketStepWait:
		return prefix + "wait " + step.Duration.String()
	case restfile.WebSocketStepClose:
		code := step.Code
		if code == 0 {
			if strings.TrimSpace(step.Reason) == "" {
				return prefix + "close"
			}
			return prefix + "close " + step.Reason
		}
		reason := strings.TrimSpace(step.Reason)
		if reason == "" {
			return fmt.Sprintf("%sclose %d", prefix, code)
		}
		return fmt.Sprintf("%sclose %d %s", prefix, code, reason)
	default:
		return ""
	}
}

func buildInlineRequest(content string, lineNumber int) *restfile.Request {
	if lineNumber < 1 {
		return nil
	}

	lines := strings.Split(content, "\n")
	if req := inlineCurlRequest(lines, lineNumber); req != nil {
		return req
	}

	if lineNumber > len(lines) {
		return nil
	}
	return inlineRequestFromLine(lines[lineNumber-1], lineNumber)
}

func inlineCurlRequest(lines []string, lineNumber int) *restfile.Request {
	idx := lineNumber - 1
	if idx < 0 || idx >= len(lines) {
		return nil
	}

	start, end, command := extractCurlCommand(lines, idx)
	if command == "" {
		return nil
	}

	parsed, err := curl.ParseCommand(command)
	if err != nil {
		return nil
	}
	parsed.LineRange = restfile.LineRange{Start: start + 1, End: end + 1}
	parsed.OriginalText = strings.Join(lines[start:end+1], "\n")
	return parsed
}

func extractCurlCommand(lines []string, cursorIdx int) (start int, end int, command string) {
	return curl.ExtractCommand(lines, cursorIdx)
}

func inlineRequestFromLine(raw string, lineNumber int) *restfile.Request {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	method := "GET"
	url := ""

	fields := strings.Fields(trimmed)
	fields, ver := httpver.SplitToken(fields)
	if len(fields) == 1 {
		url = fields[0]
	} else if len(fields) >= 2 {
		candidate := strings.ToUpper(fields[0])
		if isInlineHTTPMethod(candidate) {
			method = candidate
			url = fields[1]
		}
	}

	if url == "" {
		url = strings.Join(fields, " ")
	}

	url = strings.Trim(url, "\"'")
	if !looksLikeHTTPRequestURL(url) {
		return nil
	}

	return &restfile.Request{
		Method: method,
		URL:    url,
		LineRange: restfile.LineRange{
			Start: lineNumber,
			End:   lineNumber,
		},
		OriginalText: raw,
		Settings:     httpver.SetIfMissing(nil, ver),
	}
}

func isInlineHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func looksLikeHTTPRequestURL(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "ws://") ||
		strings.HasPrefix(lower, "wss://")
}
