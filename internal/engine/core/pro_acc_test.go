package core

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

var proT0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func proOK(d time.Duration) engine.RequestResult {
	return engine.RequestResult{Response: &httpx.Response{Status: "200 OK", StatusCode: http.StatusOK, Duration: d}}
}

func proHTTP500() engine.RequestResult {
	return engine.RequestResult{
		Response: &httpx.Response{
			Status:     "500 Internal Server Error",
			StatusCode: http.StatusInternalServerError,
			Duration:   10 * time.Millisecond,
		},
	}
}

func feedProfile(
	t *testing.T,
	spec restfile.ProfileSpec,
	iters []engine.RequestResult,
	open bool,
	done RunDone,
) *ProfileAccumulator {
	t.Helper()
	req := &restfile.Request{
		Method:   "GET",
		URL:      "https://example.com",
		Metadata: restfile.RequestMetadata{Profile: &spec},
	}
	pl, err := PrepareProfile(nil, req, RunMeta{ID: "acc"})
	if err != nil {
		t.Fatalf("PrepareProfile: %v", err)
	}
	acc := NewProfileAccumulator(pl)
	at := proT0
	emit := acc.Apply
	emit(RunStart{Meta: EvtMeta{At: at}})
	iter := func(i int) IterMeta {
		return IterMeta{Index: i, Warmup: i < spec.Warmup}
	}
	for i, res := range iters {
		emit(ProIterStart{Meta: EvtMeta{At: at}, Iter: iter(i)})
		dur := 10 * time.Millisecond
		if res.Response != nil {
			dur = res.Response.Duration
		}
		at = at.Add(dur)
		emit(ProIterDone{Meta: EvtMeta{At: at}, Iter: iter(i), Result: res})
		at = at.Add(spec.Delay)
	}
	if open {
		emit(ProIterStart{Meta: EvtMeta{At: at}, Iter: iter(len(iters))})
		at = at.Add(time.Millisecond)
	}
	done.Meta.At = at
	emit(done)
	return acc
}

func TestProfileAccumulator(t *testing.T) {
	canceled := engine.RequestResult{Err: context.Canceled}
	tests := []struct {
		name   string
		spec   restfile.ProfileSpec
		iters  []engine.RequestResult
		open   bool
		done   RunDone
		want   ProfileProgress
		window time.Duration
	}{
		{
			name: "all pass",
			spec: restfile.ProfileSpec{Count: 3},
			iters: []engine.RequestResult{
				proOK(10 * time.Millisecond),
				proOK(20 * time.Millisecond),
				proOK(30 * time.Millisecond),
			},
			want:   ProfileProgress{Status: ProfilePass, Total: 3, Done: 3, Count: 3, Measured: 3, Passed: 3},
			window: 60 * time.Millisecond,
		},
		{
			name:  "measured failure",
			spec:  restfile.ProfileSpec{Count: 2},
			iters: []engine.RequestResult{proOK(10 * time.Millisecond), proHTTP500()},
			want: ProfileProgress{
				Status:   ProfileFail,
				Total:    2,
				Done:     2,
				Count:    2,
				Measured: 2,
				Passed:   1,
				Failed:   1,
			},
			window: 20 * time.Millisecond,
		},
		{
			name:  "warmup failure does not fail",
			spec:  restfile.ProfileSpec{Count: 2, Warmup: 1},
			iters: []engine.RequestResult{proHTTP500(), proOK(10 * time.Millisecond), proOK(10 * time.Millisecond)},
			want: ProfileProgress{
				Status: ProfilePass, Total: 3, Done: 3, Warmup: 1, WarmupDone: 1, WarmupFailed: 1,
				Count: 2, Measured: 2, Passed: 2,
			},
			window: 20 * time.Millisecond,
		},
		{
			name:   "no successful samples",
			spec:   restfile.ProfileSpec{Count: 2},
			iters:  []engine.RequestResult{{Err: errors.New("dial tcp: connection refused")}, proHTTP500()},
			want:   ProfileProgress{Status: ProfileFail, Total: 2, Done: 2, Count: 2, Measured: 2, Failed: 2},
			window: 20 * time.Millisecond,
		},
		{
			name:  "skipped",
			spec:  restfile.ProfileSpec{Count: 2},
			iters: []engine.RequestResult{{Skipped: true, SkipReason: "off"}},
			done:  RunDone{Skipped: true},
			want:  ProfileProgress{Status: ProfileSkipped, Total: 2, Count: 2},
		},
		{
			name: "cancel before first iteration",
			spec: restfile.ProfileSpec{Count: 2},
			done: RunDone{Canceled: true},
			want: ProfileProgress{Status: ProfileCanceled, Total: 2, Count: 2},
		},
		{
			name:   "cancel during request",
			spec:   restfile.ProfileSpec{Count: 3},
			iters:  []engine.RequestResult{proOK(10 * time.Millisecond), canceled},
			done:   RunDone{Canceled: true},
			want:   ProfileProgress{Status: ProfileCanceled, Total: 3, Done: 1, Count: 3, Measured: 1, Passed: 1},
			window: 10 * time.Millisecond,
		},
		{
			name:  "cancel on first measured request",
			spec:  restfile.ProfileSpec{Count: 1, Warmup: 1},
			iters: []engine.RequestResult{proOK(10 * time.Millisecond), canceled},
			done:  RunDone{Canceled: true},
			want:  ProfileProgress{Status: ProfileCanceled, Total: 2, Done: 1, Warmup: 1, WarmupDone: 1, Count: 1},
		},
		{
			name:   "cancel during delay",
			spec:   restfile.ProfileSpec{Count: 2, Delay: 5 * time.Millisecond},
			iters:  []engine.RequestResult{proOK(10 * time.Millisecond)},
			done:   RunDone{Canceled: true},
			want:   ProfileProgress{Status: ProfileCanceled, Total: 2, Done: 1, Count: 2, Measured: 1, Passed: 1},
			window: 10 * time.Millisecond,
		},
		{
			name:   "infrastructure error",
			spec:   restfile.ProfileSpec{Count: 2},
			iters:  []engine.RequestResult{proOK(10 * time.Millisecond)},
			open:   true,
			done:   RunDone{Err: errors.New("executor gone")},
			want:   ProfileProgress{Status: ProfileError, Total: 2, Done: 1, Count: 2, Measured: 1, Passed: 1},
			window: 10 * time.Millisecond,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snap := feedProfile(t, test.spec, test.iters, test.open, test.done).Snapshot()
			got := snap.Progress
			got.Elapsed = 0
			if got != test.want {
				t.Fatalf("progress = %+v, want %+v", got, test.want)
			}
			if snap.Window != test.window {
				t.Fatalf("window = %s, want %s", snap.Window, test.window)
			}
			if got.Done != got.WarmupDone+got.Measured || got.Measured != got.Passed+got.Failed {
				t.Fatalf("counts do not add up: %+v", got)
			}
			codes := 0
			for _, n := range snap.StatusCodes {
				codes += n
			}
			if codes != got.Measured {
				t.Fatalf("status codes = %v, want %d measured", snap.StatusCodes, got.Measured)
			}
			if n := len(snap.Failures); n != got.Failed+got.WarmupFailed {
				t.Fatalf("failures = %d, want %d", n, got.Failed+got.WarmupFailed)
			}
			if snap.Stats.Count != got.Passed {
				t.Fatalf("stats count = %d, want %d", snap.Stats.Count, got.Passed)
			}
			if !snap.Started.Equal(proT0) || snap.Ended.Sub(snap.Started) != snap.Progress.Elapsed {
				t.Fatalf("times = %s..%s elapsed %s", snap.Started, snap.Ended, snap.Progress.Elapsed)
			}
		})
	}
}

func TestProfileAccumulatorStats(t *testing.T) {
	spec := restfile.ProfileSpec{Count: 4, Warmup: 1}
	iters := []engine.RequestResult{
		proOK(time.Second),
		proOK(10 * time.Millisecond),
		proOK(20 * time.Millisecond),
		proOK(30 * time.Millisecond),
		proHTTP500(),
	}
	acc := feedProfile(t, spec, iters, false, RunDone{})
	snap := acc.Snapshot()
	if snap.Progress.Status != ProfileFail {
		t.Fatalf("status = %s, want fail", snap.Progress.Status)
	}
	if snap.Stats.Max != 30*time.Millisecond || snap.Stats.Percentiles[50] != 20*time.Millisecond {
		t.Fatalf("stats = %+v, want warmup and failures excluded", snap.Stats)
	}
	if snap.Active != 70*time.Millisecond {
		t.Fatalf("active = %s, want 70ms", snap.Active)
	}
	f := snap.Failures[0]
	if f.Iteration != 5 || f.Warmup || f.StatusCode != http.StatusInternalServerError || f.Failure.Code == "" {
		t.Fatalf("failure = %+v", f)
	}

	snap.Failures[0].Reason = "changed"
	snap.Stats.Percentiles[50] = 0
	again := acc.Snapshot()
	if again.Failures[0].Reason == "changed" || again.Stats.Percentiles[50] == 0 {
		t.Fatal("Snapshot shares state with the accumulator")
	}
}

func TestProfileAccumulatorStatusCodes(t *testing.T) {
	iters := []engine.RequestResult{
		proHTTP500(),
		proOK(10 * time.Millisecond),
		proHTTP500(),
		{Err: errors.New("dial tcp: connection refused")},
		proOK(20 * time.Millisecond),
	}
	acc := feedProfile(t, restfile.ProfileSpec{Count: 4, Warmup: 1}, iters, false, RunDone{})
	snap := acc.Snapshot()
	want := map[int]int{0: 1, http.StatusOK: 2, http.StatusInternalServerError: 1}
	if !maps.Equal(snap.StatusCodes, want) {
		t.Fatalf("status codes = %v, want %v", snap.StatusCodes, want)
	}
	snap.StatusCodes[http.StatusOK] = 0
	if acc.Snapshot().StatusCodes[http.StatusOK] != 2 {
		t.Fatal("Snapshot shares status codes with the accumulator")
	}
}

func TestProfileAccumulatorRunningHasStats(t *testing.T) {
	req := &restfile.Request{Method: "GET", URL: "https://example.com"}
	pl, err := PrepareProfile(nil, req, RunMeta{})
	if err != nil {
		t.Fatalf("PrepareProfile: %v", err)
	}
	acc := NewProfileAccumulator(pl)
	acc.Apply(RunStart{Meta: EvtMeta{At: proT0}})
	acc.Apply(ProIterDone{Meta: EvtMeta{At: proT0.Add(time.Second)}, Result: proOK(time.Second)})
	snap := acc.Snapshot()
	if snap.Progress.Status != ProfileRunning || snap.Progress.Passed != 1 {
		t.Fatalf("running snapshot = %+v", snap)
	}
	if snap.Stats.Count != 1 || snap.Stats.Percentiles[50] != time.Second {
		t.Fatalf("running stats = %+v, want one sample", snap.Stats)
	}
	if snap.Progress.Count != restfile.DefaultProfileCount || snap.Progress.Elapsed != time.Second {
		t.Fatalf("running progress = %+v", snap.Progress)
	}
}

func TestPrepareProfileRejectsGRPC(t *testing.T) {
	req := &restfile.Request{GRPC: &restfile.GRPCRequest{}}
	if _, err := PrepareProfile(nil, req, RunMeta{}); err == nil {
		t.Fatal("PrepareProfile() error = nil")
	}
}
