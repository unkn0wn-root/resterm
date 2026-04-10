package headless

import (
	"errors"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"google.golang.org/grpc/codes"
)

func TestReportFromRunnerCounts(t *testing.T) {
	src := &runner.Report{
		Version:   "v1",
		FilePath:  "api.http",
		EnvName:   "dev",
		StartedAt: time.Unix(10, 0),
		EndedAt:   time.Unix(12, 0),
		Duration:  2 * time.Second,
		Total:     4,
		Passed:    2,
		Failed:    1,
		Skipped:   1,
		Results: []runner.Result{
			{
				Kind:        runner.ResultKindRequest,
				Name:        "ok",
				Method:      "GET",
				Target:      "https://example.com",
				Environment: "dev",
				Passed:      true,
				Response: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Proto:      "HTTP/1.1",
					Duration:   25 * time.Millisecond,
				},
				Tests: []scripts.TestResult{{Name: "status", Passed: true, Elapsed: time.Millisecond}},
				Stream: &runner.StreamInfo{
					Kind:       "sse",
					EventCount: 1,
					Summary: map[string]any{
						"meta": map[string]any{"id": "1"},
					},
				},
			},
			{
				Kind:       runner.ResultKindWorkflow,
				Name:       "wf",
				Method:     "WORKFLOW",
				Passed:     false,
				Canceled:   true,
				Duration:   time.Second,
				Summary:    "canceled",
				SkipReason: "",
				Steps: []runner.StepResult{{
					Name:       "Login",
					Method:     "GET",
					Target:     "/login",
					Canceled:   true,
					Summary:    "stop",
					Passed:     false,
					Duration:   10 * time.Millisecond,
					Response:   &httpclient.Response{Status: "200 OK", StatusCode: 200, Proto: "HTTP/1.1"},
					Tests:      []scripts.TestResult{{Name: "a", Passed: true}},
					Trace:      &runner.TraceInfo{Summary: &history.TraceSummary{}},
					SkipReason: "",
				}},
			},
			{
				Kind:       runner.ResultKindCompare,
				Name:       "cmp",
				Method:     "COMPARE",
				Passed:     true,
				Duration:   50 * time.Millisecond,
				Compare:    &runner.CompareInfo{Baseline: "stage"},
				Steps:      []runner.StepResult{{Name: "dev", Environment: "dev", Passed: true}, {Name: "stage", Environment: "stage", Passed: true}},
				Summary:    "ok",
				Canceled:   false,
				SkipReason: "",
			},
			{
				Kind:       runner.ResultKindProfile,
				Name:       "prof",
				Method:     "PROFILE",
				Skipped:    true,
				SkipReason: "not selected",
				Profile:    &runner.ProfileInfo{Count: 3, Warmup: 1},
			},
		},
	}

	got := reportFromRunner(src)
	if got == nil {
		t.Fatal("expected mapped report")
	}
	if got.Total != 4 || got.Passed != 2 || got.Failed != 1 || got.Skipped != 1 {
		t.Fatalf("unexpected counts: %+v", got)
	}
	if len(got.Results) != 4 {
		t.Fatalf("expected four results, got %+v", got.Results)
	}
	if got.Results[0].Kind != KindRequest || got.Results[0].Status != StatusPass {
		t.Fatalf("unexpected request mapping: %+v", got.Results[0])
	}
	if got.Results[1].Kind != KindWorkflow || got.Results[1].Status != StatusFail || !got.Results[1].Canceled {
		t.Fatalf("unexpected workflow mapping: %+v", got.Results[1])
	}
	if got.Results[2].Kind != KindCompare || got.Results[2].Status != StatusPass {
		t.Fatalf("unexpected compare mapping: %+v", got.Results[2])
	}
	if got.Results[3].Kind != KindProfile || got.Results[3].Status != StatusSkip {
		t.Fatalf("unexpected profile mapping: %+v", got.Results[3])
	}
}

func TestReportFromRunnerDetails(t *testing.T) {
	src := &runner.Report{
		Results: []runner.Result{
			{
				Kind:    runner.ResultKindCompare,
				Name:    "cmp",
				Method:  "COMPARE",
				Passed:  true,
				Compare: &runner.CompareInfo{Baseline: "stage"},
				Steps:   []runner.StepResult{{Name: "dev", Environment: "dev", Passed: true}, {Name: "stage", Environment: "stage", Passed: true}},
			},
			{
				Kind:     runner.ResultKindForEach,
				Name:     "loop",
				Method:   "FOR-EACH",
				Passed:   true,
				Duration: time.Second,
				Steps: []runner.StepResult{{
					Name:      "step",
					Method:    "POST",
					Target:    "/items",
					Branch:    "if-true",
					Iteration: 2,
					Total:     3,
					Passed:    true,
					Duration:  15 * time.Millisecond,
					GRPC:      &grpcclient.Response{StatusCode: codes.OK, StatusMessage: "ok", Duration: 15 * time.Millisecond},
					Tests:     []scripts.TestResult{{Name: "ok", Passed: true}},
					Stream:    &runner.StreamInfo{Kind: "ws", EventCount: 2},
					Trace:     &runner.TraceInfo{Summary: &history.TraceSummary{Duration: 15 * time.Millisecond}},
				}},
			},
			{
				Kind:   runner.ResultKindProfile,
				Name:   "prof",
				Method: "PROFILE",
				Passed: true,
				Profile: &runner.ProfileInfo{
					Count:  4,
					Warmup: 1,
					Delay:  time.Second,
					Results: &history.ProfileResults{
						TotalRuns:      4,
						WarmupRuns:     1,
						SuccessfulRuns: 3,
						FailedRuns:     1,
						Latency: &history.ProfileLatency{
							Count:  3,
							Min:    time.Millisecond,
							Max:    3 * time.Millisecond,
							Mean:   2 * time.Millisecond,
							Median: 2 * time.Millisecond,
							StdDev: time.Millisecond,
						},
						Percentiles: []history.ProfilePercentile{{Percentile: 95, Value: 3 * time.Millisecond}},
						Histogram:   []history.ProfileHistogramBin{{From: time.Millisecond, To: 3 * time.Millisecond, Count: 3}},
					},
					Failures: []runner.ProfileFailure{{
						Iteration:  3,
						Warmup:     false,
						Reason:     "boom",
						Status:     "500",
						StatusCode: 500,
						Duration:   4 * time.Millisecond,
					}},
				},
			},
		},
	}

	got := reportFromRunner(src)
	if got == nil || len(got.Results) != 3 {
		t.Fatalf("unexpected report mapping: %+v", got)
	}
	cmp := got.Results[0]
	if cmp.Compare == nil || cmp.Compare.Baseline != "stage" {
		t.Fatalf("unexpected compare mapping: %+v", cmp)
	}
	if len(cmp.Steps) != 2 || cmp.Steps[0].Environment != "dev" || cmp.Steps[1].Environment != "stage" {
		t.Fatalf("unexpected compare steps: %+v", cmp.Steps)
	}

	loop := got.Results[1]
	if loop.Kind != KindForEach || len(loop.Steps) != 1 {
		t.Fatalf("unexpected for-each mapping: %+v", loop)
	}
	step := loop.Steps[0]
	if step.Branch != "if-true" || step.Iteration != 2 || step.Total != 3 {
		t.Fatalf("unexpected workflow step mapping: %+v", step)
	}
	if step.GRPC == nil || step.GRPC.Code != codes.OK.String() || step.Stream == nil || step.Trace == nil {
		t.Fatalf("unexpected workflow protocol mapping: %+v", step)
	}

	prof := got.Results[2]
	if prof.Profile == nil {
		t.Fatalf("expected profile mapping: %+v", prof)
	}
	if prof.Profile.TotalRuns != 4 || prof.Profile.WarmupRuns != 1 || prof.Profile.FailedRuns != 1 {
		t.Fatalf("unexpected profile totals: %+v", prof.Profile)
	}
	if prof.Profile.Latency == nil || prof.Profile.Latency.Max != 3*time.Millisecond {
		t.Fatalf("unexpected profile latency: %+v", prof.Profile)
	}
	if len(prof.Profile.Percentiles) != 1 || len(prof.Profile.Histogram) != 1 || len(prof.Profile.Failures) != 1 {
		t.Fatalf("unexpected profile detail mapping: %+v", prof.Profile)
	}
}

func TestReportFromRunnerClones(t *testing.T) {
	src := &runner.Report{
		Results: []runner.Result{{
			Kind:   runner.ResultKindRequest,
			Name:   "req",
			Method: "GET",
			Passed: false,
			Err:    errors.New("boom"),
			Tests: []scripts.TestResult{{
				Name:    "status",
				Message: "bad",
				Passed:  false,
				Elapsed: time.Millisecond,
			}},
			Stream: &runner.StreamInfo{
				Kind:           "sse",
				EventCount:     1,
				TranscriptPath: "stream.json",
				Summary: map[string]any{
					"meta": map[string]any{"id": "1"},
					"list": []any{"a"},
				},
			},
			Trace: &runner.TraceInfo{
				ArtifactPath: "trace.json",
				Summary: &history.TraceSummary{
					Duration: time.Second,
					Budgets: &history.TraceBudget{
						Total:     2 * time.Second,
						Tolerance: 10 * time.Millisecond,
						Phases:    map[string]time.Duration{"dns": time.Millisecond},
					},
					Breaches: []history.TraceBreach{{
						Kind:   "dns",
						Limit:  time.Millisecond,
						Actual: 2 * time.Millisecond,
						Over:   time.Millisecond,
					}},
				},
			},
		}},
	}

	got := reportFromRunner(src)
	if got == nil || len(got.Results) != 1 {
		t.Fatalf("unexpected mapped report: %+v", got)
	}

	src.Results[0].Stream.Summary["meta"].(map[string]any)["id"] = "2"
	src.Results[0].Stream.Summary["list"].([]any)[0] = "b"
	src.Results[0].Trace.Summary.Budgets.Phases["dns"] = 2 * time.Millisecond
	src.Results[0].Tests[0].Name = "mutated"

	item := got.Results[0]
	if item.Tests[0].Name != "status" {
		t.Fatalf("expected tests to be cloned, got %+v", item.Tests)
	}
	if item.Stream == nil || item.Stream.Summary["meta"].(map[string]any)["id"] != "1" {
		t.Fatalf("expected stream summary clone, got %+v", item.Stream)
	}
	if item.Stream.Summary["list"].([]any)[0] != "a" {
		t.Fatalf("expected nested slice clone, got %+v", item.Stream.Summary)
	}
	if item.Trace == nil || item.Trace.Budget == nil || item.Trace.Budget.Phases["dns"] != time.Millisecond {
		t.Fatalf("expected trace budget clone, got %+v", item.Trace)
	}
}

func TestReportFromRunnerStrings(t *testing.T) {
	src := &runner.Report{
		Version: "  v1  ",
		EnvName: "  dev  ",
		Results: []runner.Result{{
			Kind:        runner.ResultKindRequest,
			Name:        "  req  ",
			Method:      "  GET  ",
			Target:      "  https://example.com  ",
			Environment: "  dev  ",
			Summary:     "  ok  ",
			SkipReason:  "  skipped  ",
			Err:         errors.New("  boom  "),
			ScriptErr:   errors.New("  script boom  "),
			Response: &httpclient.Response{
				Status: " 200 OK ",
				Proto:  " HTTP/1.1 ",
			},
			Tests: []scripts.TestResult{{
				Name:    "  status  ",
				Message: "  failed  ",
			}},
			Compare: &runner.CompareInfo{Baseline: "  stage  "},
			Profile: &runner.ProfileInfo{
				Failures: []runner.ProfileFailure{{
					Reason: "  flaky  ",
					Status: "  500  ",
				}},
			},
			Stream: &runner.StreamInfo{
				Kind:           "  sse  ",
				TranscriptPath: "  stream.log  ",
			},
			Trace: &runner.TraceInfo{
				ArtifactPath: "  trace.json  ",
				Summary: &history.TraceSummary{
					Error: "  timeout  ",
					Breaches: []history.TraceBreach{{
						Kind: "  total  ",
					}},
				},
			},
			Steps: []runner.StepResult{{
				Name:        "  step  ",
				Method:      "  POST  ",
				Target:      "  /items  ",
				Environment: "  stage  ",
				Branch:      "  branch-a  ",
				Summary:     "  retry  ",
				SkipReason:  "  guard  ",
				Err:         errors.New("  step boom  "),
				ScriptErr:   errors.New("  step script  "),
				GRPC: &grpcclient.Response{
					StatusCode:    codes.Internal,
					StatusMessage: "  internal  ",
				},
				Tests: []scripts.TestResult{{
					Name:    "  step test  ",
					Message: "  bad  ",
				}},
				Stream: &runner.StreamInfo{
					Kind:           "  ws  ",
					TranscriptPath: "  step.log  ",
				},
				Trace: &runner.TraceInfo{
					ArtifactPath: "  step-trace.json  ",
					Summary: &history.TraceSummary{
						Error: "  deadline  ",
						Breaches: []history.TraceBreach{{
							Kind: "  dns  ",
						}},
					},
				},
			}},
		}},
	}

	got := reportFromRunner(src)
	if got == nil || len(got.Results) != 1 {
		t.Fatalf("unexpected mapped report: %+v", got)
	}
	if got.Version != "v1" || got.EnvName != "dev" {
		t.Fatalf("expected top-level strings to be trimmed, got %+v", got)
	}

	item := got.Results[0]
	if item.Name != "req" || item.Method != "GET" || item.Target != "https://example.com" || item.Environment != "dev" {
		t.Fatalf("expected result identity strings to be trimmed, got %+v", item)
	}
	if item.Summary != "ok" || item.SkipReason != "skipped" {
		t.Fatalf("expected result status strings to be trimmed, got %+v", item)
	}
	if item.Error != "  boom  " || item.ScriptError != "  script boom  " {
		t.Fatalf("expected result errors to be preserved, got %+v", item)
	}
	if item.HTTP == nil || item.HTTP.Status != "200 OK" || item.HTTP.Protocol != "HTTP/1.1" {
		t.Fatalf("expected http strings to be trimmed, got %+v", item.HTTP)
	}
	if len(item.Tests) != 1 || item.Tests[0].Name != "status" || item.Tests[0].Message != "failed" {
		t.Fatalf("expected tests to be trimmed, got %+v", item.Tests)
	}
	if item.Compare == nil || item.Compare.Baseline != "stage" {
		t.Fatalf("expected compare to be trimmed, got %+v", item.Compare)
	}
	if item.Profile == nil || len(item.Profile.Failures) != 1 || item.Profile.Failures[0].Reason != "flaky" || item.Profile.Failures[0].Status != "500" {
		t.Fatalf("expected profile failures to be trimmed, got %+v", item.Profile)
	}
	if item.Stream == nil || item.Stream.Kind != "sse" || item.Stream.TranscriptPath != "stream.log" {
		t.Fatalf("expected stream strings to be trimmed, got %+v", item.Stream)
	}
	if item.Trace == nil || item.Trace.Error != "timeout" || item.Trace.ArtifactPath != "trace.json" || len(item.Trace.Breaches) != 1 || item.Trace.Breaches[0].Kind != "total" {
		t.Fatalf("expected trace strings to be trimmed, got %+v", item.Trace)
	}

	if len(item.Steps) != 1 {
		t.Fatalf("expected one step, got %+v", item.Steps)
	}
	step := item.Steps[0]
	if step.Name != "step" || step.Method != "POST" || step.Target != "/items" || step.Environment != "stage" || step.Branch != "branch-a" {
		t.Fatalf("expected step identity strings to be trimmed, got %+v", step)
	}
	if step.Summary != "retry" || step.SkipReason != "guard" {
		t.Fatalf("expected step status strings to be trimmed, got %+v", step)
	}
	if step.Error != "  step boom  " || step.ScriptError != "  step script  " {
		t.Fatalf("expected step errors to be preserved, got %+v", step)
	}
	if step.GRPC == nil || step.GRPC.Code != codes.Internal.String() || step.GRPC.StatusMessage != "internal" {
		t.Fatalf("expected grpc strings to be trimmed, got %+v", step.GRPC)
	}
	if len(step.Tests) != 1 || step.Tests[0].Name != "step test" || step.Tests[0].Message != "bad" {
		t.Fatalf("expected step tests to be trimmed, got %+v", step.Tests)
	}
	if step.Stream == nil || step.Stream.Kind != "ws" || step.Stream.TranscriptPath != "step.log" {
		t.Fatalf("expected step stream strings to be trimmed, got %+v", step.Stream)
	}
	if step.Trace == nil || step.Trace.Error != "deadline" || step.Trace.ArtifactPath != "step-trace.json" || len(step.Trace.Breaches) != 1 || step.Trace.Breaches[0].Kind != "dns" {
		t.Fatalf("expected step trace strings to be trimmed, got %+v", step.Trace)
	}
}
