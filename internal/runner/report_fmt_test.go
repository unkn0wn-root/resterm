package runner

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"google.golang.org/grpc/codes"
)

func TestReportModelTrimsStrings(t *testing.T) {
	rep := &Report{
		Version: "  v1  ",
		EnvName: "  dev  ",
		Results: []Result{{
			Kind:            ResultKindCompare,
			Name:            "  cmp  ",
			Method:          "  compare  ",
			Target:          "  https://example.com  ",
			EffectiveTarget: "  https://api.example.com  ",
			Environment:     "  dev  ",
			Summary:         "  ok  ",
			Passed:          true,
			SkipReason:      "  skipped  ",
			Response: &httpx.Response{
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
				Name:            "  step  ",
				Method:          "  grpc  ",
				Target:          "  /svc.Method  ",
				EffectiveTarget: "  api.example.com:443  ",
				Environment:     "  dev  ",
				Branch:          "  if-true  ",
				Summary:         "  ok  ",
				Passed:          true,
				SkipReason:      "  skipped  ",
				GRPC: &grpcx.Response{
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

	got := ReportModel(rep)
	if got.Version != "v1" || got.EnvName != "dev" {
		t.Fatalf("expected top-level strings trimmed, got %+v", got)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected one result, got %+v", got.Results)
	}

	res := got.Results[0]
	if res.Name != "cmp" || res.Method != "compare" || res.Target != "https://example.com" ||
		res.EffectiveTarget != "https://api.example.com" ||
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
		step.EffectiveTarget != "api.example.com:443" ||
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
	if step.Trace == nil || step.Trace.Error != "later" ||
		step.Trace.ArtifactPath != "step-trace.json" ||
		len(step.Trace.Breaches) != 1 ||
		step.Trace.Breaches[0].Kind != "phase" {
		t.Fatalf("expected step trace strings trimmed, got %+v", step.Trace)
	}
}

func TestReportModelUsesStructuredProfileFailure(t *testing.T) {
	rep := &Report{
		Results: []Result{{
			Kind:   ResultKindProfile,
			Name:   "prof",
			Passed: false,
			Profile: &ProfileInfo{
				Failures: []ProfileFailure{{
					Reason:     "HTTP 500",
					StatusCode: 500,
					Failure: runfail.New(
						runfail.CodeTimeout,
						"context deadline exceeded",
						"profile",
					),
				}},
			},
		}},
	}

	got := ReportModel(rep)
	if len(got.Results) != 1 || got.Results[0].Profile == nil ||
		len(got.Results[0].Profile.Failures) != 1 {
		t.Fatalf("unexpected formatted profile: %+v", got.Results)
	}
	failure := got.Results[0].Profile.Failures[0].Failure
	if failure == nil || failure.Code != runfail.CodeTimeout ||
		failure.ExitCode != runfail.ExitTimeout ||
		failure.Message != "context deadline exceeded" {
		t.Fatalf("unexpected profile failure: %+v", failure)
	}
	if got.Results[0].Failure == nil ||
		got.Results[0].Failure.Code != runfail.CodeTimeout {
		t.Fatalf(
			"expected result failure to use structured profile failure, got %+v",
			got.Results[0].Failure,
		)
	}
}

func TestReportModelIncludesErrorDetails(t *testing.T) {
	reqErr := diag.Wrap(
		&url.Error{
			Op:  "Get",
			URL: "https://api.local",
			Err: &net.DNSError{Err: "no such host", Name: "api.local"},
		},
		"perform request",
		diag.WithComponent(diag.ComponentHTTP),
	)
	scriptErr := errors.New("script boom")
	rep := &Report{
		Results: []Result{{
			Kind:      ResultKindRequest,
			Name:      "lookup",
			Err:       reqErr,
			ScriptErr: scriptErr,
			Profile: &ProfileInfo{
				Failures: []ProfileFailure{{
					Reason:  "request failed",
					Err:     reqErr,
					Failure: runfail.FromErrorSource(reqErr, "profile"),
				}},
			},
			Steps: []StepResult{{
				Name: "step",
				Err:  reqErr,
			}},
		}},
	}

	got := ReportModel(rep)
	res := got.Results[0]
	if res.ErrorDetail == nil ||
		!strings.Contains(res.ErrorDetail.Rendered, "\nperform request\n") ||
		res.ScriptErrorDetail == nil ||
		!strings.Contains(res.ScriptErrorDetail.Rendered, "script boom") {
		t.Fatalf("expected result error details, got %+v", res)
	}
	if len(res.Steps) != 1 || res.Steps[0].ErrorDetail == nil ||
		!strings.Contains(res.Steps[0].ErrorDetail.Rendered, "lookup api.local") {
		t.Fatalf("expected step error detail, got %+v", res.Steps)
	}
	if res.Profile == nil || len(res.Profile.Failures) != 1 ||
		res.Profile.Failures[0].Failure == nil ||
		len(res.Profile.Failures[0].Failure.Chain) == 0 {
		t.Fatalf("expected profile failure chain, got %+v", res.Profile)
	}
}

func TestReportModelUsesResponseDuration(t *testing.T) {
	rep := &Report{
		Results: []Result{{
			Passed: true,
			Response: &httpx.Response{
				Duration: 25 * time.Millisecond,
			},
		}},
	}

	got := ReportModel(rep)
	if len(got.Results) != 1 || got.Results[0].Duration != 25*time.Millisecond {
		t.Fatalf("expected response duration fallback, got %+v", got.Results)
	}
}

func TestFormatStreamCarriesTheFailure(t *testing.T) {
	got := formatStream(&StreamInfo{Kind: "sse", Err: errors.New("sse line exceeds 16 bytes")})
	if got.Error != "sse line exceeds 16 bytes" {
		t.Fatalf("Error = %q, want the stream failure", got.Error)
	}
	if plain := formatStream(&StreamInfo{Kind: "sse"}); plain.Error != "" {
		t.Fatalf("Error = %q, want a complete transcript to report none", plain.Error)
	}
}

// A stream carries its failure as text, so the class it recorded is what sets
// the reported failure and the exit code.
func TestStreamFailureKeepsItsExitCode(t *testing.T) {
	tests := []struct {
		name string
		sum  httpx.WebSocketSummary
		code runfail.Code
		exit int
	}{
		{
			name: "unreadable payload file",
			sum: httpx.WebSocketSummary{
				ClosedBy:    "error",
				CloseReason: "websocket step 2:send_file: open payload.bin: no such file",
				ErrorClass:  diag.ClassFilesystem,
			},
			code: runfail.CodeFilesystem,
			exit: runfail.ExitFilesystem,
		},
		{
			name: "broken frame",
			sum: httpx.WebSocketSummary{
				ClosedBy:    "error",
				CloseReason: "read websocket message: bad frame",
				ErrorClass:  diag.ClassProtocol,
			},
			code: runfail.CodeProtocol,
			exit: runfail.ExitProtocol,
		},
		{
			name: "connection dropped",
			sum: httpx.WebSocketSummary{
				ClosedBy:    "error",
				CloseReason: "read websocket message: connection reset by peer",
				ErrorClass:  diag.ClassNetwork,
			},
			code: runfail.CodeNetwork,
			exit: runfail.ExitNetwork,
		},
		{
			name: "a transcript with no class",
			sum:  httpx.WebSocketSummary{ClosedBy: "error", CloseReason: "something broke"},
			code: runfail.CodeProtocol,
			exit: runfail.ExitProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Result{Stream: &StreamInfo{Kind: "websocket", Err: tt.sum.Err()}}

			got := resultFailure(res)
			if got.Code != tt.code || got.ExitCode != tt.exit {
				t.Fatalf("failure = %s/%d, want %s/%d", got.Code, got.ExitCode, tt.code, tt.exit)
			}
			if got.Source != "stream" {
				t.Fatalf("source = %q, want %q", got.Source, "stream")
			}
		})
	}
}
