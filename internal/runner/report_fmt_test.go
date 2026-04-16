package runner

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"google.golang.org/grpc/codes"
)

func TestNormalizeReportTrimsStrings(t *testing.T) {
	rep := &Report{
		Version: "  v1  ",
		EnvName: "  dev  ",
		Results: []Result{{
			Kind:        ResultKindCompare,
			Name:        "  cmp  ",
			Method:      "  compare  ",
			Target:      "  https://example.com  ",
			Environment: "  dev  ",
			Summary:     "  ok  ",
			Passed:      true,
			SkipReason:  "  skipped  ",
			Response: &httpclient.Response{
				Status: "  200 OK  ",
				Proto:  "  HTTP/1.1  ",
			},
			Tests: []scripts.TestResult{{
				Name:    "  status  ",
				Message: "  ok  ",
				Passed:  true,
			}},
			Compare: &CompareInfo{Baseline: "  stage  "},
			Profile: &ProfileInfo{
				Failures: []ProfileFailure{{
					Reason: "  boom  ",
					Status: "  500  ",
				}},
			},
			Stream: &StreamInfo{
				Kind:           "  ws  ",
				TranscriptPath: "  stream.log  ",
				Summary:        map[string]any{"ok": true},
			},
			Trace: &TraceInfo{
				Summary: &history.TraceSummary{
					Error: "  slow  ",
					Breaches: []history.TraceBreach{{
						Kind: "  total  ",
					}},
				},
				ArtifactPath: "  trace.json  ",
			},
			Steps: []StepResult{{
				Name:        "  step  ",
				Method:      "  grpc  ",
				Target:      "  /svc.Method  ",
				Environment: "  dev  ",
				Branch:      "  if-true  ",
				Summary:     "  ok  ",
				Passed:      true,
				SkipReason:  "  skipped  ",
				GRPC: &grpcclient.Response{
					StatusCode:    codes.OK,
					StatusMessage: "  ok  ",
				},
				Tests: []scripts.TestResult{{
					Name:    "  pass  ",
					Message: "  ok  ",
					Passed:  true,
				}},
				Stream: &StreamInfo{
					Kind:           "  sse  ",
					TranscriptPath: "  step.log  ",
				},
				Trace: &TraceInfo{
					Summary: &history.TraceSummary{
						Error: "  later  ",
						Breaches: []history.TraceBreach{{
							Kind: "  phase  ",
						}},
					},
					ArtifactPath: "  step-trace.json  ",
				},
			}},
		}},
	}

	got := NormalizeReport(rep)
	if got.Version != "v1" || got.EnvName != "dev" {
		t.Fatalf("expected top-level strings trimmed, got %+v", got)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected one result, got %+v", got.Results)
	}

	res := got.Results[0]
	if res.Name != "cmp" || res.Method != "compare" || res.Target != "https://example.com" ||
		res.Environment != "dev" || res.Summary != "ok" || res.SkipReason != "skipped" {
		t.Fatalf("expected result strings trimmed, got %+v", res)
	}
	if res.HTTP == nil || res.HTTP.Status != "200 OK" || res.HTTP.Protocol != "HTTP/1.1" {
		t.Fatalf("expected http strings trimmed, got %+v", res.HTTP)
	}
	if len(res.Tests) != 1 || res.Tests[0].Name != "status" || res.Tests[0].Message != "ok" {
		t.Fatalf("expected test strings trimmed, got %+v", res.Tests)
	}
	if res.Compare == nil || res.Compare.Baseline != "stage" {
		t.Fatalf("expected compare strings trimmed, got %+v", res.Compare)
	}
	if res.Profile == nil || len(res.Profile.Failures) != 1 ||
		res.Profile.Failures[0].Reason != "boom" || res.Profile.Failures[0].Status != "500" {
		t.Fatalf("expected profile failure strings trimmed, got %+v", res.Profile)
	}
	if res.Stream == nil || res.Stream.Kind != "ws" || res.Stream.TranscriptPath != "stream.log" {
		t.Fatalf("expected stream strings trimmed, got %+v", res.Stream)
	}
	if res.Trace == nil || res.Trace.Error != "slow" || res.Trace.ArtifactPath != "trace.json" ||
		len(res.Trace.Breaches) != 1 || res.Trace.Breaches[0].Kind != "total" {
		t.Fatalf("expected trace strings trimmed, got %+v", res.Trace)
	}
	if len(res.Steps) != 1 {
		t.Fatalf("expected one step, got %+v", res.Steps)
	}

	step := res.Steps[0]
	if step.Name != "step" || step.Method != "grpc" || step.Target != "/svc.Method" ||
		step.Environment != "dev" || step.Branch != "if-true" || step.Summary != "ok" ||
		step.SkipReason != "skipped" {
		t.Fatalf("expected step strings trimmed, got %+v", step)
	}
	if step.GRPC == nil || step.GRPC.StatusMessage != "ok" {
		t.Fatalf("expected grpc strings trimmed, got %+v", step.GRPC)
	}
	if len(step.Tests) != 1 || step.Tests[0].Name != "pass" || step.Tests[0].Message != "ok" {
		t.Fatalf("expected step test strings trimmed, got %+v", step.Tests)
	}
	if step.Stream == nil || step.Stream.Kind != "sse" || step.Stream.TranscriptPath != "step.log" {
		t.Fatalf("expected step stream strings trimmed, got %+v", step.Stream)
	}
	if step.Trace == nil || step.Trace.Error != "later" || step.Trace.ArtifactPath != "step-trace.json" ||
		len(step.Trace.Breaches) != 1 ||
		step.Trace.Breaches[0].Kind != "phase" {
		t.Fatalf("expected step trace strings trimmed, got %+v", step.Trace)
	}
}

func TestNormalizeReportUsesResponseDuration(t *testing.T) {
	rep := &Report{
		Results: []Result{{
			Passed: true,
			Response: &httpclient.Response{
				Duration: 25 * time.Millisecond,
			},
		}},
	}

	got := NormalizeReport(rep)
	if len(got.Results) != 1 || got.Results[0].Duration != 25*time.Millisecond {
		t.Fatalf("expected response duration fallback, got %+v", got.Results)
	}
}
