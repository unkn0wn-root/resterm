package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func TestBuildAndRunPlanMatchRunContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "api.http")
	src := fmt.Sprintf("# @name ok\nGET %s\n", srv.URL)
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	opt := Options{
		FilePath:      file,
		WorkspaceRoot: dir,
	}
	pl, err := Build(opt)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got, err := RunPlan(context.Background(), pl)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	want, err := RunContext(context.Background(), opt)
	if err != nil {
		t.Fatalf("RunContext: %v", err)
	}
	assertRunnerReportParity(t, want, got)
}

func TestRunPlanUsesBuiltFileSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "api.http")
	src := fmt.Sprintf("# @name ok\nGET %s\n", srv.URL)
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	pl, err := Build(Options{FilePath: file, WorkspaceRoot: dir})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := os.WriteFile(file, []byte("GET http://127.0.0.1:1/nope\n"), 0o644); err != nil {
		t.Fatalf("overwrite file: %v", err)
	}

	rep, err := RunPlan(context.Background(), pl)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if rep.Passed != 1 || rep.Failed != 0 {
		t.Fatalf("unexpected report: %+v", rep)
	}
}

func TestRunPlanDoesNotMutateBuiltDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"token":"doc-123"}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "capture.http")
	src := strings.Join([]string{
		"# @name login",
		"# @capture file auth.token {{response.json.token}}",
		"GET " + srv.URL,
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	pl, err := Build(Options{FilePath: file, WorkspaceRoot: dir})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := len(pl.doc.Variables); got != 0 {
		t.Fatalf("built doc variables = %d, want 0", got)
	}

	for i := 0; i < 2; i++ {
		rep, err := RunPlan(context.Background(), pl)
		if err != nil {
			t.Fatalf("RunPlan(%d): %v", i, err)
		}
		if rep.Passed != 1 || rep.Failed != 0 {
			t.Fatalf("unexpected report: %+v", rep)
		}
		if got := len(pl.doc.Variables); got != 0 {
			t.Fatalf("built doc variables after run %d = %d, want 0", i, got)
		}
	}
}

func TestRunPlanUsesBuiltClientSnapshot(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "api.http")
	if err := os.WriteFile(file, []byte("GET https://example.com/status\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("built")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	pl, err := Build(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("mutated")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := RunPlan(context.Background(), pl)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(rep.Results) != 1 || rep.Results[0].Response == nil {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if got := strings.TrimSpace(string(rep.Results[0].Response.Body)); got != "built" {
		t.Fatalf("response body = %q, want %q", got, "built")
	}
}

func TestRunPlanRejectsNilPlan(t *testing.T) {
	_, err := RunPlan(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsUsageError(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "runner plan is nil") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunPlanRejectsNilContext(t *testing.T) {
	var ctx context.Context
	_, err := RunPlan(ctx, nil)
	if !errors.Is(err, ErrNilContext) {
		t.Fatalf("RunPlan(nil, nil): got %v want %v", err, ErrNilContext)
	}
}

func TestRunPlanRejectsInvalidSelection(t *testing.T) {
	tests := []struct {
		name string
		pl   *Plan
		msg  string
	}{
		{
			name: "request index out of range",
			pl: &Plan{
				doc: &restfile.Document{
					Requests: []*restfile.Request{{}},
				},
				sel: selectedTarget{requests: []int{1}},
			},
			msg: "invalid runner plan: request index 1 out of range",
		},
		{
			name: "workflow and requests both selected",
			pl: &Plan{
				doc: &restfile.Document{
					Requests:  []*restfile.Request{{}},
					Workflows: []restfile.Workflow{{}},
				},
				sel: selectedTarget{
					requests:    []int{0},
					workflow:    0,
					workflowSet: true,
				},
			},
			msg: "invalid runner plan: workflow and requests are both selected",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RunPlan(context.Background(), tc.pl)
			if err == nil {
				t.Fatal("expected error")
			}
			if IsUsageError(err) {
				t.Fatalf("expected non-usage error, got %v", err)
			}
			if err.Error() != tc.msg {
				t.Fatalf("error = %q, want %q", err.Error(), tc.msg)
			}
		})
	}
}

// assertRunnerReportParity compares semantic parity between two independent
// executions. It adjusts runtime-only timing fields because RunPlan(pl) and
// RunContext(opt) each perform a separate request, so timestamps and transport
// durations can differ slightly even when behavior is identical.
func assertRunnerReportParity(t *testing.T, want, got *Report) {
	t.Helper()
	want = parityRunnerReport(want)
	got = parityRunnerReport(got)
	var wantJSON strings.Builder
	if err := want.WriteJSON(&wantJSON); err != nil {
		t.Fatalf("want WriteJSON: %v", err)
	}
	var gotJSON strings.Builder
	if err := got.WriteJSON(&gotJSON); err != nil {
		t.Fatalf("got WriteJSON: %v", err)
	}
	if gotJSON.String() != wantJSON.String() {
		t.Fatalf("report mismatch\nwant:\n%s\ngot:\n%s", wantJSON.String(), gotJSON.String())
	}
}

func parityRunnerReport(src *Report) *Report {
	if src == nil {
		return nil
	}
	out := *src
	out.StartedAt = time.Time{}
	out.EndedAt = time.Time{}
	out.Duration = 0
	if len(src.Results) == 0 {
		out.Results = nil
		return &out
	}
	out.Results = make([]Result, 0, len(src.Results))
	for _, res := range src.Results {
		out.Results = append(out.Results, parityRunnerResult(res))
	}
	return &out
}

func parityRunnerResult(src Result) Result {
	out := src
	out.Duration = 0
	out.Response = parityHTTPResponse(src.Response)
	out.GRPC = parityGRPCResponse(src.GRPC)
	out.Tests = parityRunnerTests(src.Tests)
	if len(src.Steps) == 0 {
		out.Steps = nil
		return out
	}
	out.Steps = make([]StepResult, 0, len(src.Steps))
	for _, step := range src.Steps {
		out.Steps = append(out.Steps, parityRunnerStep(step))
	}
	return out
}

func parityRunnerStep(src StepResult) StepResult {
	out := src
	out.Duration = 0
	out.Response = parityHTTPResponse(src.Response)
	out.GRPC = parityGRPCResponse(src.GRPC)
	out.Tests = parityRunnerTests(src.Tests)
	return out
}

func parityHTTPResponse(src *httpclient.Response) *httpclient.Response {
	if src == nil {
		return nil
	}
	out := *src
	out.Duration = 0
	return &out
}

func parityGRPCResponse(src *grpcclient.Response) *grpcclient.Response {
	if src == nil {
		return nil
	}
	out := *src
	out.Duration = 0
	return &out
}

func parityRunnerTests(src []scripts.TestResult) []scripts.TestResult {
	if len(src) == 0 {
		return nil
	}
	out := append([]scripts.TestResult(nil), src...)
	for i := range out {
		out[i].Elapsed = 0
	}
	return out
}
