package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func buildWorkflowDoc() *restfile.Document {
	stepA := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/a",
		Metadata: restfile.RequestMetadata{
			Name: "StepA",
		},
	}
	stepB := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/b",
		Metadata: restfile.RequestMetadata{
			Name: "StepB",
		},
	}
	return &restfile.Document{
		Requests: []*restfile.Request{stepA, stepB},
	}
}

func TestEvaluateWorkflowStep(t *testing.T) {
	state := &workflowState{
		steps: []workflowStepRuntime{
			{step: restfile.WorkflowStep{OnFailure: restfile.WorkflowOnFailureStop}},
		},
	}
	resp := responseMsg{response: &httpclient.Response{Status: "200 OK", StatusCode: 200, Duration: 20 * time.Millisecond}}
	result := evaluateWorkflowStep(state, resp)
	if !result.Success {
		t.Fatalf("expected step success, got failure: %+v", result)
	}

	state.steps[0].step.Expect = map[string]string{"statuscode": "500"}
	failure := evaluateWorkflowStep(state, resp)
	if failure.Success {
		t.Fatalf("expected step failure due to status code expectation")
	}
}

func TestWorkflowRunProgression(t *testing.T) {
	doc := buildWorkflowDoc()
	workflow := restfile.Workflow{
		Name:             "demo",
		DefaultOnFailure: restfile.WorkflowOnFailureStop,
		Steps: []restfile.WorkflowStep{
			{
				Using:  "StepA",
				Expect: map[string]string{"statuscode": "200"},
				Vars:   map[string]string{"vars.workflow.token": "alpha"},
			},
			{Using: "StepB"},
		},
	}

	model := New(Config{})
	model.ready = true
	model.doc = doc

	cmd := model.startWorkflowRun(doc, workflow, model.cfg.HTTPOptions)
	if model.workflowRun == nil {
		t.Fatalf("expected workflow run state to be initialised")
	}
	if cmd == nil {
		t.Fatalf("expected start command to schedule execution")
	}
	if model.workflowRun.index != 0 {
		t.Fatalf("expected initial step index 0, got %d", model.workflowRun.index)
	}
	current := model.workflowRun.current
	if current == nil {
		t.Fatalf("expected current request to be set")
	}

	// simulate successful first step
	resp := responseMsg{response: &httpclient.Response{Status: "200 OK", StatusCode: 200, Duration: 15 * time.Millisecond}, executed: current}
	model.handleWorkflowResponse(resp)
	if model.workflowRun == nil {
		t.Fatalf("expected workflow to continue after first step")
	}
	if model.workflowRun.index != 1 {
		t.Fatalf("expected second step to be active, got index %d", model.workflowRun.index)
	}
	if got := model.workflowRun.vars["vars.workflow.token"]; got != "alpha" {
		t.Fatalf("expected workflow variable to persist, got %q", got)
	}

	second := model.workflowRun.current
	if second == nil {
		t.Fatalf("expected second step request to be prepared")
	}

	// simulate failure on second step
	failure := responseMsg{response: &httpclient.Response{Status: "500 Internal Server Error", StatusCode: 500}, executed: second}
	model.handleWorkflowResponse(failure)
	if model.workflowRun != nil {
		t.Fatalf("expected workflow run to finish after failure")
	}
	if len(model.statusMessage.text) == 0 {
		t.Fatalf("expected status message describing workflow result")
	}
}

func TestWorkflowCancelStopsRun(t *testing.T) {
	doc := buildWorkflowDoc()
	workflow := restfile.Workflow{
		Name:             "demo",
		DefaultOnFailure: restfile.WorkflowOnFailureStop,
		Steps: []restfile.WorkflowStep{
			{Using: "StepA"},
			{Using: "StepB"},
		},
	}

	model := New(Config{})
	model.ready = true
	model.doc = doc

	cmd := model.startWorkflowRun(doc, workflow, model.cfg.HTTPOptions)
	if cmd == nil {
		t.Fatalf("expected workflow start command")
	}
	current := model.workflowRun.current
	if current == nil {
		t.Fatalf("expected current workflow request")
	}

	cancelMsg := responseMsg{err: context.Canceled, executed: current}
	if follow := model.handleWorkflowResponse(cancelMsg); follow != nil {
		collectMsgs(follow)
	}

	if model.workflowRun != nil {
		t.Fatalf("expected workflow run to clear after cancel")
	}
	if model.statusMessage.level != statusWarn {
		t.Fatalf("expected warning level status, got %v", model.statusMessage.level)
	}
	if !strings.Contains(strings.ToLower(model.statusMessage.text), "canceled") {
		t.Fatalf("expected canceled status message, got %q", model.statusMessage.text)
	}
	if len(workflow.Steps) == 0 {
		t.Fatalf("expected workflow steps to remain defined")
	}
}

func TestBuildWorkflowReportIncludesCanceledSteps(t *testing.T) {
	state := &workflowState{
		workflow: restfile.Workflow{Name: "demo"},
		steps: []workflowStepRuntime{
			{step: restfile.WorkflowStep{Name: "One"}},
			{step: restfile.WorkflowStep{Name: "Two"}},
		},
		results: []workflowStepResult{
			{Step: restfile.WorkflowStep{Name: "One"}, Success: true},
		},
		canceled:     true,
		cancelReason: "stopped",
		start:        time.Now(),
		end:          time.Now(),
	}

	var model Model
	report := model.buildWorkflowReport(state)
	if !strings.Contains(report, "2. Two "+workflowStatusCanceled) {
		t.Fatalf("expected canceled step to be listed, got %q", report)
	}
}

func TestSyncWorkflowList(t *testing.T) {
	model := New(Config{})
	model.ready = true
	if len(model.workflowItems) != 0 {
		t.Fatalf("expected empty workflow list, got %d", len(model.workflowItems))
	}

	doc := &restfile.Document{
		Workflows: []restfile.Workflow{
			{Name: "alpha", Steps: []restfile.WorkflowStep{{Using: "step1"}}},
		},
	}
	if !model.syncWorkflowList(doc) {
		t.Fatalf("expected workflow visibility change to be reported")
	}
	if len(model.workflowItems) != 1 {
		t.Fatalf("expected one workflow item, got %d", len(model.workflowItems))
	}
	if len(model.workflowItems) != 1 {
		t.Fatalf("expected workflow list length 1, got %d", len(model.workflowItems))
	}
	if model.activeWorkflowKey == "" {
		t.Fatalf("expected active workflow key to be set")
	}
}

func TestWorkflowFocusFallback(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Name: "demo",
		},
	}
	withWorkflow := &restfile.Document{
		Requests: []*restfile.Request{req},
		Workflows: []restfile.Workflow{
			{
				Name:  "flow",
				Steps: []restfile.WorkflowStep{{Using: "demo"}},
			},
		},
	}
	noWorkflow := &restfile.Document{
		Requests: []*restfile.Request{req},
	}

	model := New(Config{})
	model.ready = true
	model.doc = withWorkflow
	model.syncRequestList(withWorkflow)
	_ = model.setFocus(focusWorkflows)
	if model.focus != focusWorkflows {
		t.Fatalf("expected focus to be workflows, got %d", model.focus)
	}

	model.doc = noWorkflow
	model.syncRequestList(noWorkflow)
	if model.focus == focusWorkflows {
		t.Fatalf("expected focus to leave workflows after removal")
	}
	if model.focus != focusRequests {
		t.Fatalf("expected focus to fall back to requests, got %d", model.focus)
	}
}
