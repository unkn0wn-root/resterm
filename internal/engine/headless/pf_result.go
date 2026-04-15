package headless

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type profileState struct {
	req       *restfile.Request
	env       string
	spec      restfile.ProfileSpec
	total     int
	idx       int
	start     time.Time
	end       time.Time
	mStart    time.Time
	mEnd      time.Time
	ok        []time.Duration
	fail      []engine.ProfileFailure
	skip      bool
	skipMsg   string
	cancel    bool
	cancelMsg string
}

func profileOutcome(out engine.RequestResult) (bool, string) {
	if out.Skipped {
		reason := strings.TrimSpace(out.SkipReason)
		if reason == "" {
			reason = "request skipped"
		}
		return false, reason
	}
	if out.Err != nil {
		return false, errdef.Message(out.Err)
	}
	if out.Response != nil && out.Response.StatusCode >= 400 {
		return false, fmt.Sprintf("HTTP %s", out.Response.Status)
	}
	if out.ScriptErr != nil {
		return false, out.ScriptErr.Error()
	}
	for _, test := range out.Tests {
		if test.Passed {
			continue
		}
		if strings.TrimSpace(test.Message) != "" {
			return false, fmt.Sprintf("Test failed: %s – %s", test.Name, test.Message)
		}
		return false, fmt.Sprintf("Test failed: %s", test.Name)
	}
	if out.Response == nil {
		return false, "no response"
	}
	return true, ""
}

func buildProfileResults(st *profileState, stats analysis.LatencyStats) *history.ProfileResults {
	if st == nil {
		return nil
	}

	return &history.ProfileResults{
		TotalRuns:      st.idx,
		WarmupRuns:     min(st.idx, st.spec.Warmup),
		SuccessfulRuns: len(st.ok),
		FailedRuns:     len(st.fail),
		Latency:        buildProfileLatency(stats),
		Percentiles:    buildProfilePercentiles(stats.Percentiles),
		Histogram:      buildProfileHistogram(stats.Histogram),
	}
}

func buildProfileLatency(stats analysis.LatencyStats) *history.ProfileLatency {
	if stats.Count == 0 {
		return nil
	}
	return &history.ProfileLatency{
		Count:  stats.Count,
		Min:    stats.Min,
		Max:    stats.Max,
		Mean:   stats.Mean,
		Median: stats.Median,
		StdDev: stats.StdDev,
	}
}

func buildProfilePercentiles(src map[int]time.Duration) []history.ProfilePercentile {
	if len(src) == 0 {
		return nil
	}

	out := make([]history.ProfilePercentile, 0, len(src))
	for percentile, value := range src {
		out = append(out, history.ProfilePercentile{
			Percentile: percentile,
			Value:      value,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Percentile < out[j].Percentile
	})
	return out
}

func buildProfileHistogram(src []analysis.HistogramBucket) []history.ProfileHistogramBin {
	if len(src) == 0 {
		return nil
	}

	out := make([]history.ProfileHistogramBin, len(src))
	for i, bin := range src {
		out[i] = history.ProfileHistogramBin{
			From:  bin.From,
			To:    bin.To,
			Count: bin.Count,
		}
	}
	return out
}
