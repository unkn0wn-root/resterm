package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type compareState struct {
	id           string
	base         *restfile.Request
	options      httpclient.Options
	targets      []vars.Target
	group        string
	baseline     string
	index        int
	current      *restfile.Request
	currentEnv   string
	requestText  string
	results      []compareResult
	label        string
	canceled     bool
	cancelReason string
	latGen       int
}

func compareStateFromPlan(
	pl *core.ComparePlan,
	opts httpclient.Options,
	label string,
) *compareState {
	if pl == nil {
		return nil
	}
	return &compareState{
		id:       strings.TrimSpace(pl.Run.ID),
		base:     pl.Request.Clone(),
		options:  opts,
		targets:  slices.Clone(pl.Targets),
		group:    pl.Group,
		baseline: pl.Baseline,
		results:  make([]compareResult, 0, len(pl.Targets)),
		label:    label,
	}
}

// envAt is the environment label of target i, empty when i is out of range.
func (s *compareState) envAt(i int) string {
	if s == nil || i < 0 || i >= len(s.targets) {
		return ""
	}
	return s.targets[i].Env.Label()
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

	title := strings.TrimSpace(m.statusRequestTitle(doc, req))
	if title == "" {
		title = requestBaseTitle(req)
	}
	label := fmt.Sprintf("Compare %s", title)
	spec = core.NormalizeCompareSpec(spec)
	env := m.env
	targets, err := m.cfg.Catalog.CompareTargets(
		env.Selection(),
		spec.Group,
		spec.Baseline,
		spec.Environments,
	)
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}
	pl, err := core.PrepareCompare(core.CompareInput{
		Doc:      doc,
		Request:  req,
		Targets:  targets,
		Group:    spec.Group,
		Baseline: spec.Baseline,
		Run: core.RunMeta{
			ID:  fmt.Sprintf("%d", time.Now().UnixNano()),
			Env: env,
		},
	})
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}
	state := compareStateFromPlan(pl, options, label)
	return m.startCompareCoreRun(pl, state)
}

func (m *Model) beginCompareRun(state *compareState) []tea.Cmd {
	if state == nil {
		return nil
	}
	m.resetCompareState()
	m.compareBundle = nil
	state.latGen = m.latencySeries.generation()
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

func (m *Model) startCompareCoreRun(pl *core.ComparePlan, state *compareState) tea.Cmd {
	if pl == nil || state == nil {
		return nil
	}
	rq := m.runRequestSvc(state.options)
	if rq == nil {
		return nil
	}
	cmds := m.beginCompareRun(state)
	if len(state.targets) > 0 {
		state.currentEnv = state.envAt(0)
		state.current = state.base.Clone()
		state.requestText = rqeng.RenderRequestText(state.current)
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

func (m *Model) handleCompareRunEvt(evt core.Evt) tea.Cmd {
	st := m.compareRun
	if st == nil || evt == nil {
		return nil
	}
	meta := core.MetaOf(evt)
	if runIDMismatch(st.id, meta.Run.ID) {
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
	st.current = evt.Request.Clone()
	st.requestText = rqeng.RenderRequestText(st.current)
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
	msg.latGen = st.latGen
	m.recordResponseLatency(msg)
	env := st.currentEnv
	if strings.TrimSpace(env) == "" {
		env = compareEnvAt(st, evt.Row.Index, evt.Row.Env)
	}
	canceled, cmd := m.consumeCompareRow(st, st.current, env, msg)
	if canceled || st.index >= len(st.targets) {
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
	if env := st.envAt(i); env != "" {
		return env
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
		Selection:   msg.selection,
		Stream:      cloneStreamInfo(msg.stream),
		Transcript:  append([]byte(nil), msg.transcript...),
		Tests:       append([]scripts.TestResult(nil), msg.tests...),
		ScriptErr:   msg.scriptErr,
		RequestText: state.requestText,
		Canceled:    canceled,
		Skipped:     msg.skipped,
		SkipReason:  msg.skipReason,
	}
	if state.group != "" {
		result.Profile, _ = msg.selection.Profile(state.group)
	}
	if currentReq == nil {
		currentReq = msg.executed
	}
	if currentReq != nil {
		result.Request = currentReq.Clone()
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

	m.compareRun = nil
	m.stopSending()
	m.stopStatusPulseIfIdle()

	if secondary := m.pane(responsePaneSecondary); secondary != nil {
		secondary.followLatest = true
		secondary.snapshot = m.responseLatest
		secondary.invalidateCaches()
	}

	if bundle := buildCompareBundle(state.results, state.baseline); bundle != nil {
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
		RequestText: rqeng.RenderRequestText(baseReq),
		Compare:     &history.CompareEntry{},
	}
	entry.Environment = m.env.Label()
	entry.EnvironmentSelection = history.EnvironmentSelection(m.env.Selection().Groups())
	if state.canceled {
		status := fmt.Sprintf("canceled after %d/%d", len(state.results), len(state.targets))
		if strings.TrimSpace(state.label) != "" {
			status = fmt.Sprintf("%s | %s", strings.TrimSpace(state.label), status)
		}
		entry.Status = status
	}
	entry.Compare.Baseline = state.baseline
	entry.Compare.Group = state.group

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
		Environment:          env,
		Profile:              result.Profile,
		EnvironmentSelection: history.EnvironmentSelection(result.Selection.Groups()),
		Status:               status,
		Duration:             compareRowDuration(&result),
		RequestText:          strings.TrimSpace(result.RequestText),
	}

	req := result.Request
	if req != nil && strings.TrimSpace(entry.RequestText) == "" {
		entry.RequestText = rqeng.RenderRequestText(req)
	}
	if req != nil {
		secrets := m.secretValuesForSelection(result.Selection, req)
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
		entry.BodySnippet = m.compareHTTPSnippet(result.Response, req, result.Selection)
		entry.StatusCode = result.Response.StatusCode
	case result.GRPC != nil:
		entry.BodySnippet = m.compareGRPCSnippet(result.GRPC, req, result.Selection)
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

func (m *Model) compareHTTPSnippet(
	resp *httpclient.Response,
	req *restfile.Request,
	sel vars.Selection,
) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	return redactHistoryText(string(resp.Body), m.secretValuesForSelection(sel, req), false)
}

func (m *Model) compareGRPCSnippet(
	resp *grpcclient.Response,
	req *restfile.Request,
	sel vars.Selection,
) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	return redactHistoryText(resp.Message, m.secretValuesForSelection(sel, req), false)
}

func (s *compareState) progressSummary() string {
	if s == nil || len(s.targets) == 0 {
		return ""
	}

	parts := make([]string, len(s.targets))
	for idx, target := range s.targets {
		label := target.Env.Label()
		if s.baseline != "" && strings.EqualFold(target.Name(), s.baseline) {
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
