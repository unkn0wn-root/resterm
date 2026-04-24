package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type profileState struct {
	id            string
	core          bool
	base          *restfile.Request
	doc           *restfile.Document
	options       httpclient.Options
	spec          restfile.ProfileSpec
	total         int
	warmup        int
	delay         time.Duration
	index         int
	successes     []time.Duration
	failures      []profileFailure
	current       *restfile.Request
	messageBase   string
	start         time.Time
	measuredStart time.Time
	measuredEnd   time.Time
	canceled      bool
	cancelReason  string
	skipped       bool
	skipReason    string
}

type profileFailure struct {
	Iteration  int
	Warmup     bool
	Reason     string
	Status     string
	StatusCode int
	Duration   time.Duration
}

func (s *profileState) matches(req *restfile.Request) bool {
	return s != nil && s.current != nil && req != nil && s.current == req
}

func (s *profileState) successCount() int {
	return len(s.successes)
}

func (s *profileState) failureCount() int {
	n := 0
	for _, failure := range s.failures {
		if !failure.Warmup {
			n++
		}
	}
	return n
}

func profileStateFromPlan(
	pl *core.ProfilePlan,
	opts httpclient.Options,
	useCore bool,
	msgBase string,
) *profileState {
	if pl == nil {
		return nil
	}
	return &profileState{
		id:          strings.TrimSpace(pl.Run.ID),
		core:        useCore,
		base:        cloneRequest(pl.Request),
		doc:         pl.Doc,
		options:     opts,
		spec:        pl.Spec,
		total:       pl.Total,
		warmup:      pl.Spec.Warmup,
		delay:       pl.Spec.Delay,
		successes:   make([]time.Duration, 0, pl.Spec.Count),
		failures:    make([]profileFailure, 0, pl.Spec.Count/2+1),
		messageBase: msgBase,
		start:       time.Now(),
	}
}

func (m *Model) startProfileRun(
	doc *restfile.Document,
	req *restfile.Request,
	options httpclient.Options,
) tea.Cmd {
	if req == nil {
		return nil
	}
	if req.GRPC != nil {
		m.setStatusMessage(
			statusMsg{text: "Profiling is not supported for gRPC requests", level: statusWarn},
		)
		return m.executeRequest(doc, req, options, "", nil)
	}
	title := strings.TrimSpace(m.statusRequestTitle(doc, req, ""))
	if title == "" {
		title = requestBaseTitle(req)
	}
	msgBase := fmt.Sprintf("Profiling %s", title)
	env := vars.SelectEnv(m.cfg.EnvironmentSet, "", m.cfg.EnvironmentName)
	pl, err := core.PrepareProfile(doc, req, core.RunMeta{
		ID:  fmt.Sprintf("%d", time.Now().UnixNano()),
		Env: env,
	})
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}
	state := profileStateFromPlan(pl, options, true, msgBase)
	if requestNeedsUIDrivenRun(req) {
		state.core = false
		return m.startProfileUIDrivenState(state)
	}
	return m.startProfileCoreRun(pl, state)
}

func (m *Model) startProfileUIDrivenState(state *profileState) tea.Cmd {
	if state == nil {
		return nil
	}
	m.profileRun = state
	spin := m.startSending()
	m.statusPulseBase = strings.TrimSpace(profileProgressLabel(state))
	m.statusPulseFrame = 0
	execCmd := m.executeProfileIteration()
	pulse := m.startStatusPulse()
	return batchCmds([]tea.Cmd{execCmd, pulse, spin})
}

func (m *Model) startProfileCoreRun(pl *core.ProfilePlan, state *profileState) tea.Cmd {
	if pl == nil || state == nil {
		return nil
	}
	rq := m.requestSvc(state.options)
	if rq == nil {
		return nil
	}
	m.profileRun = state
	m.statusPulseFrame = 0
	if state.total > 0 {
		state.current = cloneRequest(state.base)
		m.currentRequest = state.current
		if state.index >= state.warmup && state.measuredStart.IsZero() {
			state.measuredStart = time.Now()
		}
	}
	m.statusPulseBase = strings.TrimSpace(profileProgressLabel(state))
	m.showProfileProgress(state)
	spin := m.startSending()
	pulse := m.startStatusPulse()
	ch := m.runMsgChan
	worker := m.startRunWorker(state.id, func(ctx context.Context) error {
		return core.RunProfile(ctx, rq, runSink(ch), pl)
	})
	return batchCmds([]tea.Cmd{worker, pulse, spin})
}

func (m *Model) executeProfileIteration() tea.Cmd {
	state := m.profileRun
	if state == nil || state.canceled || state.index >= state.total {
		return nil
	}

	iterationReq := cloneRequest(state.base)
	state.current = iterationReq
	m.currentRequest = iterationReq

	if state.index >= state.warmup && state.measuredStart.IsZero() {
		state.measuredStart = time.Now()
	}

	progressText := profileProgressLabel(state)
	m.statusPulseBase = progressText
	m.showProfileProgress(state)

	return m.executeRequest(state.doc, iterationReq, state.options, "", nil)
}

func (m *Model) handleProfileUIDrivenResponse(msg responseMsg) tea.Cmd {
	state := m.profileRun
	if state == nil {
		return nil
	}
	return m.consumeProfileResult(state, msg, time.Time{})
}

func (m *Model) handleProfileRunEvt(evt core.Evt) tea.Cmd {
	state := m.profileRun
	if state == nil || !state.core || evt == nil {
		return nil
	}
	meta := core.MetaOf(evt)
	if state.id != "" && meta.Run.ID != "" && state.id != meta.Run.ID {
		return nil
	}
	switch v := evt.(type) {
	case core.RunStart:
		state.start = v.Meta.At
	case core.ProIterStart:
		return m.handleProfileIterStart(state, v)
	case core.ProIterDone:
		return m.handleProfileIterDone(state, v)
	case core.RunDone:
		return m.handleProfileRunDone(state, v)
	}
	return nil
}

func (m *Model) handleProfileIterStart(state *profileState, evt core.ProIterStart) tea.Cmd {
	if state == nil {
		return nil
	}
	state.current = cloneRequest(evt.Request)
	m.currentRequest = state.current
	if !evt.Iter.Warmup && state.measuredStart.IsZero() {
		state.measuredStart = evt.Meta.At
	}
	m.statusPulseBase = strings.TrimSpace(profileProgressLabel(state))
	m.showProfileProgress(state)
	spin := m.startSending()
	pulse := m.startStatusPulse()
	return batchCmds([]tea.Cmd{spin, pulse})
}

func (m *Model) handleProfileIterDone(state *profileState, evt core.ProIterDone) tea.Cmd {
	if state == nil {
		return nil
	}
	msg := m.responseMsgFromRunState(evt.Result, false)
	m.recordResponseLatency(msg)
	return m.consumeProfileResult(state, msg, evt.Meta.At)
}

func (m *Model) handleProfileRunDone(state *profileState, evt core.RunDone) tea.Cmd {
	if state == nil {
		return nil
	}
	m.sendCancel = nil
	if evt.Canceled {
		state.canceled = true
		if strings.TrimSpace(state.cancelReason) == "" {
			state.cancelReason = "Profiling canceled"
		}
	}
	if m.profileRun != state {
		return nil
	}
	if state.current == nil && (state.canceled || state.skipped || state.index >= state.total) {
		return m.finalizeProfileRun(responseMsg{}, state)
	}
	return nil
}

func (m *Model) consumeProfileResult(
	state *profileState,
	msg responseMsg,
	at time.Time,
) tea.Cmd {
	if state == nil {
		return nil
	}

	hadCurrent := state.current != nil
	canceled := state.canceled || isCanceled(msg.err)

	m.lastError = nil
	m.testResults = msg.tests
	m.scriptError = msg.scriptErr
	if msg.err != nil && !canceled {
		m.lastError = msg.err
		m.lastResponse = nil
		m.lastGRPC = nil
	}

	if canceled {
		state.canceled = true
		m.lastError = nil
		m.lastResponse = nil
		m.lastGRPC = nil
		msg.err = nil
		msg.response = nil
	}
	state.current = nil
	if canceled {
		if state.cancelReason == "" {
			state.cancelReason = "Profiling canceled"
		}
		if hadCurrent && state.index < state.total {
			state.index++
		}
		m.stopSending()
		return m.finalizeProfileRun(msg, state)
	}

	if msg.skipped {
		state.skipped = true
		if strings.TrimSpace(state.skipReason) == "" {
			state.skipReason = msg.skipReason
		}
		m.lastError = nil
		m.lastResponse = nil
		m.lastGRPC = nil
		m.stopSending()
		return m.finalizeProfileRun(msg, state)
	}

	duration := time.Duration(0)
	if msg.response != nil {
		duration = msg.response.Duration
	}

	success, reason := evaluateProfileOutcome(msg)
	warmup := state.index < state.warmup

	if !warmup {
		now := at
		if now.IsZero() {
			now = time.Now()
		}
		if state.measuredStart.IsZero() {
			state.measuredStart = now
		}
		state.measuredEnd = now
	}

	if success {
		if !warmup {
			state.successes = append(state.successes, duration)
		}
	} else {
		failure := profileFailure{
			Iteration: state.index + 1,
			Warmup:    warmup,
			Reason:    reason,
			Duration:  duration,
		}
		if msg.response != nil {
			failure.Status = msg.response.Status
			failure.StatusCode = msg.response.StatusCode
		}
		state.failures = append(state.failures, failure)
	}

	state.index++
	if state.index < state.total {
		progressText := profileProgressLabel(state)
		m.statusPulseBase = progressText
		m.setStatusMessage(statusMsg{text: progressText, level: statusInfo})
		if state.core {
			return nil
		}
		spin := m.startSending()
		if state.delay > 0 {
			next := tea.Tick(
				state.delay,
				func(time.Time) tea.Msg { return profileNextIterationMsg{} },
			)
			pulse := m.startStatusPulse()
			return batchCmds([]tea.Cmd{next, pulse, spin})
		}
		exec := m.executeProfileIteration()
		pulse := m.startStatusPulse()
		return batchCmds([]tea.Cmd{exec, pulse, spin})
	}

	return m.finalizeProfileRun(msg, state)
}

func evaluateProfileOutcome(msg responseMsg) (bool, string) {
	if msg.skipped {
		reason := strings.TrimSpace(msg.skipReason)
		if reason == "" {
			reason = "request skipped"
		}
		return false, reason
	}
	if msg.err != nil {
		return false, msg.err.Error()
	}
	if msg.response != nil && msg.response.StatusCode >= 400 {
		return false, fmt.Sprintf("HTTP %s", msg.response.Status)
	}
	if msg.scriptErr != nil {
		return false, msg.scriptErr.Error()
	}
	for _, test := range msg.tests {
		if !test.Passed {
			reason := test.Name
			if strings.TrimSpace(test.Message) != "" {
				reason = fmt.Sprintf("%s – %s", test.Name, test.Message)
			}
			return false, fmt.Sprintf("Test failed: %s", reason)
		}
	}
	if msg.response == nil {
		return false, "no response"
	}
	return true, ""
}

func profileProgressLabel(state *profileState) string {
	if state == nil {
		return ""
	}
	if state.index < state.warmup {
		return fmt.Sprintf("%s warmup %d/%d", state.messageBase, state.index+1, state.warmup)
	}

	measured := state.index - state.warmup + 1
	if measured > state.spec.Count {
		measured = state.spec.Count
	}
	return fmt.Sprintf("%s run %d/%d", state.messageBase, measured, state.spec.Count)
}

func (m *Model) finalizeProfileRun(msg responseMsg, state *profileState) tea.Cmd {
	m.profileRun = nil
	m.stopSending()
	m.stopStatusPulseIfIdle()

	report := ""
	var stats analysis.LatencyStats
	var statsPtr *analysis.LatencyStats
	if len(state.successes) > 0 {
		stats = analysis.ComputeLatencyStats(
			state.successes,
			analysis.DefaultProfilePercentiles(),
			10,
		)
		statsPtr = &stats
		report = m.buildProfileReport(state, stats)
	} else {
		report = m.buildProfileReport(state, stats)
	}

	var cmds []tea.Cmd
	canceled := state != nil && state.canceled
	if msg.err != nil && (!canceled || !isCanceled(msg.err)) {
		if cmd := m.consumeRequestError(msg.err, msg.explain); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else if msg.response != nil {
		if cmd := m.consumeHTTPResponse(
			msg.response,
			msg.tests,
			msg.scriptErr,
			msg.environment,
			msg.explain,
		); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else {
		summary := buildProfileSummary(state)
		body := report
		if canceled && strings.TrimSpace(summary) != "" {
			body = summary
		}
		snapshot := &responseSnapshot{
			pretty:         body,
			raw:            body,
			headers:        body,
			requestHeaders: body,
			stats:          report,
			statsColorize:  true,
			statsKind:      statsReportKindProfile,
			profileStats:   statsPtr,
			statsColored:   "",
			ready:          true,
		}
		m.setResponseSnapshotContent(snapshot)
	}

	if m.responseLatest != nil {
		m.responseLatest.stats = report
		m.responseLatest.statsColored = ""
		m.responseLatest.statsColorize = true
		m.responseLatest.statsKind = statsReportKindProfile
		m.responseLatest.profileStats = statsPtr

		if canceled {
			summary := buildProfileSummary(state)
			body := summary
			m.responseLatest.pretty = body
			m.responseLatest.raw = body
			m.responseLatest.headers = body
			m.responseLatest.requestHeaders = body
			m.setResponseSnapshotContent(m.responseLatest)
		}

		cmds = append(cmds, m.activateProfileStatsTab(m.responseLatest))
	}

	m.recordProfileHistory(state, stats, msg, report)

	summary := buildProfileSummary(state)
	level := statusInfo
	if canceled || (state != nil && state.skipped) {
		level = statusWarn
	}
	m.setStatusMessage(statusMsg{text: summary, level: level})

	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane != nil && pane.snapshot == m.responseLatest {
			pane.invalidateCaches()
		}
	}
	if cmd := m.syncResponsePanes(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return batchCmds(cmds)
}

func buildProfileSummary(state *profileState) string {
	if state == nil {
		return "Profiling complete"
	}
	if state.skipped {
		reason := strings.TrimSpace(state.skipReason)
		if reason == "" {
			reason = "condition evaluated to false"
		}
		return fmt.Sprintf("Profiling skipped: %s", reason)
	}

	mt := profileMetricsFromState(state)
	if state.canceled {
		planned := state.total
		if planned == 0 {
			planned = mt.total
		}
		measuredPlanned := state.spec.Count
		if measuredPlanned == 0 {
			measuredPlanned = mt.measured
		}
		return fmt.Sprintf(
			"Profiling canceled after %d/%d runs (%d/%d measured)",
			mt.total,
			planned,
			mt.measured,
			measuredPlanned,
		)
	}

	return fmt.Sprintf(
		"Profiling complete: %d/%d success (%d failure, %d warmup)",
		mt.success,
		state.spec.Count,
		mt.failures,
		mt.warmup,
	)
}

func (m *Model) buildProfileReport(state *profileState, stats analysis.LatencyStats) string {
	mt := profileMetricsFromState(state)
	var b strings.Builder

	writeProfileHeader(&b, state.messageBase)
	writeProfileSummary(&b, state, mt)
	writeLatencySection(&b, stats)
	writeDistributionSection(&b, stats)
	writeFailureSection(&b, state)

	return strings.TrimRight(b.String(), "\n")
}

func renderLatencyTable(stats analysis.LatencyStats) string {
	labels := []string{"min", "p50", "p90", "p95", "p99", "max"}
	values := []string{
		formatDurationShort(stats.Min),
		formatDurationShort(percentileValue(stats, 50)),
		formatDurationShort(percentileValue(stats, 90)),
		formatDurationShort(percentileValue(stats, 95)),
		formatDurationShort(percentileValue(stats, 99)),
		formatDurationShort(stats.Max),
	}

	widths := make([]int, len(labels))
	for i := range labels {
		widths[i] = len(labels[i])
		if w := len(values[i]); w > widths[i] {
			widths[i] = w
		}
	}

	var b strings.Builder
	b.WriteString(formatLatencyRow(labels, widths))
	b.WriteString("\n")
	b.WriteString(formatLatencyRow(values, widths))
	b.WriteString("\n")
	b.WriteString("  mean: ")
	b.WriteString(formatDurationShort(stats.Mean))
	b.WriteString(" | median: ")
	b.WriteString(formatDurationShort(stats.Median))
	b.WriteString(" | stddev: ")
	b.WriteString(formatDurationShort(stats.StdDev))
	b.WriteString("\n")
	return b.String()
}

func formatLatencyRow(items []string, widths []int) string {
	var b strings.Builder
	b.WriteString("  ")
	for i, item := range items {
		if i > 0 {
			b.WriteString("  ")
		}
		fmt.Fprintf(&b, "%-*s", widths[i], item)
	}
	return b.String()
}

func percentileValue(stats analysis.LatencyStats, percentile int) time.Duration {
	if stats.Percentiles != nil {
		if v, ok := stats.Percentiles[percentile]; ok {
			return v
		}
	}
	return stats.Median
}

type profileMetrics struct {
	success          int
	failures         int
	warmup           int
	total            int
	measured         int
	measuredElapsed  time.Duration
	totalElapsed     time.Duration
	measuredDuration time.Duration
	elapsed          time.Duration
	throughput       string
	throughputNoWait string
	successRate      string
	delay            time.Duration
}

func profileMetricsFromState(state *profileState) profileMetrics {
	if state == nil {
		return profileMetrics{}
	}

	success := state.successCount()
	failures := state.failureCount()
	measured := success + failures
	completed := profileCompletedRuns(state)
	warmupCompleted := profileCompletedWarmup(state)

	measuredElapsed := elapsedBetween(state.measuredStart, state.measuredEnd)
	totalElapsed := elapsedBetween(state.start, state.measuredEnd)
	elapsed := measuredElapsed
	if elapsed <= 0 && totalElapsed > 0 {
		elapsed = totalElapsed
	}

	measuredDuration := profileMeasuredDuration(state.successes, state.failures)
	mt := profileMetrics{
		success:          success,
		failures:         failures,
		warmup:           warmupCompleted,
		total:            completed,
		measured:         measured,
		measuredElapsed:  measuredElapsed,
		totalElapsed:     totalElapsed,
		measuredDuration: measuredDuration,
		elapsed:          elapsed,
		delay:            state.delay,
	}
	mt.successRate = profileSuccessRate(success, measured)
	mt.throughput = profileThroughput(measured, elapsed, state.delay > 0)
	mt.throughputNoWait = profileThroughput(measured, measuredDuration, false)
	return mt
}

func profileCompletedRuns(state *profileState) int {
	if state == nil || state.index < 0 {
		return 0
	}
	if state.total > 0 && state.index > state.total {
		return state.total
	}
	return state.index
}

func profileCompletedWarmup(state *profileState) int {
	if state == nil {
		return 0
	}
	completed := profileCompletedRuns(state)
	if completed < state.warmup {
		return completed
	}
	return state.warmup
}

func elapsedBetween(start, end time.Time) time.Duration {
	if start.IsZero() {
		return 0
	}
	if end.IsZero() {
		end = time.Now()
	}
	if end.Before(start) {
		return 0
	}
	return end.Sub(start)
}

func profileMeasuredDuration(successes []time.Duration, failures []profileFailure) time.Duration {
	total := time.Duration(0)
	for _, d := range successes {
		total += d
	}
	for _, f := range failures {
		if f.Warmup {
			continue
		}
		total += f.Duration
	}
	return total
}

func profileSuccessRate(success, measured int) string {
	if measured <= 0 {
		return "n/a"
	}
	rate := (float64(success) / float64(measured)) * 100
	return fmt.Sprintf("%.0f%% (%d/%d)", rate, success, measured)
}

func profileThroughput(samples int, span time.Duration, includeDelay bool) string {
	if samples <= 0 || span <= 0 {
		return "n/a"
	}
	rps := float64(samples) / span.Seconds()
	text := fmt.Sprintf("%.1f rps", rps)
	if includeDelay {
		text += " (with delay)"
	}
	return text
}

func writeProfileHeader(b *strings.Builder, title string) {
	b.WriteString(title)
	b.WriteString("\n")

	lineWidth := len(title)
	if lineWidth < 12 {
		lineWidth = 12
	}
	b.WriteString(strings.Repeat("─", lineWidth))
	b.WriteString("\n\n")
}

func (m *Model) showProfileProgress(state *profileState) {
	if state == nil {
		return
	}
	dots := profileProgressDots(m.statusPulseFrame)
	m.setStatusMessage(statusMsg{text: profileProgressText(state, dots), level: statusInfo})
}

func profileProgressText(state *profileState, dots int) string {
	base := strings.TrimSpace(profileProgressLabel(state))
	if base == "" {
		base = "Profiling in progress"
	}
	if dots < 1 {
		dots = 1
	}
	if dots > 3 {
		dots = 3
	}
	return base + strings.Repeat(".", dots)
}

func profileProgressDots(frame int) int {
	if frame < 0 {
		frame = 0
	}
	return (frame % 3) + 1
}

func profileStatusText(state *profileState) string {
	if state == nil {
		return ""
	}
	if state.skipped {
		reason := strings.TrimSpace(state.skipReason)
		if reason == "" {
			reason = "condition evaluated to false"
		}
		return fmt.Sprintf("Skipped: %s", reason)
	}
	if !state.canceled {
		return ""
	}
	if summary := buildProfileSummary(state); strings.TrimSpace(summary) != "" {
		return summary
	}
	if reason := strings.TrimSpace(state.cancelReason); reason != "" {
		return reason
	}
	return "Profiling canceled"
}

func writeProfileSummary(b *strings.Builder, state *profileState, mt profileMetrics) {
	if state == nil {
		return
	}

	b.WriteString("Summary:\n")
	if status := profileStatusText(state); status != "" {
		writeProfileRow(b, "Status", status)
	}
	writeProfileRow(b, "Runs", formatProfileRuns(mt))
	writeProfileRow(b, "Success", mt.successRate)
	writeProfileRow(b, "Window", formatProfileWindow(mt))
	if state.delay > 0 {
		writeProfileRow(
			b,
			"Delay",
			fmt.Sprintf("%s between runs", formatDurationShort(state.delay)),
		)
	}
	writeProfileRow(b, "Throughput", formatProfileThroughput(mt))
	if mt.success == 0 {
		writeProfileRow(b, "Note", "No successful measurements.")
	}
	b.WriteString("\n")
}

func writeProfileRow(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "  %-10s %s\n", label+":", value)
}

func formatProfileRuns(mt profileMetrics) string {
	parts := []string{
		fmt.Sprintf("%d total", mt.total),
		fmt.Sprintf("%d success", mt.success),
		fmt.Sprintf("%d failure", mt.failures),
	}
	if mt.warmup > 0 {
		parts = append(parts, fmt.Sprintf("%d warmup", mt.warmup))
	}
	return strings.Join(parts, " | ")
}

func formatProfileWindow(mt profileMetrics) string {
	if mt.measured <= 0 && mt.totalElapsed <= 0 {
		return "n/a"
	}

	var parts []string
	if mt.measured > 0 && mt.elapsed > 0 {
		runLabel := "run"
		if mt.measured != 1 {
			runLabel = "runs"
		}
		parts = append(
			parts,
			fmt.Sprintf("%d %s in %s", mt.measured, runLabel, formatDurationShort(mt.elapsed)),
		)
	} else if mt.elapsed > 0 {
		parts = append(parts, formatDurationShort(mt.elapsed))
	}
	if mt.totalElapsed > 0 && mt.totalElapsed != mt.elapsed {
		parts = append(parts, fmt.Sprintf("wall %s", formatDurationShort(mt.totalElapsed)))
	}
	return strings.Join(parts, " | ")
}

func formatProfileThroughput(mt profileMetrics) string {
	if mt.throughput == "n/a" && mt.throughputNoWait == "n/a" {
		return "n/a"
	}
	if mt.throughput == "n/a" {
		return mt.throughputNoWait
	}
	if mt.throughputNoWait == "n/a" || mt.throughputNoWait == mt.throughput {
		return mt.throughput
	}
	return fmt.Sprintf("%s | no-delay: %s", mt.throughput, mt.throughputNoWait)
}

func writeLatencySection(b *strings.Builder, stats analysis.LatencyStats) {
	if stats.Count == 0 {
		return
	}
	fmt.Fprintf(b, "Latency (%d samples):\n", stats.Count)
	b.WriteString(renderLatencyTable(stats))
}

func writeDistributionSection(b *strings.Builder, stats analysis.LatencyStats) {
	if len(stats.Histogram) == 0 {
		return
	}
	b.WriteString("\nDistribution:\n")
	b.WriteString(renderHistogram(stats.Histogram, histogramDefaultIndent))
	b.WriteString("\n")
	b.WriteString(renderHistogramLegend(histogramDefaultIndent))
}

func writeFailureSection(b *strings.Builder, state *profileState) {
	if state == nil || len(state.failures) == 0 {
		return
	}
	b.WriteString("\nFailures:\n")
	for _, failure := range state.failures {
		b.WriteString(formatProfileFailure(failure))
	}
}

func formatProfileFailure(failure profileFailure) string {
	label := fmt.Sprintf("Run %d", failure.Iteration)
	if failure.Warmup {
		label = fmt.Sprintf("Warmup %d", failure.Iteration)
	}

	details := strings.TrimSpace(failure.Reason)
	meta := formatFailureMeta(failure)
	switch {
	case details != "" && meta != "":
		details = fmt.Sprintf("%s [%s]", details, meta)
	case details == "" && meta != "":
		details = meta
	case details == "":
		details = "failed"
	}
	return fmt.Sprintf("  - %s: %s\n", label, details)
}

func formatFailureMeta(failure profileFailure) string {
	parts := make([]string, 0, 3)
	if failure.Status != "" {
		parts = append(parts, failure.Status)
	}
	if failure.Duration > 0 {
		parts = append(parts, formatDurationShort(failure.Duration))
	}
	return strings.Join(parts, " | ")
}

func (m *Model) recordProfileHistory(
	st *profileState,
	stats analysis.LatencyStats,
	msg responseMsg,
	report string,
) {
	hs := m.historyStore()
	if hs == nil || st == nil || st.base == nil || st.base.Metadata.NoLog {
		return
	}

	entry := m.buildProfileHistoryEntry(st, stats, msg, report)
	if entry == nil {
		return
	}

	if err := hs.Append(*entry); err != nil {
		m.setStatusMessage(
			statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn},
		)
		return
	}
	m.historySelectedID = entry.ID
	m.syncHistory()
}

func (m *Model) buildProfileHistoryEntry(
	st *profileState,
	stats analysis.LatencyStats,
	msg responseMsg,
	report string,
) *history.Entry {
	req := st.base
	if req == nil {
		return nil
	}

	secrets := m.secretValuesForRedaction(req)
	mask := !req.Metadata.AllowSensitiveHeaders
	text := redactHistoryText(renderRequestText(req), secrets, mask)
	status, code := profileHistoryStatus(st, msg)
	now := time.Now()
	dur := time.Duration(0)
	if !st.start.IsZero() {
		dur = now.Sub(st.start)
	}

	return &history.Entry{
		ID:             fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:     now,
		Environment:    msg.environment,
		RequestName:    requestIdentifier(req),
		FilePath:       m.historyFilePath(),
		Method:         req.Method,
		URL:            req.URL,
		Status:         status,
		StatusCode:     code,
		Duration:       dur,
		BodySnippet:    "<profile run – see profileResults>",
		RequestText:    text,
		Description:    strings.TrimSpace(req.Metadata.Description),
		Tags:           normalizedTags(req.Metadata.Tags),
		ProfileResults: buildProfileResults(st, stats),
	}
}

func profileHistoryStatus(st *profileState, msg responseMsg) (string, int) {
	if st != nil && st.skipped {
		reason := strings.TrimSpace(st.skipReason)
		if reason == "" {
			reason = "SKIPPED"
		}
		if !strings.EqualFold(reason, "skipped") {
			return fmt.Sprintf("SKIPPED: %s", reason), 0
		}
		return "SKIPPED", 0
	}
	if st != nil && st.canceled {
		completed := profileCompletedRuns(st)
		total := st.total
		if total == 0 {
			total = st.spec.Count + st.warmup
		}
		if completed > 0 && total > 0 {
			return fmt.Sprintf("Canceled at %d/%d", completed, total), 0
		}
		return strings.TrimSpace(st.cancelReason), 0
	}

	switch {
	case msg.response != nil:
		return msg.response.Status, msg.response.StatusCode
	case msg.err != nil:
		return msg.err.Error(), 0
	case msg.scriptErr != nil:
		return msg.scriptErr.Error(), 0
	case st != nil && len(st.failures) > 0:
		last := st.failures[len(st.failures)-1]
		status := strings.TrimSpace(last.Status)
		if status == "" {
			status = strings.TrimSpace(last.Reason)
		}
		if status == "" {
			status = "profile failed"
		}
		return status, last.StatusCode
	default:
		return "profile completed", 0
	}
}

func buildProfileResults(st *profileState, stats analysis.LatencyStats) *history.ProfileResults {
	if st == nil {
		return nil
	}

	totalRuns := profileCompletedRuns(st)
	if totalRuns == 0 {
		totalRuns = st.total
	}
	warmupRuns := profileCompletedWarmup(st)

	res := &history.ProfileResults{
		TotalRuns:      totalRuns,
		WarmupRuns:     warmupRuns,
		SuccessfulRuns: len(st.successes),
		FailedRuns:     st.failureCount(),
	}
	if stats.Count == 0 {
		return res
	}

	res.Latency = &history.ProfileLatency{
		Count:  stats.Count,
		Min:    stats.Min,
		Max:    stats.Max,
		Mean:   stats.Mean,
		Median: stats.Median,
		StdDev: stats.StdDev,
	}

	if len(stats.Percentiles) > 0 {
		ps := make([]history.ProfilePercentile, 0, len(stats.Percentiles))
		for p, v := range stats.Percentiles {
			ps = append(ps, history.ProfilePercentile{Percentile: p, Value: v})
		}
		sort.Slice(ps, func(i, j int) bool { return ps[i].Percentile < ps[j].Percentile })
		res.Percentiles = ps
	}

	if len(stats.Histogram) > 0 {
		bins := make([]history.ProfileHistogramBin, len(stats.Histogram))
		for i, b := range stats.Histogram {
			bins[i] = history.ProfileHistogramBin{From: b.From, To: b.To, Count: b.Count}
		}
		res.Histogram = bins
	}

	return res
}
