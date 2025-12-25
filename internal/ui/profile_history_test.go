package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestBuildProfileResults(t *testing.T) {
	st := &profileState{
		total:     4,
		warmup:    1,
		successes: []time.Duration{10 * time.Millisecond, 30 * time.Millisecond},
		failures:  []profileFailure{{Warmup: false}, {Warmup: true}},
		index:     4,
	}
	stats := analysis.ComputeLatencyStats(st.successes, []int{50, 90}, 2)
	res := buildProfileResults(st, stats)
	if res == nil {
		t.Fatalf("expected results")
	}
	if res.TotalRuns != st.total {
		t.Fatalf("expected total runs %d, got %d", st.total, res.TotalRuns)
	}
	if res.WarmupRuns != st.warmup {
		t.Fatalf("expected warmup %d, got %d", st.warmup, res.WarmupRuns)
	}
	if res.SuccessfulRuns != len(st.successes) {
		t.Fatalf("expected successes %d, got %d", len(st.successes), res.SuccessfulRuns)
	}
	if res.FailedRuns != st.failureCount() {
		t.Fatalf("expected failures %d, got %d", st.failureCount(), res.FailedRuns)
	}
	if stats.Count > 0 {
		if res.Latency == nil {
			t.Fatalf("expected latency stats")
		}
		if len(res.Percentiles) != len(stats.Percentiles) {
			t.Fatalf(
				"expected %d percentiles, got %d",
				len(stats.Percentiles),
				len(res.Percentiles),
			)
		}
		for i := 1; i < len(res.Percentiles); i++ {
			if res.Percentiles[i-1].Percentile > res.Percentiles[i].Percentile {
				t.Fatalf("expected percentiles sorted ascending")
			}
		}
		if len(res.Histogram) != len(stats.Histogram) {
			t.Fatalf("expected histogram size %d, got %d", len(stats.Histogram), len(res.Histogram))
		}
	}
}

func TestRecordProfileHistoryStoresEntry(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	model := New(Config{History: store})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Name:        "Profile Run",
			Description: "profiling endpoint",
			Tags:        []string{" perf ", "benchmark"},
		},
	}
	st := &profileState{
		base:      req,
		total:     3,
		warmup:    1,
		successes: []time.Duration{20 * time.Millisecond, 25 * time.Millisecond},
		index:     3,
		start:     time.Time{},
	}
	stats := analysis.ComputeLatencyStats(st.successes, []int{90}, 2)
	resp := &httpclient.Response{Status: "200 OK", StatusCode: 200}
	msg := responseMsg{response: resp, environment: "dev"}
	report := "Profiling Profile Run\nMeasured runs: 2"

	model.recordProfileHistory(st, stats, msg, report)

	entries := store.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Method != req.Method || entry.URL != req.URL {
		t.Fatalf("expected entry to match request, got %+v", entry)
	}
	if entry.ProfileResults == nil {
		t.Fatalf("expected profile results to be stored")
	}
	if entry.ProfileResults.TotalRuns != st.total {
		t.Fatalf("expected total runs %d, got %d", st.total, entry.ProfileResults.TotalRuns)
	}
	if entry.ProfileResults.Latency == nil {
		t.Fatalf("expected latency stats to be stored")
	}
	if entry.Status != resp.Status || entry.StatusCode != resp.StatusCode {
		t.Fatalf(
			"expected status %s (%d), got %s (%d)",
			resp.Status,
			resp.StatusCode,
			entry.Status,
			entry.StatusCode,
		)
	}
	if strings.TrimSpace(entry.RequestText) == "" {
		t.Fatalf("expected request text to be recorded")
	}
	if entry.BodySnippet != "<profile run – see profileResults>" {
		t.Fatalf("expected profile snippet placeholder, got %q", entry.BodySnippet)
	}
}

func TestRecordProfileHistorySkipsNoLog(t *testing.T) {
	dir := t.TempDir()
	store := history.NewStore(filepath.Join(dir, "history.json"), 10)
	model := New(Config{History: store})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			NoLog: true,
		},
	}
	st := &profileState{base: req, total: 1, successes: []time.Duration{time.Millisecond}}
	stats := analysis.ComputeLatencyStats(st.successes, []int{}, 1)
	msg := responseMsg{response: &httpclient.Response{Status: "200 OK", StatusCode: 200}}
	model.recordProfileHistory(st, stats, msg, "")
	if entries := store.Entries(); len(entries) != 0 {
		t.Fatalf("expected no history entries when no-log set, got %d", len(entries))
	}
}
