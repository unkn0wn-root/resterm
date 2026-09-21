package core

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestProfileHistoryRoundTrip(t *testing.T) {
	iters := []engine.RequestResult{
		{Err: errors.New("dial secret-host: refused")},
		proOK(10 * time.Millisecond),
		proHTTP500(),
		proOK(30 * time.Millisecond),
	}
	snap := feedProfile(t, restfile.ProfileSpec{Count: 3, Warmup: 1}, iters, false, RunDone{}).Snapshot()
	var masked int
	redact := func(s string, mask bool) string {
		if mask {
			masked++
		}
		return strings.ReplaceAll(s, "secret-host", "***")
	}
	req := &restfile.Request{
		Method:   "GET",
		URL:      "https://example.com",
		Metadata: restfile.RequestMetadata{Name: "prof"},
	}

	raw, err := json.Marshal(snap.HistoryEntry(req, testEnvironment("dev"), "prof.http", redact))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var ent history.Entry
	if err := json.Unmarshal(raw, &ent); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if ent.Status != "FAIL 2/3" || ent.RequestName != "prof" || ent.Duration != snap.Progress.Elapsed || masked != 1 {
		t.Fatalf("entry = %+v, masked = %d", ent, masked)
	}
	rec := ent.ProfileResults
	if rec.FailedRuns != 1 || rec.WarmupFailedRuns != 1 || rec.Status != "fail" || rec.Count != 3 {
		t.Fatalf("record = %+v", rec)
	}
	if got := rec.Failures[0].Reason; strings.Contains(got, "secret-host") || !rec.Failures[0].Warmup {
		t.Fatalf("warmup failure = %+v, want redacted", rec.Failures[0])
	}

	got := ProfileSnapshotFromHistory(ent)
	want := snap.Progress
	want.Total = want.Done
	if got.Progress != want {
		t.Fatalf("progress = %+v, want %+v", got.Progress, want)
	}
	if got.Legacy || got.Window != snap.Window || got.Active != snap.Active {
		t.Fatalf("snapshot = %+v", got)
	}
	if !reflect.DeepEqual(got.Stats, snap.Stats) {
		t.Fatalf("stats = %+v, want %+v", got.Stats, snap.Stats)
	}
	if len(got.Failures) != 2 || got.Failures[1].StatusCode != 500 {
		t.Fatalf("failures = %+v", got.Failures)
	}
}

func TestProfileHistoryCapsFailures(t *testing.T) {
	iters := make([]engine.RequestResult, 30)
	for i := range iters {
		iters[i] = proHTTP500()
	}
	snap := feedProfile(t, restfile.ProfileSpec{Count: 30}, iters, false, RunDone{}).Snapshot()
	rec := snap.historyResults(func(s string) string { return s })
	if rec.FailedRuns != 30 || len(rec.Failures) != maxProfileHistoryFailures {
		t.Fatalf("failed = %d, stored = %d", rec.FailedRuns, len(rec.Failures))
	}
	if rec.Latency != nil || rec.Percentiles != nil || rec.Histogram != nil {
		t.Fatalf("record without samples = %+v", rec)
	}
}

func TestProfileSnapshotFromLegacyHistory(t *testing.T) {
	var rec history.ProfileResults
	err := json.Unmarshal([]byte(`{"totalRuns":12,"warmupRuns":2,"successfulRuns":9,"failedRuns":1}`), &rec)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	snap := ProfileSnapshotFromHistory(history.Entry{ProfileResults: &rec})
	if !snap.Legacy || snap.Progress.Status != ProfileFail || snap.Progress.Count != 10 {
		t.Fatalf("legacy snapshot = %+v", snap)
	}
}

func TestProfileHistoryStatus(t *testing.T) {
	tests := []struct {
		prog ProfileProgress
		want string
	}{
		{ProfileProgress{Status: ProfilePass, Passed: 10, Count: 10}, "PASS 10/10"},
		{ProfileProgress{Status: ProfileFail, Passed: 7, Count: 10}, "FAIL 7/10"},
		{ProfileProgress{Status: ProfileCanceled, Passed: 3, Count: 10}, "CANCELED 3/10"},
		{ProfileProgress{Status: ProfileSkipped}, "SKIPPED"},
		{ProfileProgress{Status: ProfileError}, "ERROR"},
	}
	for _, test := range tests {
		if got := (ProfileSnapshot{Progress: test.prog}).historyStatus(); got != test.want {
			t.Fatalf("historyStatus() = %q, want %q", got, test.want)
		}
	}
}
