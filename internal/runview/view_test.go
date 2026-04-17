package runview

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"google.golang.org/grpc/codes"
)

func TestRenderHTTPPrettyWithHeadersAndFailures(t *testing.T) {
	rep := testHTTPPrettyReport()

	out, err := Render(rep, Options{Mode: ModePretty, Headers: true})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected plain output, got %q", out)
	}
	for _, want := range []string{
		"Name: items",
		"Request: GET https://example.com/items",
		"Environment: dev",
		"Status: 200 OK",
		"Duration: 12ms",
		"Request Headers:",
		"Response Headers:",
		"Errors:",
		"Script error: script boom",
		"Warnings:",
		"Unresolved template variables: reporting.token",
		"Tests:",
		"[PASS] method - expected GET (1ms)",
		"[FAIL] status - expected 201 (3ms)",
		`id: 1`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func TestRenderHTTPPrettyUsesTrackedUnresolvedVarsAndEffectiveTarget(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:            runner.ResultKindRequest,
			Name:            "ReportsList",
			Method:          "GET",
			Target:          "{{services.api.base}}/reports",
			EffectiveTarget: "https://httpbin.org/anything/api/reports",
			Environment:     "dev",
			Response: &httpclient.Response{
				Status:       "200 OK",
				StatusCode:   200,
				Headers:      http.Header{"Content-Type": {"application/json"}},
				Body:         []byte(`{"ok":true}`),
				Duration:     463 * time.Millisecond,
				EffectiveURL: "https://httpbin.org/anything/api/reports",
				ReqMethod:    "GET",
			},
		}},
	}
	rep.Results[0].SetRequestText(strings.Join([]string{
		"GET {{services.api.base}}/reports",
		"X-API-Key: {{reporting.apiKey}}",
		"X-Shared-Secret: {{reporting.sharedSecret}}",
	}, "\n"))
	rep.Results[0].SetUnresolvedTemplateVars([]string{"reporting.token"})

	out, err := Render(rep, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if !strings.Contains(out, "Request: GET https://httpbin.org/anything/api/reports") {
		t.Fatalf("expected effective request target, got %q", out)
	}
	if !strings.Contains(out, "Source Target: {{services.api.base}}/reports") {
		t.Fatalf("expected source target details, got %q", out)
	}
	if !strings.Contains(out, "Unresolved template variables: reporting.token") {
		t.Fatalf("expected tracked unresolved variable warning, got %q", out)
	}
	for _, bad := range []string{
		"reporting.apiKey",
		"reporting.sharedSecret",
	} {
		if strings.Contains(out, bad) {
			t.Fatalf("did not expect %q in output, got %q", bad, out)
		}
	}
}

func TestRenderHTTPPrettyColorPreservesPlainText(t *testing.T) {
	rep := testHTTPPrettyReport()

	plain, err := Render(rep, Options{Mode: ModePretty, Headers: true})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	out, err := Render(rep, Options{
		Mode:    ModePretty,
		Headers: true,
		Color:   termcolor.TrueColor(),
	})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected colored output, got %q", out)
	}
	if got := ansi.Strip(out); got != plain {
		t.Fatalf("expected color stripping to preserve output\nwant:\n%s\n\ngot:\n%s", plain, got)
	}
}

func TestRenderGRPCRawFallsBackToStreamSummary(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:   runner.ResultKindRequest,
			Method: "GRPC",
			Target: "/demo.Service/Watch",
			GRPC: &grpcclient.Response{
				StatusCode:    codes.OK,
				StatusMessage: "ok",
				Duration:      25 * time.Millisecond,
			},
			Stream: &runner.StreamInfo{
				Kind: "websocket",
				Summary: map[string]any{
					"eventCount": 2,
					"reason":     "complete",
				},
			},
		}},
	}

	out, err := Render(rep, Options{Mode: ModeRaw})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	for _, want := range []string{
		"Request: GRPC /demo.Service/Watch",
		"Status: OK",
		"Raw Body:",
		"Stream: websocket",
		"eventCount: 2",
		"reason: complete",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
}

func TestRenderFallsBackToReportTextForWorkflow(t *testing.T) {
	rep := &runner.Report{
		Total:  1,
		Passed: 1,
		Results: []runner.Result{{
			Kind:     runner.ResultKindWorkflow,
			Name:     "deploy",
			Method:   "WORKFLOW",
			Duration: time.Second,
			Passed:   true,
			Steps: []runner.StepResult{{
				Name:     "stage",
				Passed:   true,
				Duration: 250 * time.Millisecond,
			}},
		}},
	}

	out, err := Render(rep, Options{Mode: ModePretty})
	if err != nil {
		t.Fatalf("Render(...): %v", err)
	}
	if !strings.Contains(out, "PASS WORKFLOW deploy") {
		t.Fatalf("expected report text fallback, got %q", out)
	}
}

func TestRenderBodyRawReturnsBodyOnly(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:   runner.ResultKindRequest,
			Name:   "items",
			Method: "GET",
			Target: "https://example.com/items",
			Response: &httpclient.Response{
				Status:       "200 OK",
				StatusCode:   200,
				Headers:      http.Header{"Content-Type": {"application/json"}},
				Body:         []byte(`{"id":1}`),
				Duration:     12 * time.Millisecond,
				EffectiveURL: "https://example.com/items",
				ReqMethod:    "GET",
			},
			Passed: true,
		}},
	}

	out, err := RenderBody(rep, BodyOptions{Mode: ModeRaw})
	if err != nil {
		t.Fatalf("RenderBody(...): %v", err)
	}
	if out != "{\n  \"id\": 1\n}" {
		t.Fatalf("unexpected body output %q", out)
	}
}

func TestRenderBodyPrettyCanColorOutput(t *testing.T) {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind: runner.ResultKindRequest,
			Response: &httpclient.Response{
				Headers: http.Header{"Content-Type": {"application/json"}},
				Body:    []byte(`{"id":1}`),
			},
		}},
	}

	plain, err := RenderBody(rep, BodyOptions{Mode: ModePretty})
	if err != nil {
		t.Fatalf("RenderBody(...): %v", err)
	}
	out, err := RenderBody(rep, BodyOptions{Mode: ModePretty, Color: termcolor.TrueColor()})
	if err != nil {
		t.Fatalf("RenderBody(...): %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected colored body output, got %q", out)
	}
	if got := ansi.Strip(out); got != plain {
		t.Fatalf("expected stripped body to match plain output\nwant:\n%s\n\ngot:\n%s", plain, got)
	}
}

func testHTTPPrettyReport() *runner.Report {
	rep := &runner.Report{
		Results: []runner.Result{{
			Kind:        runner.ResultKindRequest,
			Name:        "items",
			Method:      "GET",
			Target:      "https://example.com/items",
			Environment: "dev",
			Response: &httpclient.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Headers:    http.Header{"Content-Type": {"application/json"}, "X-Resp": {"ok"}},
				RequestHeaders: http.Header{
					"Accept": {"application/json"},
				},
				ReqHost:      "example.com",
				Body:         []byte(`{"id":1}`),
				Duration:     12 * time.Millisecond,
				EffectiveURL: "https://example.com/items",
				ReqMethod:    "GET",
				ReqLen:       7,
				ReqTE:        []string{"gzip"},
			},
			ScriptErr: errors.New("script boom"),
			Tests: []scripts.TestResult{
				{
					Name:    "method",
					Message: "expected GET",
					Passed:  true,
					Elapsed: 1 * time.Millisecond,
				},
				{
					Name:    "status",
					Message: "expected 201",
					Passed:  false,
					Elapsed: 3 * time.Millisecond,
				},
			},
		}},
	}
	rep.Results[0].SetUnresolvedTemplateVars([]string{"reporting.token"})
	return rep
}
