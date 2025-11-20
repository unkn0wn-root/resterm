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
	base        *restfile.Request
	doc         *restfile.Document
	options     httpclient.Options
	spec        restfile.ProfileSpec
	total       int
	warmup      int
	delay       time.Duration
	index       int
	successes   []time.Duration
	failures    []profileFailure
	current     *restfile.Request
	messageBase string
	start       time.Time
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
	if len(state.successes) > 0 {
		stats = analysis.ComputeLatencyStats(state.successes, []int{50, 90, 95, 99}, 10)
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
			pretty:        report,
			raw:           report,
			headers:       report,
			stats:         report,
			statsColorize: true,
			statsKind:     statsReportKindProfile,
			statsColored:  "",
			ready:         true,
		}
		m.responseLatest = snapshot
		m.responsePending = nil
	}

	if m.responseLatest != nil {
		m.responseLatest.stats = report
		m.responseLatest.statsColored = ""
		m.responseLatest.statsColorize = true
		m.responseLatest.statsKind = statsReportKindProfile
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
	success := state.successCount()
	failures := state.failureCount()
	return fmt.Sprintf("Profiling complete: %d/%d success (%d failure, %d warmup)", success, state.spec.Count, failures, state.warmup)
}

func (m *Model) buildProfileReport(state *profileState, stats analysis.LatencyStats) string {
	var builder strings.Builder
	title := state.messageBase
	builder.WriteString(title)
	builder.WriteString("\n")

	lineWidth := len(title)
	if lineWidth < 12 {
		lineWidth = 12
	}
	builder.WriteString(strings.Repeat("─", lineWidth))
	builder.WriteString("\n\n")

	success := state.successCount()
	failures := state.failureCount()
	measured := success + failures

	appendSummaryRow := func(label, value string) {
		builder.WriteString("  ")
		builder.WriteString(fmt.Sprintf("%-10s", label+":"))
		builder.WriteString(" ")
		builder.WriteString(value)
		builder.WriteString("\n")
	}

	builder.WriteString("Summary:\n")
	warmupText := ""
	if state.warmup > 0 {
		warmupText = fmt.Sprintf(" | %d warmup", state.warmup)
	}
	appendSummaryRow("Runs", fmt.Sprintf("%d total | %d success | %d failure%s", state.total, success, failures, warmupText))

	successRate := "n/a"
	if measured > 0 {
		rate := (float64(success) / float64(measured)) * 100
		successRate = fmt.Sprintf("%.0f%% (%d/%d)", rate, success, measured)
	}
	appendSummaryRow("Success", successRate)

	elapsed := time.Duration(0)
	if !state.start.IsZero() {
		elapsed = time.Since(state.start)
	}
	throughput := "n/a"
	if elapsed > 0 && measured > 0 {
		throughput = fmt.Sprintf("%.1f rps", float64(measured)/elapsed.Seconds())
	}
	appendSummaryRow("Elapsed", fmt.Sprintf("%s | Throughput: %s", formatDurationShort(elapsed), throughput))
	if success == 0 {
		appendSummaryRow("Note", "No successful measurements.")
	}

	if stats.Count > 0 {
		builder.WriteString("\n")
		builder.WriteString(fmt.Sprintf("Latency (%d samples):\n", stats.Count))
		builder.WriteString(renderLatencyTable(stats))
	}

	if len(stats.Histogram) > 0 {
		builder.WriteString("\nDistribution:\n")
		builder.WriteString(renderHistogram(stats.Histogram, "  "))
	}

	if len(state.failures) > 0 {
		builder.WriteString("\nFailures:\n")
		for _, failure := range state.failures {
			label := fmt.Sprintf("Run %d", failure.Iteration)
			if failure.Warmup {
				label = fmt.Sprintf("Warmup %d", failure.Iteration)
			}
			details := strings.TrimSpace(failure.Reason)
			var meta []string
			if failure.Status != "" {
				meta = append(meta, failure.Status)
			}
			if failure.Duration > 0 {
				meta = append(meta, formatDurationShort(failure.Duration))
			}
			if len(meta) > 0 {
				if details == "" {
					details = strings.Join(meta, " | ")
				} else {
					details = fmt.Sprintf("%s [%s]", details, strings.Join(meta, " | "))
				}
			}
			if details == "" {
				details = "failed"
			}
			builder.WriteString(fmt.Sprintf("  - %s: %s\n", label, details))
		}
	}

	return strings.TrimRight(builder.String(), "\n")
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
