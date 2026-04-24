package headless

import (
	"fmt"
	"sort"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runfail"
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

type profileFailureOutcome struct {
	reason  string
	failure runfail.Failure
}

func profileOutcome(out engine.RequestResult) (bool, profileFailureOutcome) {
	if out.Skipped {
		reason := out.SkipReason
		if reason == "" {
			reason = "request skipped"
		}
		failure := runfail.Assertion(reason, "profile")
		return false, profileFailureOutcome{reason: failure.Message, failure: failure}
	}
	if out.Err != nil {
		failure := runfail.FromErrorSource(out.Err, "profile")
		return false, profileFailureOutcome{reason: failure.Message, failure: failure}
	}
	if out.Response != nil && out.Response.StatusCode >= 400 {
		failure := runfail.Assertion(
			profileHTTPStatus(out.Response.Status, out.Response.StatusCode),
			"profile",
		)
		return false, profileFailureOutcome{reason: failure.Message, failure: failure}
	}
	if out.ScriptErr != nil {
		failure := runfail.Script(out.ScriptErr.Error(), "profile")
		return false, profileFailureOutcome{reason: failure.Message, failure: failure}
	}
	for _, test := range out.Tests {
		if test.Passed {
			continue
		}
		failure := runfail.Assertion(
			profileTestFailure(test.Name, test.Message),
			"profile",
		)
		return false, profileFailureOutcome{reason: failure.Message, failure: failure}
	}
	if out.Response == nil {
		failure := runfail.New(
			runfail.CodeProtocol,
			"no response",
			"profile",
		)
		return false, profileFailureOutcome{reason: failure.Message, failure: failure}
	}
	return true, profileFailureOutcome{}
}

func profileHTTPStatus(status string, statusCode int) string {
	if status != "" {
		return "HTTP " + status
	}
	if statusCode != 0 {
		return fmt.Sprintf("HTTP %d", statusCode)
	}
	return "HTTP request failed"
}

func profileTestFailure(name, msg string) string {
	if msg != "" {
		if name != "" {
			return fmt.Sprintf("Test failed: %s - %s", name, msg)
		}
		return "Test failed: " + msg
	}
	if name != "" {
		return "Test failed: " + name
	}
	return "Test failed"
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
