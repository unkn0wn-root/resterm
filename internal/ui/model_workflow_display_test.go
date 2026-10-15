package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestWorkflowRunSubjectForEachUsesRequestBaseTitle(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/" + strings.Repeat("segment/", 12),
	}
	st := &workflowState{
		origin: workflowOriginForEach,
		workflow: restfile.Workflow{
			Name: "GET " + req.URL,
		},
		steps: []workflowStepRuntime{{
			request: req,
		}},
	}

	if got, want := st.runSubject(), requestBaseTitle(req); got != want {
		t.Fatalf("runSubject() = %q, want %q", got, want)
	}
}

func TestWorkflowReportIndentsEveryMessageLine(t *testing.T) {
	st := &workflowState{
		workflow: restfile.Workflow{Name: "wf"},
		steps:    []workflowStepRuntime{{step: restfile.WorkflowStep{Name: "Orders"}}},
		results: []workflowStepResult{{
			Step:    restfile.WorkflowStep{Name: "Orders"},
			Message: "api.http:11: Order 0 has negative total\napi.http:11: Order 3 missing tracking",
		}},
	}

	got := (&Model{}).buildWorkflowReport(st)
	want := "\n    api.http:11: Order 0 has negative total\n    api.http:11: Order 3 missing tracking"
	if !strings.HasSuffix(got, want) {
		t.Fatalf("report =\n%s\nwant it to end with%s", got, want)
	}
}

func TestWorkflowStepDurationIncludesPollWaits(t *testing.T) {
	res := engine.RequestResult{
		Executed: &restfile.Request{Method: "GET", URL: "https://example.com/jobs/1"},
		Response: &httpx.Response{Status: "200 OK", StatusCode: 200, Duration: 5 * time.Millisecond},
		Timing:   engine.Timing{Total: 30 * time.Second, Transport: 5 * time.Millisecond},
	}
	out := workflowResultFromRun(restfile.WorkflowStep{}, core.StepMeta{}, res, time.Minute)
	if out.Duration != 30*time.Second {
		t.Fatalf("step duration = %s, want 30s", out.Duration)
	}
}

func TestWorkflowStepDurationFallsBackToStepWallTime(t *testing.T) {
	res := engine.RequestResult{
		Executed: &restfile.Request{Method: "GET", URL: "https://example.com/jobs/1"},
		Skipped:  true,
	}
	out := workflowResultFromRun(restfile.WorkflowStep{}, core.StepMeta{}, res, 7*time.Millisecond)
	if out.Duration != 7*time.Millisecond {
		t.Fatalf("step duration = %s, want 7ms", out.Duration)
	}
}

func TestWorkflowSummaryStaysOnOneLine(t *testing.T) {
	st := &workflowState{
		workflow: restfile.Workflow{Name: "wf"},
		steps:    []workflowStepRuntime{{step: restfile.WorkflowStep{Name: "Orders"}}},
		results: []workflowStepResult{{
			Step:    restfile.WorkflowStep{Name: "Orders"},
			Message: "api.http:11: Order 0 has negative total\napi.http:11: Order 3 missing tracking",
		}},
	}

	got := st.summaryFor("Workflow wf")
	want := "Workflow wf failed at step Orders: api.http:11: Order 0 has negative total api.http:11: Order 3 missing tracking"
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}
