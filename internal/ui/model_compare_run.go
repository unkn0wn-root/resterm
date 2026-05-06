package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type compareState struct {
	id           string
	core         bool
	doc          *restfile.Document
	base         *restfile.Request
	options      httpclient.Options
	spec         *restfile.CompareSpec
	envs         []string
	index        int
	originEnv    string
	current      *restfile.Request
	currentEnv   string
	requestText  string
	results      []compareResult
	label        string
	canceled     bool
	cancelReason string
}

func (s *compareState) matches(req *restfile.Request) bool {
	return s != nil && s.current != nil && req == s.current
}

func compareStateFromPlan(
	pl *core.ComparePlan,
	opts httpclient.Options,
	useCore bool,
	originEnv string,
	label string,
) *compareState {
	if pl == nil {
		return nil
	}
	envs := append([]string(nil), pl.Spec.Environments...)
	return &compareState{
		id:        strings.TrimSpace(pl.Run.ID),
		core:      useCore,
		doc:       pl.Doc,
		base:      cloneRequest(pl.Request),
		options:   opts,
		spec:      core.CloneCompareSpec(&pl.Spec),
		envs:      envs,
		originEnv: originEnv,
		results:   make([]compareResult, 0, len(envs)),
		label:     label,
	}
}

func (m *Model) startCompareRun(
	doc *restfile.Document,
	req *restfile.Request,
	spec *restfile.CompareSpec,
	options httpclient.Options,
) tea.Cmd {
	if err := docErr(doc); err != nil {
		return batchCommands(m.restorePane(paneRegionResponse), m.failErr(err))
	}
	if spec == nil || len(spec.Environments) < 2 {
		m.setStatusMessage(
			statusMsg{level: statusWarn, text: "Compare requires at least two environments"},
		)
		return nil
	}
	if m.compareRun != nil {
		m.setStatusMessage(
			statusMsg{level: statusWarn, text: "Another compare run is already active"},
		)
		return nil
	}

	title := strings.TrimSpace(m.statusRequestTitle(doc, req, ""))
	if title == "" {
		title = requestBaseTitle(req)
	}
	label := fmt.Sprintf("Compare %s", title)
	env := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	pl, err := core.PrepareCompare(doc, req, spec, core.RunMeta{
		ID:  fmt.Sprintf("%d", time.Now().UnixNano()),
		Env: env,
	})
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}
	state := compareStateFromPlan(pl, options, true, m.cfg.EnvironmentName, label)
	if requestNeedsUIDrivenRun(req) {
		state.core = false
		return m.startCompareUIDrivenState(state)
	}
	return m.startCompareCoreRun(pl, state)
}

func (m *Model) beginCompareRun(state *compareState) []tea.Cmd {
	if state == nil {
		return nil
	}
	m.resetCompareState()
	m.compareBundle = nil
	m.compareRun = state
	m.statusPulseBase = state.label
	m.statusPulseFrame = -1

	var cmds []tea.Cmd
	if !m.responseSplit {
		targetOrientation := responseSplitHorizontal
		if m.mainSplitOrientation == mainSplitHorizontal {
			targetOrientation = responseSplitVertical
		}
		if cmd := m.enableResponseSplit(targetOrientation); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

func (m *Model) startCompareUIDrivenState(state *compareState) tea.Cmd {
	if state == nil {
		return nil
	}
	cmds := m.beginCompareRun(state)
	if cmd := m.executeCompareIteration(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return batchCmds(cmds)
}

func (m *Model) startCompareCoreRun(pl *core.ComparePlan, state *compareState) tea.Cmd {
	if pl == nil || state == nil {
		return nil
	}
	rq := m.requestSvc(state.options)
	if rq == nil {
		return nil
	}
	cmds := m.beginCompareRun(state)
	if len(state.envs) > 0 {
		state.currentEnv = state.envs[0]
		state.current = cloneRequest(state.base)
		state.requestText = renderRequestText(state.current)
		m.statusPulseBase = state.statusLine()
		m.setStatusMessage(statusMsg{text: state.statusLine(), level: statusInfo})
		if spin := m.startSending(); spin != nil {
			cmds = append(cmds, spin)
		}
		if pulse := m.startStatusPulse(); pulse != nil {
			cmds = append(cmds, pulse)
		}
	}
	ch := m.runMsgChan
	cmds = append(cmds, m.startRunWorker(state.id, func(ctx context.Context) error {
		return core.RunCompare(ctx, rq, runSink(ch), pl)
	}))
	return batchCmds(cmds)
}

func (m *Model) executeCompareIteration() tea.Cmd {
	state := m.compareRun
	if state == nil {
		return nil
	}
	if state.index >= len(state.envs) {
		return m.finalizeCompareRun(state)
	}

	env := state.envs[state.index]
	clone := cloneRequest(state.base)
	state.current = clone
	state.currentEnv = env
	state.requestText = renderRequestText(clone)

	spin := m.startSending()
	m.statusPulseBase = state.statusLine()
	m.setStatusMessage(statusMsg{text: state.statusLine(), level: statusInfo})

	runCmd := m.withEnvironment(env, func() tea.Cmd {
		return m.executeRequest(state.doc, clone, state.options, env, nil)
	})

	pulse := m.startStatusPulse()
	return batchCmds([]tea.Cmd{runCmd, pulse, spin})
}

func (m *Model) handleCompareUIDrivenResponse(msg responseMsg) tea.Cmd {
	state := m.compareRun
	if state == nil {
		return nil
	}
	canceled, cmd := m.consumeCompareRow(state, state.current, state.currentEnv, msg)
	var cmds []tea.Cmd
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	if canceled || state.index >= len(state.envs) {
		if cmd := m.finalizeCompareRun(state); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return batchCmds(cmds)
	}

	if cmd := m.executeCompareIteration(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return batchCmds(cmds)
}

func (m *Model) handleCompareRunEvt(evt core.Evt) tea.Cmd {
	st := m.compareRun
	if st == nil || !st.core || evt == nil {
		return nil
	}
	meta := core.MetaOf(evt)
	if st.id != "" && meta.Run.ID != "" && st.id != meta.Run.ID {
		return nil
	}
	switch v := evt.(type) {
	case core.CmpRowStart:
		return m.handleCompareRowStart(st, v)
	case core.CmpRowDone:
		return m.handleCompareRowDone(st, v)
	case core.RunDone:
		return m.handleCompareRunDone(st, v)
	}
	return nil
}

func (m *Model) handleCompareRowStart(st *compareState, evt core.CmpRowStart) tea.Cmd {
	if st == nil {
		return nil
	}
	st.index = evt.Row.Index
	st.currentEnv = compareEnvAt(st, evt.Row.Index, evt.Row.Env)
	st.current = cloneRequest(evt.Request)
	st.requestText = renderRequestText(st.current)
	m.statusPulseBase = st.statusLine()
	m.setStatusMessage(statusMsg{text: st.statusLine(), level: statusInfo})
	spin := m.startSending()
	pulse := m.startStatusPulse()
	return batchCmds([]tea.Cmd{spin, pulse})
}

func (m *Model) handleCompareRowDone(st *compareState, evt core.CmpRowDone) tea.Cmd {
	if st == nil {
		return nil
	}
	msg := m.responseMsgFromRunState(evt.Result, false)
	env := st.currentEnv
	if strings.TrimSpace(env) == "" {
		env = compareEnvAt(st, evt.Row.Index, evt.Row.Env)
	}
	canceled, cmd := m.consumeCompareRow(st, st.current, env, msg)
	if canceled || st.index >= len(st.envs) {
		return batchCmds([]tea.Cmd{cmd, m.finalizeCompareRun(st)})
	}
	return cmd
}

func (m *Model) handleCompareRunDone(st *compareState, evt core.RunDone) tea.Cmd {
	if st == nil {
		return nil
	}
	if evt.Canceled {
		st.canceled = true
	}
	m.sendCancel = nil
	m.stopSending()
	if m.compareRun != st {
		return nil
	}
	return m.finalizeCompareRun(st)
}

func compareEnvAt(st *compareState, i int, fallback string) string {
	if st != nil && i >= 0 && i < len(st.envs) {
		return st.envs[i]
	}
	return fallback
}

func (m *Model) consumeCompareRow(
	state *compareState,
	currentReq *restfile.Request,
	currentEnv string,
	msg responseMsg,
) (bool, tea.Cmd) {
	if state == nil {
		return false, nil
	}
	state.current = nil
	m.stopSending()

	canceled := state.canceled || isCanceled(msg.err)
	if canceled {
		state.canceled = true
		m.lastError = nil
		msg.err = nil
		if strings.TrimSpace(state.cancelReason) == "" {
			state.cancelReason = "Compare run canceled"
		}
	}

	if canceled {
		msg.skipped = false
	}
	result := compareResult{
		Environment: currentEnv,
		Stream:      cloneStreamInfo(msg.stream),
		Transcript:  append([]byte(nil), msg.transcript...),
		Tests:       append([]scripts.TestResult(nil), msg.tests...),
		ScriptErr:   msg.scriptErr,
		RequestText: state.requestText,
		Canceled:    canceled,
		Skipped:     msg.skipped,
		SkipReason:  msg.skipReason,
	}
	if currentReq == nil {
		currentReq = msg.executed
	}
	if currentReq != nil {
		result.Request = cloneRequest(currentReq)
	}
	if strings.TrimSpace(result.RequestText) == "" {
		result.RequestText = strings.TrimSpace(msg.requestText)
	}

	var cmds []tea.Cmd
	if !canceled && msg.skipped {
		m.lastError = nil
		if cmd := m.consumeSkippedRequest(msg.skipReason, msg.explain); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else if !canceled && msg.err != nil {
		result.Err = msg.err
		m.lastError = msg.err
		if cmd := m.consumeRequestError(msg.err, msg.explain); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else if !canceled && msg.grpc != nil {
		result.GRPC = msg.grpc
		m.lastError = nil
		if cmd := m.consumeGRPCResponse(
			msg.grpc,
			msg.tests,
			msg.scriptErr,
			msg.executed,
			msg.environment,
			msg.explain,
		); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else if !canceled && msg.response != nil {
		result.Response = msg.response
		m.lastError = nil
		if cmd := m.consumeHTTPResponse(
			msg.response,
			msg.tests,
			msg.scriptErr,
			msg.environment,
			msg.explain,
		); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else if !canceled && (msg.stream != nil || len(msg.transcript) > 0) {
		m.lastError = nil
		m.applyRunSnapshot(newStreamSnapshot(msg.stream, msg.transcript, msg.environment), nil, nil)
	} else {
		m.lastError = nil
	}

	state.results = append(state.results, result)
	m.storeCompareSnapshot(result.Environment)
	m.compareFocusedEnv = strings.TrimSpace(result.Environment)
	m.pinCompareReferencePane(state)
	state.index++

	level := statusInfo
	if canceled || !compareResultSuccess(&result) {
		level = statusWarn
	}
	m.setStatusMessage(statusMsg{text: state.statusLine(), level: level})
	return canceled, batchCmds(cmds)
}

func (m *Model) finalizeCompareRun(state *compareState) tea.Cmd {
	if state == nil {
		return nil
	}

	m.cfg.EnvironmentName = state.originEnv
	m.compareRun = nil
	m.stopSending()
	m.stopStatusPulseIfIdle()

	if secondary := m.pane(responsePaneSecondary); secondary != nil {
		secondary.followLatest = true
		secondary.snapshot = m.responseLatest
		secondary.invalidateCaches()
	}

	if bundle := buildCompareBundle(state.results, baselineFromSpec(state.spec)); bundle != nil {
		m.compareBundle = bundle
		if m.responseLatest != nil {
			m.responseLatest.compareBundle = bundle
		}
		if m.responsePrevious != nil {
			m.responsePrevious.compareBundle = bundle
		}
		for _, id := range m.visiblePaneIDs() {
			if pane := m.pane(id); pane != nil && pane.snapshot != nil {
				pane.snapshot.compareBundle = bundle
			}
		}
		for key, snap := range m.compareSnapshots {
			if snap == nil {
				delete(m.compareSnapshots, key)
				continue
			}
			snap.compareBundle = bundle
		}
		if len(bundle.Rows) > 0 {
			m.compareSelectedEnv = strings.TrimSpace(bundle.Rows[0].Result.Environment)
			m.compareFocusedEnv = m.compareSelectedEnv
			m.compareRowIndex = compareRowIndexForEnv(bundle, m.compareSelectedEnv)
		} else {
			m.compareRowIndex = 0
		}
		m.invalidateCompareTabCaches()
	}

	label := fmt.Sprintf("%s complete", state.label)
	level := statusSuccess
	if state.canceled {
		label = fmt.Sprintf("%s canceled", state.label)
		level = statusWarn
	} else if state.hasFailures() {
		level = statusWarn
	}
	m.setStatusMessage(
		statusMsg{text: fmt.Sprintf("%s | %s", label, state.progressSummary()), level: level},
	)
	m.recordCompareHistory(state)
	return nil
}

func (m *Model) withEnvironment(env string, fn func() tea.Cmd) tea.Cmd {
	prev := m.cfg.EnvironmentName
	m.cfg.EnvironmentName = env
	if fn == nil {
		m.cfg.EnvironmentName = prev
		return nil
	}
	defer func() {
		m.cfg.EnvironmentName = prev
	}()
	return fn()
}

func (m *Model) pinCompareReferencePane(state *compareState) {
	if state == nil || !m.responseSplit {
		return
	}

	secondary := m.pane(responsePaneSecondary)
	if secondary == nil {
		return
	}

	var snapshot *responseSnapshot
	if state.index == 0 {
		snapshot = m.responseLatest
	} else {
		snapshot = m.responsePrevious
		if snapshot == nil {
			snapshot = m.responseLatest
		}
	}
	if snapshot == nil {
		return
	}
	secondary.snapshot = snapshot
	secondary.followLatest = false
	secondary.invalidateCaches()
}

func (m *Model) storeCompareSnapshot(env string) {
	snap := m.responseLatest
	if snap == nil {
		return
	}
	m.setCompareSnapshot(env, snap)
}

func (m *Model) recordCompareHistory(state *compareState) {
	hs := m.historyStore()
	if hs == nil || state == nil || len(state.results) == 0 {
		return
	}

	baseReq := state.base
	if baseReq == nil {
		for _, res := range state.results {
			if res.Request != nil {
				baseReq = res.Request
				break
			}
		}
	}
	if baseReq == nil {
		return
	}

	entry := history.Entry{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		ExecutedAt:  time.Now(),
		RequestName: requestIdentifier(baseReq),
		FilePath:    m.historyFilePath(),
		Method:      restfile.HistoryMethodCompare,
		URL:         baseReq.URL,
		Description: strings.TrimSpace(baseReq.Metadata.Description),
		Tags:        normalizedTags(baseReq.Metadata.Tags),
		Status:      state.progressSummary(),
		RequestText: renderRequestText(baseReq),
		Compare:     &history.CompareEntry{},
	}
	if state.canceled {
		status := fmt.Sprintf("canceled after %d/%d", len(state.results), len(state.envs))
		if strings.TrimSpace(state.label) != "" {
			status = fmt.Sprintf("%s | %s", strings.TrimSpace(state.label), status)
		}
		entry.Status = status
	}
	if state.spec != nil {
		entry.Compare.Baseline = state.spec.Baseline
	}

	var totalDur time.Duration
	results := make([]history.CompareResult, 0, len(state.results))
	for _, res := range state.results {
		item := m.buildCompareHistoryResult(res)
		if item.Duration > 0 {
			totalDur += item.Duration
		}
		results = append(results, item)
	}
	entry.Compare.Results = results
	entry.Duration = totalDur
	if entry.Status == "" {
		entry.Status = fmt.Sprintf("Compare %d env", len(results))
	}

	if err := hs.Append(entry); err != nil {
		m.setStatusMessage(
			statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn},
		)
		return
	}
	m.historySelectedID = entry.ID
	m.syncHistory()
}

func (m *Model) buildCompareHistoryResult(result compareResult) history.CompareResult {
	env := strings.TrimSpace(result.Environment)
	status, _ := compareRowStatus(&result)

	entry := history.CompareResult{
		Environment: env,
		Status:      status,
		Duration:    compareRowDuration(&result),
		RequestText: strings.TrimSpace(result.RequestText),
	}

	req := result.Request
	if req != nil && strings.TrimSpace(entry.RequestText) == "" {
		entry.RequestText = renderRequestText(req)
	}
	if req != nil {
		secrets := m.secretValuesForEnvironment(env, req)
		maskHeaders := !req.Metadata.AllowSensitiveHeaders
		entry.RequestText = redactHistoryText(entry.RequestText, secrets, maskHeaders)
	}

	switch {
	case result.Canceled:
		entry.Error = "canceled"
		entry.BodySnippet = entry.Error
	case result.Skipped:
		reason := strings.TrimSpace(result.SkipReason)
		if reason == "" {
			reason = "skipped"
		}
		entry.Error = reason
		entry.BodySnippet = reason
	case result.Err != nil:
		entry.Error = result.Err.Error()
		entry.BodySnippet = entry.Error
	case result.Response != nil:
		entry.BodySnippet = buildCompareHTTPSnippet(result.Response, req, env, m)
		entry.StatusCode = result.Response.StatusCode
	case result.GRPC != nil:
		entry.BodySnippet = buildCompareGRPCSnippet(result.GRPC, req, env, m)
		entry.StatusCode = int(result.GRPC.StatusCode)
	case result.Stream != nil || len(result.Transcript) > 0:
		entry.BodySnippet = streamSummaryText(result.Stream)
	default:
		entry.BodySnippet = "No response captured"
	}

	const limit = 2000
	if len(entry.BodySnippet) > limit {
		entry.BodySnippet = entry.BodySnippet[:limit]
	}
	return entry
}

func buildCompareHTTPSnippet(
	resp *httpclient.Response,
	req *restfile.Request,
	env string,
	m *Model,
) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	secrets := m.secretValuesForEnvironment(env, req)
	return redactHistoryText(string(resp.Body), secrets, false)
}

func buildCompareGRPCSnippet(
	resp *grpcclient.Response,
	req *restfile.Request,
	env string,
	m *Model,
) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	secrets := m.secretValuesForEnvironment(env, req)
	return redactHistoryText(resp.Message, secrets, false)
}

func baselineFromSpec(spec *restfile.CompareSpec) string {
	if spec == nil {
		return ""
	}
	return strings.TrimSpace(spec.Baseline)
}

func (s *compareState) progressSummary() string {
	if s == nil || len(s.envs) == 0 {
		return ""
	}

	parts := make([]string, len(s.envs))
	for idx, env := range s.envs {
		label := env
		if s.spec != nil && strings.EqualFold(env, s.spec.Baseline) {
			label += "*"
		}
		switch {
		case idx < len(s.results):
			res := &s.results[idx]
			switch {
			case res.Canceled:
				label += "!"
			case compareResultSuccess(res):
				label += "✓"
			default:
				label += "✗"
			}
		case idx == s.index && s.current != nil:
			label += "…"
		default:
			label += "?"
		}
		parts[idx] = label
	}
	return strings.Join(parts, " ")
}

func (s *compareState) statusLine() string {
	if s == nil {
		return ""
	}
	summary := strings.TrimSpace(s.progressSummary())
	if summary == "" {
		return s.label
	}
	return fmt.Sprintf("%s | %s", s.label, summary)
}

func (s *compareState) hasFailures() bool {
	if s == nil {
		return false
	}
	for i := range s.results {
		if !compareResultSuccess(&s.results[i]) {
			return true
		}
	}
	return false
}
