package core

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRunProfileEmitsWarmupAndMeasuredIterations(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Name: "profile",
			Profile: &restfile.ProfileSpec{
				Count:  2,
				Warmup: 1,
			},
		},
	}
	pl, err := PrepareProfile(&restfile.Document{Path: "pro.http"}, req, RunMeta{
		ID:  "pro-1",
		Env: "dev",
	})
	if err != nil {
		t.Fatalf("PrepareProfile: %v", err)
	}

	dep := &proDep{
		res: []engine.RequestResult{
			{Response: &httpclient.Response{Status: "200 OK", StatusCode: http.StatusOK, Duration: 3 * time.Millisecond}},
			{Response: &httpclient.Response{Status: "200 OK", StatusCode: http.StatusOK, Duration: 4 * time.Millisecond}},
			{Response: &httpclient.Response{Status: "200 OK", StatusCode: http.StatusOK, Duration: 5 * time.Millisecond}},
		},
	}

	var got []string
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case RunStart:
			got = append(got, "run-start:"+v.Meta.Run.Mode.String())
		case ProIterStart:
			got = append(got, "iter-start:"+iterName(v.Iter))
		case ProIterDone:
			got = append(got, "iter-done:"+iterName(v.Iter))
		case RunDone:
			got = append(got, "run-done:"+v.Meta.Run.Mode.String())
			if !v.Success {
				t.Fatalf("expected profile success, got %+v", v)
			}
		default:
			t.Fatalf("unexpected event %T", e)
		}
		return nil
	})

	if err := RunProfile(context.Background(), dep, sink, pl); err != nil {
		t.Fatalf("RunProfile: %v", err)
	}

	want := []string{
		"run-start:profile",
		"iter-start:warmup-1/1",
		"iter-done:warmup-1/1",
		"iter-start:run-1/2",
		"iter-done:run-1/2",
		"iter-start:run-2/2",
		"iter-done:run-2/2",
		"run-done:profile",
	}
	if len(got) != len(want) {
		t.Fatalf("events: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: got %q want %q", i, got[i], want[i])
		}
	}
	if len(dep.call) != 3 {
		t.Fatalf("expected 3 profile calls, got %v", dep.call)
	}
	for i, call := range dep.call {
		if call {
			t.Fatalf("expected profile call %d to skip history recording", i)
		}
	}
}

func TestRunProfileStopsOnSkippedIteration(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Name: "profile",
			Profile: &restfile.ProfileSpec{
				Count: 2,
			},
		},
	}
	pl, err := PrepareProfile(nil, req, RunMeta{ID: "pro-2", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareProfile: %v", err)
	}

	dep := &proDep{
		res: []engine.RequestResult{
			{Skipped: true, SkipReason: "condition false"},
			{Response: &httpclient.Response{Status: "200 OK", StatusCode: http.StatusOK}},
		},
	}

	var got []string
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case ProIterStart:
			got = append(got, "start:"+strconv.Itoa(v.Iter.Index))
		case ProIterDone:
			got = append(got, "done:"+strconv.Itoa(v.Iter.Index))
		case RunDone:
			got = append(got, "run-done")
			if !v.Skipped || v.Success || v.Canceled {
				t.Fatalf("expected skipped profile run, got %+v", v)
			}
		}
		return nil
	})

	if err := RunProfile(context.Background(), dep, sink, pl); err != nil {
		t.Fatalf("RunProfile: %v", err)
	}

	want := []string{"start:0", "done:0", "run-done"}
	if len(got) != len(want) {
		t.Fatalf("events: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: got %q want %q", i, got[i], want[i])
		}
	}
	if len(dep.call) != 1 {
		t.Fatalf("expected one profile call after skip, got %v", dep.call)
	}
}

func TestRunProfileCancelDuringDelayEmitsRunDone(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Name: "profile",
			Profile: &restfile.ProfileSpec{
				Count: 2,
				Delay: time.Hour,
			},
		},
	}
	pl, err := PrepareProfile(nil, req, RunMeta{ID: "pro-3", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareProfile: %v", err)
	}

	dep := &proDep{
		res: []engine.RequestResult{
			{Response: &httpclient.Response{Status: "200 OK", StatusCode: http.StatusOK, Duration: time.Millisecond}},
			{Response: &httpclient.Response{Status: "200 OK", StatusCode: http.StatusOK, Duration: time.Millisecond}},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var got []string
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case ProIterStart:
			got = append(got, "start:"+strconv.Itoa(v.Iter.Index))
		case ProIterDone:
			got = append(got, "done:"+strconv.Itoa(v.Iter.Index))
			if v.Iter.Index == 0 {
				cancel()
			}
		case RunDone:
			got = append(got, "run-done")
			if !v.Canceled || v.Success {
				t.Fatalf("expected canceled profile run, got %+v", v)
			}
		}
		return nil
	})

	if err := RunProfile(ctx, dep, sink, pl); err != nil {
		t.Fatalf("RunProfile: %v", err)
	}

	want := []string{"start:0", "done:0", "run-done"}
	if len(got) != len(want) {
		t.Fatalf("events: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: got %q want %q", i, got[i], want[i])
		}
	}
	if len(dep.call) != 1 {
		t.Fatalf("expected cancel during delay to stop before second run, got %v", dep.call)
	}
}

type proDep struct {
	fakeDep
	res  []engine.RequestResult
	call []bool
}

func (d *proDep) ExecuteWith(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	opt request.ExecOptions,
) (engine.RequestResult, error) {
	d.call = append(d.call, opt.Record)
	if len(d.res) == 0 {
		return engine.RequestResult{}, nil
	}
	out := d.res[0]
	d.res = d.res[1:]
	out.Executed = request.CloneRequest(req)
	out.RequestText = request.RenderRequestText(req)
	out.Environment = env
	return out, nil
}

func iterName(it IterMeta) string {
	if it.Warmup {
		return "warmup-" + strconv.Itoa(it.WarmupIndex) + "/" + strconv.Itoa(it.WarmupTotal)
	}
	return "run-" + strconv.Itoa(it.RunIndex) + "/" + strconv.Itoa(it.RunTotal)
}
