package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/history"
)

func (m *Model) recordProfileHistory(
	st *profileState,
	stats analysis.LatencyStats,
	msg responseMsg,
	report string,
) {
	if m.historyStore == nil || st == nil || st.base == nil {
		return
	}
	if st.base.Metadata.NoLog {
		return
	}

	entry := m.buildProfileHistoryEntry(st, stats, msg, report)
	if entry == nil {
		return
	}

	if err := m.historyStore.Append(*entry); err != nil {
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
	desc := strings.TrimSpace(req.Metadata.Description)
	tags := normalizedTags(req.Metadata.Tags)
	status, code := profileHistoryStatus(st, msg)
	res := buildProfileResults(st, stats)
	now := time.Now()
	dur := time.Duration(0)
	if !st.start.IsZero() {
		dur = now.Sub(st.start)
	}
	snippet := "<profile run – see profileResults>"

	return &history.Entry{
		ID:             fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:     now,
		Environment:    msg.environment,
		RequestName:    requestIdentifier(req),
		Method:         req.Method,
		URL:            req.URL,
		Status:         status,
		StatusCode:     code,
		Duration:       dur,
		BodySnippet:    snippet,
		RequestText:    text,
		Description:    desc,
		Tags:           tags,
		ProfileResults: res,
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
		return errdef.Message(msg.err), 0
	case msg.scriptErr != nil:
		return strings.TrimSpace(msg.scriptErr.Error()), 0
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
