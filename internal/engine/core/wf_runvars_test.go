package core

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func runVarDoc() *restfile.Document {
	return &restfile.Document{
		Path: "run.http",
		Requests: []*restfile.Request{
			{
				Method:   "GET",
				URL:      "https://example.com/a",
				Metadata: restfile.RequestMetadata{Name: "a"},
				RunVars:  []restfile.RunVar{{Name: "id", Value: "req"}},
			},
			{
				Method:   "GET",
				URL:      "https://example.com/items/b",
				Metadata: restfile.RequestMetadata{Name: "b"},
			},
		},
	}
}

func runVarPlan(t *testing.T, doc *restfile.Document, wf restfile.Workflow) *WorkflowPlan {
	t.Helper()
	pl, err := PrepareWorkflow(doc, wf, RunMeta{ID: "wf-run", Env: testEnvironment("dev")})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}
	return pl
}

func runStep(name, using string) restfile.WorkflowStep {
	return restfile.WorkflowStep{Kind: restfile.WorkflowStepKindRequest, Name: name, Using: using}
}

func collectStepDone(evts *[]WfStepDone) Sink {
	return SinkFunc(func(_ context.Context, e Evt) error {
		if v, ok := e.(WfStepDone); ok {
			*evts = append(*evts, v)
		}
		return nil
	})
}

func TestRunPlanEvaluatesRunVarsOncePerRun(t *testing.T) {
	doc := runVarDoc()
	pl := runVarPlan(t, doc, restfile.Workflow{
		Name:    "order",
		RunVars: []restfile.RunVar{{Name: "suffix", Value: "wf"}},
		Steps: []restfile.WorkflowStep{
			runStep("A", "a"),
			runStep("B", "b"),
			runStep("A again", "a"),
			{
				Kind:    restfile.WorkflowStepKindForEach,
				Name:    "Each",
				Using:   "a",
				ForEach: &restfile.WorkflowForEach{Expr: "items", Var: "item"},
			},
		},
	})
	dep := &fakeDep{each: map[string][]rts.Value{"items": {rts.Str("x"), rts.Str("y")}}}

	if err := RunPlan(context.Background(), dep, nil, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if dep.runVarEvals != 2 {
		t.Fatalf("evaluated %d times, want once for the workflow and once for request a", dep.runVarEvals)
	}
	if len(dep.sent) != 5 {
		t.Fatalf("sent %d requests, want 5", len(dep.sent))
	}
	for i, opt := range dep.sent {
		if got, _ := opt.Run.RunVars.Get("suffix"); got != "wf-1" {
			t.Fatalf("request %d suffix = %q, want wf-1", i, got)
		}
		id, ok := opt.Run.RunVars.Get("id")
		if i == 1 {
			if ok {
				t.Fatalf("request b sees request a's id %q", id)
			}
			continue
		}
		if id != "req-2" {
			t.Fatalf("request %d id = %q, want req-2", i, id)
		}
	}

	dep.sent = nil
	if err := RunPlan(context.Background(), dep, nil, pl); err != nil {
		t.Fatalf("second RunPlan: %v", err)
	}
	if got, _ := dep.sent[0].Run.RunVars.Get("suffix"); got != "wf-3" {
		t.Fatalf("second run suffix = %q, want a fresh wf-3", got)
	}
}

func TestRunPlanRequestRunVarShadowsWorkflow(t *testing.T) {
	doc := runVarDoc()
	pl := runVarPlan(t, doc, restfile.Workflow{
		Name:    "shadow",
		RunVars: []restfile.RunVar{{Name: "id", Value: "wf"}},
		Steps:   []restfile.WorkflowStep{runStep("A", "a"), runStep("B", "b")},
	})
	dep := &fakeDep{}

	if err := RunPlan(context.Background(), dep, nil, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if got, _ := dep.sent[0].Run.RunVars.Get("id"); got != "req-2" {
		t.Fatalf("request a id = %q, want its own req-2", got)
	}
	if got, _ := dep.sent[1].Run.RunVars.Get("id"); got != "wf-1" {
		t.Fatalf("request b id = %q, want the workflow's wf-1", got)
	}
}

func TestRunPlanBranchSeesRunVars(t *testing.T) {
	doc := runVarDoc()
	pl := runVarPlan(t, doc, restfile.Workflow{
		Name:    "branch",
		RunVars: []restfile.RunVar{{Name: "suffix", Value: "wf"}},
		Steps: []restfile.WorkflowStep{{
			Kind: restfile.WorkflowStepKindIf,
			Name: "Choose",
			If:   &restfile.WorkflowIf{Then: restfile.WorkflowIfBranch{Cond: "true", Run: "a"}},
		}},
	})
	dep := &fakeDep{}

	if err := RunPlan(context.Background(), dep, nil, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(dep.evalVars) != 1 || dep.evalVars[0]["suffix"] != "wf-1" {
		t.Fatalf("@if saw %v, want suffix=wf-1", dep.evalVars)
	}
	if len(dep.sent) != 1 {
		t.Fatalf("sent %d requests, want 1", len(dep.sent))
	}
	if got, _ := dep.sent[0].Run.RunVars.Get("id"); got != "req-2" {
		t.Fatalf("branch request id = %q, want req-2", got)
	}
}

func TestRunPlanWorkflowRunVarFailureFailsEveryStep(t *testing.T) {
	doc := runVarDoc()
	pl := runVarPlan(t, doc, restfile.Workflow{
		Name:             "broken",
		DefaultOnFailure: restfile.WorkflowOnFailureContinue,
		RunVars:          []restfile.RunVar{{Name: "bad", Value: "fail"}},
		Steps: []restfile.WorkflowStep{
			runStep("A", "a"),
			{
				Kind: restfile.WorkflowStepKindIf,
				Name: "Choose",
				If:   &restfile.WorkflowIf{Then: restfile.WorkflowIfBranch{Cond: "true", Run: "b"}},
			},
		},
	})
	dep := &fakeDep{}
	var done []WfStepDone
	var end RunDone
	sink := SinkFunc(func(ctx context.Context, e Evt) error {
		if v, ok := e.(RunDone); ok {
			end = v
		}
		return collectStepDone(&done).OnEvt(ctx, e)
	})

	if err := RunPlan(context.Background(), dep, sink, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(dep.sent) != 0 {
		t.Fatalf("sent %d requests, want none", len(dep.sent))
	}
	if len(done) != 2 {
		t.Fatalf("got %d finished steps, want 2", len(done))
	}
	for _, evt := range done {
		if evt.Result.Err == nil || !strings.Contains(evt.Result.Err.Error(), "@run var bad failed") {
			t.Fatalf("step %q error = %v, want the @run var failure", evt.Step.Name, evt.Result.Err)
		}
	}
	if end.Success {
		t.Fatalf("run reported success: %+v", end)
	}
}

func TestRunPlanRequestRunVarFailureStaysWithItsRequest(t *testing.T) {
	doc := runVarDoc()
	doc.Requests[0].RunVars = []restfile.RunVar{{Name: "id", Value: "fail"}}
	pl := runVarPlan(t, doc, restfile.Workflow{
		Name:             "partial",
		DefaultOnFailure: restfile.WorkflowOnFailureContinue,
		Steps:            []restfile.WorkflowStep{runStep("A", "a"), runStep("B", "b"), runStep("A again", "a")},
	})
	dep := &fakeDep{}
	var done []WfStepDone

	if err := RunPlan(context.Background(), dep, collectStepDone(&done), pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(dep.sent) != 1 {
		t.Fatalf("sent %d requests, want only b", len(dep.sent))
	}
	if dep.runVarEvals != 1 {
		t.Fatalf("evaluated %d times, want the failure kept for the second use", dep.runVarEvals)
	}
	for _, i := range []int{0, 2} {
		if done[i].Result.Err == nil {
			t.Fatalf("step %q passed, want the @run var failure", done[i].Step.Name)
		}
	}
	if done[1].Result.Err != nil {
		t.Fatalf("step B failed: %v", done[1].Result.Err)
	}
}

func TestRunPlanForEachSharesRequestRunVars(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/items/{{vars.request.item}}",
		Metadata: restfile.RequestMetadata{
			Name:    "each",
			ForEach: &restfile.ForEachSpec{Expression: "items", Var: "item"},
		},
		RunVars: []restfile.RunVar{{Name: "id", Value: "req"}},
	}
	pl, err := PrepareForEach(&restfile.Document{Path: "each.http"}, req, RunMeta{Env: testEnvironment("dev")})
	if err != nil {
		t.Fatalf("PrepareForEach: %v", err)
	}
	dep := &fakeDep{each: map[string][]rts.Value{"items": {rts.Str("x"), rts.Str("y"), rts.Str("z")}}}

	if err := RunPlan(context.Background(), dep, nil, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(dep.sent) != 3 {
		t.Fatalf("sent %d requests, want 3", len(dep.sent))
	}
	for i, opt := range dep.sent {
		if got, _ := opt.Run.RunVars.Get("id"); got != "req-1" {
			t.Fatalf("iteration %d id = %q, want the shared req-1", i, got)
		}
	}
}
