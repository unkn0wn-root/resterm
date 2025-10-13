package ui

import (
	"fmt"
	"sort"
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
		return m.executeRequest(doc, req, options)
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

	cmd := m.executeRequest(state.doc, iterationReq, state.options)
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
		if cmd := m.consumeHTTPResponse(msg.response, msg.tests, msg.scriptErr); cmd != nil {
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

	success := state.successCount()
	failures := state.failureCount()
	builder.WriteString(fmt.Sprintf("Measured runs: %d\n", success))
	if state.warmup > 0 {
		builder.WriteString(fmt.Sprintf("Warmup runs: %d\n", state.warmup))
	}
	builder.WriteString(fmt.Sprintf("Failures: %d\n", failures))
	if success == 0 {
		builder.WriteString("\nNo successful measurements.\n")
	}

	if stats.Count > 0 {
		builder.WriteString("\nLatency Summary:\n")
		builder.WriteString(fmt.Sprintf("  Min: %s\n", stats.Min))
		builder.WriteString(fmt.Sprintf("  Max: %s\n", stats.Max))
		builder.WriteString(fmt.Sprintf("  Mean: %s\n", stats.Mean))
		builder.WriteString(fmt.Sprintf("  Median: %s\n", stats.Median))
		builder.WriteString(fmt.Sprintf("  StdDev: %s\n", stats.StdDev))

		if len(stats.Percentiles) > 0 {
			builder.WriteString("\n  Percentiles:\n")
			keys := make([]int, 0, len(stats.Percentiles))
			for k := range stats.Percentiles {
				keys = append(keys, k)
			}
			sort.Ints(keys)
			for _, k := range keys {
				builder.WriteString(fmt.Sprintf("    P%d: %s\n", k, stats.Percentiles[k]))
			}
		}

		if len(stats.Histogram) > 0 {
			builder.WriteString("\n")
			builder.WriteString(renderHistogram(stats.Histogram))
		}
	}

	if len(state.failures) > 0 {
		builder.WriteString("\nFailures:\n")
		for _, failure := range state.failures {
			label := fmt.Sprintf("Run %d", failure.Iteration)
			if failure.Warmup {
				label = fmt.Sprintf("Warmup %d", failure.Iteration)
			}
			builder.WriteString(fmt.Sprintf("  - %s: %s", label, failure.Reason))
			if failure.Status != "" {
				builder.WriteString(fmt.Sprintf(" [%s]", failure.Status))
			}
			builder.WriteString("\n")
		}
	}

	return strings.TrimRight(builder.String(), "\n")
}
