package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type profileState struct {
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
	count := 0
	for _, failure := range s.failures {
		if !failure.Warmup {
			count++
		}
	}
	return count
}

func (m *Model) startProfileRun(doc *restfile.Document, req *restfile.Request, options httpclient.Options) tea.Cmd {
	if req == nil {
		return nil
	}
	if req.GRPC != nil {
		m.setStatusMessage(statusMsg{text: "Profiling is not supported for gRPC requests", level: statusWarn})
		return m.executeRequest(doc, req, options, "")
	}

	spec := restfile.ProfileSpec{}
	if req.Metadata.Profile != nil {
		spec = *req.Metadata.Profile
	}
	if spec.Count <= 0 {
		spec.Count = 10
	}
	if spec.Warmup < 0 {
		spec.Warmup = 0
	}
	if spec.Delay < 0 {
		spec.Delay = 0
	}

	total := spec.Count + spec.Warmup
	if total <= 0 {
		total = spec.Count
	}

	state := &profileState{
		base:        cloneRequest(req),
		doc:         doc,
		options:     options,
		spec:        spec,
		total:       total,
		warmup:      spec.Warmup,
		delay:       spec.Delay,
		successes:   make([]time.Duration, 0, spec.Count),
		failures:    make([]profileFailure, 0, spec.Count/2+1),
		messageBase: fmt.Sprintf("Profiling %s", requestBaseTitle(req)),
		start:       time.Now(),
	}

	m.profileRun = state
	m.sending = true
	m.statusPulseBase = ""
	m.statusPulseFrame = 0

	m.setStatusMessage(statusMsg{text: fmt.Sprintf("%s warmup 0/%d", state.messageBase, state.warmup), level: statusInfo})
	return m.executeProfileIteration()
}

func (m *Model) executeProfileIteration() tea.Cmd {
	state := m.profileRun
	if state == nil {
		return nil
	}
	if state.index >= state.total {
		return nil
	}

	iterationReq := cloneRequest(state.base)
	state.current = iterationReq
	m.currentRequest = iterationReq

	if state.index >= state.warmup && state.measuredStart.IsZero() {
		state.measuredStart = time.Now()
	}

	progressText := profileProgressLabel(state)
	m.setStatusMessage(statusMsg{text: progressText, level: statusInfo})

	cmd := m.executeRequest(state.doc, iterationReq, state.options, "")
	return cmd
}

func (m *Model) handleProfileResponse(msg responseMsg) tea.Cmd {
	state := m.profileRun
	if state == nil {
		return nil
	}

	m.lastError = nil
	m.testResults = msg.tests
	m.scriptError = msg.scriptErr
	if msg.err != nil {
		m.lastError = msg.err
		m.lastResponse = nil
		m.lastGRPC = nil
	}

	duration := time.Duration(0)
	if msg.response != nil {
		duration = msg.response.Duration
	}

	success, reason := evaluateProfileOutcome(msg)
	warmup := state.index < state.warmup

	if !warmup {
		now := time.Now()
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
	state.current = nil
	m.statusPulseBase = ""

	if state.index < state.total {
		progressText := profileProgressLabel(state)
		m.setStatusMessage(statusMsg{text: progressText, level: statusInfo})
		if state.delay > 0 {
			return tea.Tick(state.delay, func(time.Time) tea.Msg { return profileNextIterationMsg{} })
		}
		return m.executeProfileIteration()
	}

	return m.finalizeProfileRun(msg, state)
}

func evaluateProfileOutcome(msg responseMsg) (bool, string) {
	if msg.err != nil {
		return false, errdef.Message(msg.err)
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
	m.sending = false
	m.statusPulseBase = ""

	report := ""
	var stats analysis.LatencyStats
	var statsPtr *analysis.LatencyStats
	if len(state.successes) > 0 {
		stats = analysis.ComputeLatencyStats(state.successes, []int{50, 90, 95, 99}, 10)
		statsPtr = &stats
		report = m.buildProfileReport(state, stats)
	} else {
		report = m.buildProfileReport(state, stats)
	}

	var cmds []tea.Cmd
	if msg.err != nil {
		if cmd := m.consumeRequestError(msg.err); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else if msg.response != nil {
		if cmd := m.consumeHTTPResponse(msg.response, msg.tests, msg.scriptErr, msg.environment); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else {
		snapshot := &responseSnapshot{
			pretty:         report,
			raw:            report,
			headers:        report,
			requestHeaders: report,
			stats:          report,
			statsColorize:  true,
			statsKind:      statsReportKindProfile,
			profileStats:   statsPtr,
			statsColored:   "",
			ready:          true,
		}
		m.responseLatest = snapshot
		m.responsePending = nil
	}

	if m.responseLatest != nil {
		m.responseLatest.stats = report
		m.responseLatest.statsColored = ""
		m.responseLatest.statsColorize = true
		m.responseLatest.statsKind = statsReportKindProfile
		m.responseLatest.profileStats = statsPtr
	}

	m.recordProfileHistory(state, stats, msg, report)

	summary := buildProfileSummary(state)
	m.setStatusMessage(statusMsg{text: summary, level: statusInfo})

	if len(cmds) == 0 {
		return nil
	}
	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

func buildProfileSummary(state *profileState) string {
	if state == nil {
		return "Profiling complete"
	}

	mt := profileMetricsFromState(state)
	return fmt.Sprintf("Profiling complete: %d/%d success (%d failure, %d warmup)", mt.success, state.spec.Count, mt.failures, mt.warmup)
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

	var builder strings.Builder
	builder.WriteString(formatLatencyRow(labels, widths))
	builder.WriteString("\n")
	builder.WriteString(formatLatencyRow(values, widths))
	builder.WriteString("\n")
	builder.WriteString("  mean: ")
	builder.WriteString(formatDurationShort(stats.Mean))
	builder.WriteString(" | median: ")
	builder.WriteString(formatDurationShort(stats.Median))
	builder.WriteString(" | stddev: ")
	builder.WriteString(formatDurationShort(stats.StdDev))
	builder.WriteString("\n")
	return builder.String()
}

func formatLatencyRow(items []string, widths []int) string {
	var builder strings.Builder
	builder.WriteString("  ")
	for i, item := range items {
		if i > 0 {
			builder.WriteString("  ")
		}
		builder.WriteString(fmt.Sprintf("%-*s", widths[i], item))
	}
	return builder.String()
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
		warmup:           state.warmup,
		total:            state.total,
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

func writeProfileSummary(b *strings.Builder, state *profileState, mt profileMetrics) {
	if state == nil {
		return
	}

	b.WriteString("Summary:\n")
	writeProfileRow(b, "Runs", formatProfileRuns(mt))
	writeProfileRow(b, "Success", mt.successRate)
	writeProfileRow(b, "Window", formatProfileWindow(mt))
	if state.delay > 0 {
		writeProfileRow(b, "Delay", fmt.Sprintf("%s between runs", formatDurationShort(state.delay)))
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
		parts = append(parts, fmt.Sprintf("%d %s in %s", mt.measured, runLabel, formatDurationShort(mt.elapsed)))
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
