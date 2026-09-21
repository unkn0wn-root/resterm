package core

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// History keeps exact counts but stores details for only the first 20 failures.
const maxProfileHistoryFailures = 20

// HistoryEntry builds a record without samples or response bodies. The redact
// function removes secrets and, when mask is true, sensitive headers.
func (s ProfileSnapshot) HistoryEntry(
	req *restfile.Request,
	env vars.Environment,
	path string,
	redact func(text string, mask bool) string,
) history.Entry {
	now := time.Now()
	return history.Entry{
		ID:                   fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:           now,
		Environment:          env.Label(),
		EnvironmentSelection: history.EnvironmentSelection(env.Selection().Groups()),
		RequestName:          engine.ReqID(req),
		FilePath:             path,
		Method:               req.Method,
		URL:                  req.URL,
		Status:               s.historyStatus(),
		Duration:             s.Progress.Elapsed,
		BodySnippet:          "<profile run - see profileResults>",
		RequestText:          redact(request.RenderRequestText(req), !req.Metadata.AllowSensitiveHeaders),
		Description:          strings.TrimSpace(req.Metadata.Description),
		Tags:                 engine.Tags(req.Metadata.Tags),
		ProfileResults:       s.historyResults(func(text string) string { return redact(text, false) }),
	}
}

func (s ProfileSnapshot) historyStatus() string {
	p := s.Progress
	name := strings.ToUpper(p.Status.String())
	switch p.Status {
	case ProfileSkipped, ProfileError:
		return name
	default:
		return fmt.Sprintf("%s %d/%d", name, p.Passed, p.Count)
	}
}

func (s ProfileSnapshot) historyResults(redact func(string) string) *history.ProfileResults {
	p := s.Progress
	out := &history.ProfileResults{
		TotalRuns:        p.Done,
		WarmupRuns:       p.WarmupDone,
		SuccessfulRuns:   p.Passed,
		FailedRuns:       p.Failed,
		Latency:          historyLatency(s.Stats),
		Percentiles:      historyPercentiles(s.Stats.Percentiles),
		Histogram:        historyHistogram(s.Stats.Histogram),
		Status:           p.Status.String(),
		Count:            p.Count,
		Delay:            s.Delay,
		Window:           s.Window,
		Active:           s.Active,
		WarmupFailedRuns: p.WarmupFailed,
	}
	if s.Err != nil {
		out.Error = redact(s.Err.Error())
	}
	for _, f := range s.Failures[:min(len(s.Failures), maxProfileHistoryFailures)] {
		out.Failures = append(out.Failures, history.ProfileFailure{
			Iteration:  f.Iteration,
			Warmup:     f.Warmup,
			Reason:     redact(f.Reason),
			StatusCode: f.StatusCode,
			Duration:   f.Duration,
		})
	}
	return out
}

// ProfileSnapshotFromHistory restores a completed profile run. For older
// records, it derives the status from the counts and sets Legacy.
func ProfileSnapshotFromHistory(ent history.Entry) ProfileSnapshot {
	r := ent.ProfileResults
	measured := r.SuccessfulRuns + r.FailedRuns
	s := ProfileSnapshot{
		Progress: ProfileProgress{
			Total:        r.TotalRuns,
			Done:         r.TotalRuns,
			Warmup:       r.WarmupRuns,
			WarmupDone:   r.WarmupRuns,
			WarmupFailed: r.WarmupFailedRuns,
			Count:        r.Count,
			Measured:     measured,
			Passed:       r.SuccessfulRuns,
			Failed:       r.FailedRuns,
			Elapsed:      ent.Duration,
		},
		Delay:  r.Delay,
		Window: r.Window,
		Active: r.Active,
		Stats:  statsFromHistory(r),
	}
	if r.Error != "" {
		s.Err = errors.New(r.Error)
	}
	for _, f := range r.Failures {
		s.Failures = append(s.Failures, engine.ProfileFailure{
			Iteration:  f.Iteration,
			Warmup:     f.Warmup,
			Reason:     f.Reason,
			StatusCode: f.StatusCode,
			Duration:   f.Duration,
		})
	}

	status, ok := ParseProfileStatus(r.Status)
	if !ok {
		s.Legacy = true
		s.Progress.Count = measured
		status = ProfilePass
		if r.FailedRuns > 0 {
			status = ProfileFail
		}
	}
	s.Progress.Status = status
	return s
}

func historyLatency(st analysis.LatencyStats) *history.ProfileLatency {
	if st.Count == 0 {
		return nil
	}
	return &history.ProfileLatency{
		Count:  st.Count,
		Min:    st.Min,
		Max:    st.Max,
		Mean:   st.Mean,
		Median: st.Median,
		StdDev: st.StdDev,
	}
}

func historyPercentiles(src map[int]time.Duration) []history.ProfilePercentile {
	var out []history.ProfilePercentile
	for _, p := range slices.Sorted(maps.Keys(src)) {
		out = append(out, history.ProfilePercentile{Percentile: p, Value: src[p]})
	}
	return out
}

func historyHistogram(src []analysis.HistogramBucket) []history.ProfileHistogramBin {
	var out []history.ProfileHistogramBin
	for _, b := range src {
		out = append(out, history.ProfileHistogramBin{From: b.From, To: b.To, Count: b.Count})
	}
	return out
}

func statsFromHistory(r *history.ProfileResults) analysis.LatencyStats {
	var st analysis.LatencyStats
	if l := r.Latency; l != nil {
		st = analysis.LatencyStats{
			Count:  l.Count,
			Min:    l.Min,
			Max:    l.Max,
			Mean:   l.Mean,
			Median: l.Median,
			StdDev: l.StdDev,
		}
	}
	if len(r.Percentiles) > 0 {
		st.Percentiles = make(map[int]time.Duration, len(r.Percentiles))
		for _, p := range r.Percentiles {
			st.Percentiles[p.Percentile] = p.Value
		}
	}
	for _, b := range r.Histogram {
		st.Histogram = append(st.Histogram, analysis.HistogramBucket{From: b.From, To: b.To, Count: b.Count})
	}
	return st
}
