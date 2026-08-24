package headless

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestEvaluateStepStatusCode(t *testing.T) {
	code := 404
	res := wfStepRes{
		step: restfile.WorkflowStep{
			Expect: restfile.WorkflowExpect{StatusCode: &code},
		},
		dur: 1500 * time.Millisecond,
		http: &httpx.Response{
			Status:     http.StatusText(http.StatusNotFound),
			StatusCode: http.StatusNotFound,
			Duration:   400 * time.Millisecond,
		},
	}

	got := evaluateStep(res)
	if !got.ok {
		t.Fatalf("expected step to pass, got %+v", got)
	}
	if got.msg != "" {
		t.Fatalf("expected empty message, got %q", got.msg)
	}
	if got.status != http.StatusText(http.StatusNotFound) {
		t.Fatalf("expected status %q, got %q", http.StatusText(http.StatusNotFound), got.status)
	}
	if got.dur != 1500*time.Millisecond {
		t.Fatalf("expected duration %s, got %s", 1500*time.Millisecond, got.dur)
	}
}

func TestWorkflowSummaryNamesOnlyATrailingFailure(t *testing.T) {
	tests := []struct {
		name string
		res  []wfStepRes
		want string
	}{
		{
			name: "continued past the failure",
			res: []wfStepRes{
				{name: "Seed", ok: true},
				{name: "Failure", msg: "unexpected status code 500"},
				{name: "Check", ok: true},
			},
			want: "Workflow Atomic finished with 1 failure(s)",
		},
		{
			name: "stopped at the failure",
			res: []wfStepRes{
				{name: "Seed", ok: true},
				{name: "Failure", msg: "unexpected status code 500"},
			},
			want: "Workflow Atomic failed at step Failure: unexpected status code 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &wfState{wf: restfile.Workflow{Name: "Atomic"}, res: tt.res}
			if got := workflowSummary(st); got != tt.want {
				t.Fatalf("workflowSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEvaluateStepScriptErr(t *testing.T) {
	res := wfStepRes{
		dur: 250 * time.Millisecond,
		http: &httpx.Response{
			Status:     http.StatusText(http.StatusOK),
			StatusCode: http.StatusOK,
			Duration:   80 * time.Millisecond,
		},
		sErr: errors.New("script crashed"),
	}

	got := evaluateStep(res)
	if got.ok {
		t.Fatalf("expected step to fail, got %+v", got)
	}
	if got.msg != "script crashed" {
		t.Fatalf("expected script error message, got %q", got.msg)
	}
	if got.status != http.StatusText(http.StatusOK) {
		t.Fatalf("expected transport status to be preserved, got %q", got.status)
	}
	if got.dur != 250*time.Millisecond {
		t.Fatalf("expected duration %s, got %s", 250*time.Millisecond, got.dur)
	}
}

func TestMakeStepResReportsLogicalDuration(t *testing.T) {
	res := makeStepRes(
		restfile.WorkflowStep{},
		&restfile.Request{Method: "GET", URL: "https://example.com/jobs/1"},
		engine.RequestResult{
			Response: &httpx.Response{
				Status:     http.StatusText(http.StatusOK),
				StatusCode: http.StatusOK,
				Duration:   30 * time.Millisecond,
			},
			Timing: engine.Timing{Total: 4 * time.Second},
		},
		"",
		0,
		0,
	)
	if res.dur != 4*time.Second {
		t.Fatalf("step duration = %s, want the whole request", res.dur)
	}
}

func TestMakeStepResSkippedHasNoDuration(t *testing.T) {
	res := makeStepRes(
		restfile.WorkflowStep{},
		&restfile.Request{Method: "GET", URL: "https://example.com/jobs/1"},
		engine.RequestResult{
			Skipped:    true,
			SkipReason: "condition was false",
			Timing:     engine.Timing{Total: 2 * time.Millisecond},
		},
		"",
		0,
		0,
	)
	if res.dur != 0 {
		t.Fatalf("skipped step duration = %s, want none", res.dur)
	}
}
