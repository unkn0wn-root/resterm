package headless

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"google.golang.org/grpc/codes"
)

func TestReportWriteTextParity(t *testing.T) {
	src := sampleRunnerReport()

	var want strings.Builder
	if err := src.WriteText(&want); err != nil {
		t.Fatalf("runner WriteText: %v", err)
	}

	gotRep := reportFromRunner(src)
	var got strings.Builder
	if err := gotRep.WriteText(&got); err != nil {
		t.Fatalf("public WriteText: %v", err)
	}

	if got.String() != want.String() {
		t.Fatalf("text mismatch\nwant:\n%s\ngot:\n%s", want.String(), got.String())
	}
}

func TestReportWriteJSONParity(t *testing.T) {
	src := sampleRunnerReport()

	var want strings.Builder
	if err := src.WriteJSON(&want); err != nil {
		t.Fatalf("runner WriteJSON: %v", err)
	}

	gotRep := reportFromRunner(src)
	var got strings.Builder
	if err := gotRep.WriteJSON(&got); err != nil {
		t.Fatalf("public WriteJSON: %v", err)
	}

	if got.String() != want.String() {
		t.Fatalf("json mismatch\nwant:\n%s\ngot:\n%s", want.String(), got.String())
	}
}

func TestReportWriteJUnitParity(t *testing.T) {
	src := sampleRunnerReport()

	var want strings.Builder
	if err := src.WriteJUnit(&want); err != nil {
		t.Fatalf("runner WriteJUnit: %v", err)
	}

	gotRep := reportFromRunner(src)
	var got strings.Builder
	if err := gotRep.WriteJUnit(&got); err != nil {
		t.Fatalf("public WriteJUnit: %v", err)
	}

	if got.String() != want.String() {
		t.Fatalf("junit mismatch\nwant:\n%s\ngot:\n%s", want.String(), got.String())
	}
}

func TestWriteNilWriter(t *testing.T) {
	rep := reportFromRunner(sampleRunnerReport())
	if rep == nil {
		t.Fatal("expected report")
	}

	if err := rep.WriteText(nil); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("WriteText(nil): got %v want %v", err, ErrNilWriter)
	}
	if err := rep.WriteJSON(nil); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("WriteJSON(nil): got %v want %v", err, ErrNilWriter)
	}
	if err := rep.WriteJUnit(nil); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("WriteJUnit(nil): got %v want %v", err, ErrNilWriter)
	}
}

func TestReportWriteJSONUsesCanonicalStatusValues(t *testing.T) {
	rep := &Report{
		FilePath: "api.http",
		Results: []Result{{
			Name:     "wf",
			Method:   "WORKFLOW",
			Status:   StatusFail,
			Canceled: true,
			Steps: []Step{{
				Name:     "step",
				Status:   StatusFail,
				Canceled: true,
			}},
		}},
	}

	var out strings.Builder
	if err := rep.WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got struct {
		Results []struct {
			Status   string `json:"status"`
			Canceled bool   `json:"canceled"`
			Steps    []struct {
				Status   string `json:"status"`
				Canceled bool   `json:"canceled"`
			} `json:"steps"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Status != "fail" || !got.Results[0].Canceled {
		t.Fatalf("unexpected result json: %+v", got.Results)
	}
	if len(got.Results[0].Steps) != 1 || got.Results[0].Steps[0].Status != "fail" ||
		!got.Results[0].Steps[0].Canceled {
		t.Fatalf("unexpected step json: %+v", got.Results[0].Steps)
	}
}

func sampleRunnerReport() *runner.Report {
	return &runner.Report{
		Version:   "v1",
		FilePath:  "/tmp/api.http",
		EnvName:   "dev",
		StartedAt: time.Unix(10, 0).UTC(),
		EndedAt:   time.Unix(13, 0).UTC(),
		Duration:  3 * time.Second,
		Total:     5,
		Passed:    2,
		Failed:    2,
		Skipped:   1,
		Results: []runner.Result{
			{
				Kind:        runner.ResultKindRequest,
				Name:        "ok",
				Method:      "GET",
				Target:      "https://example.com/ok",
				Environment: "dev",
				Passed:      true,
				Response: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Proto:      "HTTP/1.1",
					Duration:   25 * time.Millisecond,
				},
				Stream: &runner.StreamInfo{
					Kind:       "sse",
					EventCount: 1,
					Summary: map[string]any{
						"wait": 5 * time.Millisecond,
					},
				},
				Trace: &runner.TraceInfo{
					Summary: &history.TraceSummary{
						Duration: 25 * time.Millisecond,
						Budgets: &history.TraceBudget{
							Total:     50 * time.Millisecond,
							Tolerance: 5 * time.Millisecond,
							Phases: map[string]time.Duration{
								"total": 25 * time.Millisecond,
							},
						},
					},
				},
			},
			{
				Kind:       runner.ResultKindRequest,
				Name:       "skip",
				Method:     "POST",
				Target:     "https://example.com/skip",
				Skipped:    true,
				SkipReason: "not selected",
			},
			{
				Kind:     runner.ResultKindWorkflow,
				Name:     "wf",
				Method:   "WORKFLOW",
				Duration: time.Second,
				Canceled: true,
				Summary:  "canceled",
				Passed:   false,
				Steps: []runner.StepResult{
					{
						Name:     "Login",
						Method:   "GET",
						Target:   "/login",
						Passed:   true,
						Duration: 40 * time.Millisecond,
						Response: &httpclient.Response{
							Status:     "200 OK",
							StatusCode: 200,
							Proto:      "HTTP/1.1",
						},
					},
					{
						Name:       "Guard",
						Skipped:    true,
						SkipReason: "guard",
					},
					{
						Name:     "Use",
						Method:   "GRPC",
						Target:   "/demo.Service/Use",
						Canceled: true,
						Summary:  "stop",
						GRPC: &grpcclient.Response{
							StatusCode:    codes.Canceled,
							StatusMessage: "stop",
							Duration:      10 * time.Millisecond,
						},
					},
				},
			},
			{
				Kind:     runner.ResultKindCompare,
				Name:     "cmp",
				Method:   "COMPARE",
				Duration: 50 * time.Millisecond,
				Summary:  "diffs found",
				Compare: &runner.CompareInfo{
					Baseline: "dev",
				},
				Steps: []runner.StepResult{
					{
						Name:        "dev",
						Environment: "dev",
						Passed:      true,
						Duration:    20 * time.Millisecond,
						Response: &httpclient.Response{
							Status:     "200 OK",
							StatusCode: 200,
							Proto:      "HTTP/1.1",
						},
					},
					{
						Name:        "stage",
						Environment: "stage",
						Duration:    30 * time.Millisecond,
						Err:         errors.New("stage failed"),
					},
				},
			},
			{
				Kind:     runner.ResultKindProfile,
				Name:     "prof",
				Method:   "PROFILE",
				Passed:   true,
				Duration: 2 * time.Second,
				Profile: &runner.ProfileInfo{
					Count:  4,
					Warmup: 1,
					Delay:  time.Second,
					Results: &history.ProfileResults{
						TotalRuns:      4,
						WarmupRuns:     1,
						SuccessfulRuns: 3,
						FailedRuns:     0,
						Latency: &history.ProfileLatency{
							Count:  3,
							Min:    time.Millisecond,
							Max:    3 * time.Millisecond,
							Mean:   2 * time.Millisecond,
							Median: 2 * time.Millisecond,
							StdDev: time.Millisecond,
						},
						Percentiles: []history.ProfilePercentile{
							{Percentile: 99, Value: 3 * time.Millisecond},
							{Percentile: 95, Value: 2 * time.Millisecond},
						},
						Histogram: []history.ProfileHistogramBin{
							{From: time.Millisecond, To: 3 * time.Millisecond, Count: 3},
						},
					},
				},
			},
		},
	}
}
