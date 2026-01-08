package ui

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"google.golang.org/grpc/codes"
)

type compareState struct {
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

type compareResult struct {
	Environment string
	Response    *httpclient.Response
	GRPC        *grpcclient.Response
	Err         error
	Tests       []scripts.TestResult
	ScriptErr   error
	Request     *restfile.Request
	RequestText string
	Canceled    bool
	Skipped     bool
	SkipReason  string
}

func (s *compareState) matches(req *restfile.Request) bool {
	return s != nil && s.current != nil && req == s.current
}

func (m *Model) resetCompareState() {
	if m.compareSnapshots != nil {
		for k := range m.compareSnapshots {
			delete(m.compareSnapshots, k)
		}
	}
	m.compareRowIndex = 0
	m.compareSelectedEnv = ""
	m.compareFocusedEnv = ""
}

// Clone the active request and reset compare bookkeeping so every environment
// run starts from the same baseline and the diff panes are ready as
// soon as responses arrive.
func (m *Model) startCompareRun(
	doc *restfile.Document,
	req *restfile.Request,
	spec *restfile.CompareSpec,
	options httpclient.Options,
) tea.Cmd {
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

	m.resetCompareState()

	state := &compareState{
		doc:       doc,
		base:      cloneRequest(req),
		options:   options,
		spec:      cloneCompareSpec(spec),
		envs:      append([]string(nil), spec.Environments...),
		originEnv: m.cfg.EnvironmentName,
		results:   make([]compareResult, 0, len(spec.Environments)),
	}
	title := strings.TrimSpace(m.statusRequestTitle(doc, req, ""))
	if title == "" {
		title = requestBaseTitle(req)
	}
	state.label = fmt.Sprintf("Compare %s", title)

	m.compareRun = state
	m.lastCompareResults = nil
	m.lastCompareSpec = nil
	m.compareBundle = nil
	spin := m.startSending()
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
	if cmd := m.executeCompareIteration(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if spin != nil {
		cmds = append(cmds, spin)
	}
	return batchCmds(cmds)
}

// Each iteration swaps in its own environment so resolvers see the right values
// without leaving the global selection changed afterward
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

// Snapshot each iteration immediately so the compare tab and diff panes can
// revisit the response even while the sweep continues.
func (m *Model) handleCompareResponse(msg responseMsg) tea.Cmd {
	state := m.compareRun
	if state == nil {
		return nil
	}

	currentReq := state.current
	currentEnv := state.currentEnv
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
		Tests:       append([]scripts.TestResult(nil), msg.tests...),
		ScriptErr:   msg.scriptErr,
		RequestText: state.requestText,
		Canceled:    canceled,
		Skipped:     msg.skipped,
		SkipReason:  msg.skipReason,
	}
	if currentReq != nil {
		result.Request = cloneRequest(currentReq)
	}

	var cmds []tea.Cmd
	if !canceled && msg.skipped {
		m.lastError = nil
		if cmd := m.consumeSkippedRequest(msg.skipReason); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else if !canceled && msg.err != nil {
		result.Err = msg.err
		m.lastError = msg.err
		if cmd := m.consumeRequestError(msg.err); cmd != nil {
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
		); cmd != nil {
			cmds = append(cmds, cmd)
		}
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

// Build the reusable bundle and history entry so panes and history reuse the
// same frozen data instead of rehydrating adhoc
func (m *Model) finalizeCompareRun(state *compareState) tea.Cmd {
	if state == nil {
		return nil
	}

	m.cfg.EnvironmentName = state.originEnv
	m.compareRun = nil
	m.lastCompareResults = state.results
	m.lastCompareSpec = cloneCompareSpec(state.spec)
	m.stopSending()
	m.stopStatusPulseIfIdle()

	if secondary := m.pane(responsePaneSecondary); secondary != nil {
		secondary.followLatest = true
		secondary.snapshot = m.responseLatest
		secondary.invalidateCaches()
	}

	if bundle := buildCompareBundle(state.results, state.spec); bundle != nil {
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

// Temporarily swap the active environment so callers can run work under a
// different scope without leaking the selection afterward.
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

// Keep the diff stable by pinning whichever response should act as the baseline
// before the next iteration updates the live pane.
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

func (m *Model) setCompareSnapshot(env string, snap *responseSnapshot) {
	trimmed := strings.TrimSpace(env)
	if trimmed == "" || snap == nil {
		return
	}
	if m.compareSnapshots == nil {
		m.compareSnapshots = make(map[string]*responseSnapshot)
	}

	key := strings.ToLower(trimmed)
	m.compareSnapshots[key] = snap
	if strings.TrimSpace(snap.environment) == "" {
		snap.environment = trimmed
	}
}

func (m *Model) compareSnapshot(env string) *responseSnapshot {
	if m.compareSnapshots == nil {
		return nil
	}

	key := strings.ToLower(strings.TrimSpace(env))
	if key == "" {
		return nil
	}
	return m.compareSnapshots[key]
}

type compareBundle struct {
	Baseline string
	Rows     []compareRow
}

type compareRow struct {
	Result   *compareResult
	Status   string
	Code     string
	Duration time.Duration
	Summary  string
}

// Condense raw iteration results into baseline-anchored rows so the compare tab
// and history list can render summaries without recomputing deltas.
func buildCompareBundle(results []compareResult, spec *restfile.CompareSpec) *compareBundle {
	if len(results) == 0 {
		return nil
	}

	var baselineName string
	if spec != nil {
		baselineName = strings.TrimSpace(spec.Baseline)
	}

	baseIdx := findBaselineIndex(results, baselineName)
	if baseIdx < 0 {
		baseIdx = 0
	}

	base := &results[baseIdx]
	bundle := &compareBundle{
		Baseline: base.Environment,
		Rows:     make([]compareRow, 0, len(results)),
	}
	for i := range results {
		res := &results[i]
		status, code := compareRowStatus(res)
		row := compareRow{
			Result:   res,
			Status:   status,
			Code:     code,
			Duration: compareRowDuration(res),
			Summary:  summarizeCompareDelta(base, res),
		}
		bundle.Rows = append(bundle.Rows, row)
	}
	return bundle
}

func findBaselineIndex(results []compareResult, baseline string) int {
	if strings.TrimSpace(baseline) == "" {
		return -1
	}
	for idx := range results {
		if strings.EqualFold(results[idx].Environment, baseline) {
			return idx
		}
	}
	return -1
}

func compareRowStatus(result *compareResult) (string, string) {
	switch {
	case result == nil:
		return "n/a", "-"
	case result.Canceled:
		return "canceled", "-"
	case result.Skipped:
		return "skipped", "-"
	case result.Err != nil:
		return "error", ""
	case result.Response != nil:
		return result.Response.Status, fmt.Sprintf("%d", result.Response.StatusCode)
	case result.GRPC != nil:
		return result.GRPC.StatusCode.String(), fmt.Sprintf("%d", result.GRPC.StatusCode)
	default:
		return "pending", "-"
	}
}

func compareRowDuration(result *compareResult) time.Duration {
	switch {
	case result == nil:
		return 0
	case result.Response != nil:
		return result.Response.Duration
	case result.GRPC != nil:
		return result.GRPC.Duration
	default:
		return 0
	}
}

func summarizeCompareDelta(base, target *compareResult) string {
	if target == nil {
		return "unavailable"
	}
	if base != nil && strings.EqualFold(base.Environment, target.Environment) {
		return "baseline"
	}
	if target.Skipped {
		reason := strings.TrimSpace(target.SkipReason)
		if reason == "" {
			return "skipped"
		}
		return fmt.Sprintf("skipped: %s", reason)
	}
	if base != nil && base.Skipped {
		return "baseline skipped"
	}
	if target.Err != nil {
		return fmt.Sprintf("error: %s", errdef.Message(target.Err))
	}
	if target.ScriptErr != nil {
		return fmt.Sprintf("tests error: %v", target.ScriptErr)
	}
	if fails := countTestFailures(target.Tests); fails > 0 {
		return fmt.Sprintf("%d test(s) failed", fails)
	}

	switch {
	case target.Response != nil && base != nil && base.Response != nil:
		return summarizeHTTPDelta(base.Response, target.Response)
	case target.GRPC != nil && base != nil && base.GRPC != nil:
		return summarizeGRPCDelta(base.GRPC, target.GRPC)
	default:
		return "unavailable"
	}
}

func countTestFailures(tests []scripts.TestResult) int {
	count := 0
	for _, t := range tests {
		if !t.Passed {
			count++
		}
	}
	return count
}

func summarizeHTTPDelta(base, target *httpclient.Response) string {
	if base == nil || target == nil {
		return "unavailable"
	}

	var deltas []string
	if target.StatusCode != base.StatusCode {
		deltas = append(deltas, "status")
	}
	if !bytes.Equal(target.Body, base.Body) {
		deltas = append(deltas, "body")
	}
	if !headersEqual(target.Headers, base.Headers) {
		deltas = append(deltas, "headers")
	}
	if len(deltas) == 0 {
		return "match"
	}
	return strings.Join(deltas, ", ") + " differ"
}

func summarizeGRPCDelta(base, target *grpcclient.Response) string {
	if base == nil || target == nil {
		return "unavailable"
	}

	var deltas []string
	if target.StatusCode != base.StatusCode {
		deltas = append(deltas, "status")
	}
	if strings.TrimSpace(target.StatusMessage) != strings.TrimSpace(base.StatusMessage) {
		deltas = append(deltas, "message")
	}
	if strings.TrimSpace(target.Message) != strings.TrimSpace(base.Message) {
		deltas = append(deltas, "body")
	}
	if len(deltas) == 0 {
		return "match"
	}
	return strings.Join(deltas, ", ") + " differ"
}

func headersEqual(a, b http.Header) bool {
	if len(a) != len(b) {
		return false
	}
	for key, values := range a {
		other, ok := b[key]
		if !ok {
			return false
		}
		sortedValues := append([]string(nil), values...)
		sortedOther := append([]string(nil), other...)
		sort.Strings(sortedValues)
		sort.Strings(sortedOther)
		if len(sortedValues) != len(sortedOther) {
			return false
		}
		for i := range sortedValues {
			if sortedValues[i] != sortedOther[i] {
				return false
			}
		}
	}
	return true
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
	for idx := range s.results {
		if !compareResultSuccess(&s.results[idx]) {
			return true
		}
	}
	return false
}

func compareResultSuccess(result *compareResult) bool {
	if result == nil {
		return false
	}
	if result.Canceled {
		return false
	}
	if result.Skipped {
		return false
	}
	if result.Err != nil || result.ScriptErr != nil {
		return false
	}
	if countTestFailures(result.Tests) > 0 {
		return false
	}
	if resp := result.Response; resp != nil {
		return resp.StatusCode < 400
	}
	if resp := result.GRPC; resp != nil {
		return resp.StatusCode == codes.OK
	}
	return false
}
