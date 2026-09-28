package core

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func failureDoc() *restfile.Document {
	return &restfile.Document{
		Path: "demo.http",
		Requests: []*restfile.Request{
			{
				Method:   "GET",
				URL:      "https://example.com/first",
				Metadata: restfile.RequestMetadata{Name: "first"},
			},
			{
				Method:   "GET",
				URL:      "https://example.com/next",
				Metadata: restfile.RequestMetadata{Name: "next"},
			},
		},
	}
}

func nextStep() restfile.WorkflowStep {
	return restfile.WorkflowStep{
		Kind:  restfile.WorkflowStepKindRequest,
		Name:  "Next",
		Using: "next",
	}
}

// runFailureWorkflow runs a two step workflow and reports which requests were
// sent plus whether the run was reported as successful.
func runFailureWorkflow(
	t *testing.T,
	first restfile.WorkflowStep,
	mode restfile.WorkflowFailureMode,
) ([]string, bool) {
	t.Helper()

	first.OnFailure = mode
	next := nextStep()
	next.OnFailure = mode

	pl, err := PrepareWorkflow(failureDoc(), restfile.Workflow{
		Name:             "demo",
		DefaultOnFailure: mode,
		Steps:            []restfile.WorkflowStep{first, next},
	}, RunMeta{ID: "wf-1", Env: testEnvironment("dev")})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}

	var sent []string
	success := false
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case ReqStart:
			sent = append(sent, v.Req.Label)
		case RunDone:
			success = v.Success
		}
		return nil
	})

	if err := RunPlan(context.Background(), &fakeDep{}, sink, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	return sent, success
}

// A control flow step that fails has to obey the same on-failure policy as a
// request step. Only the request path used to honor it.
func TestRunPlanControlFlowFailureHonorsOnFailure(t *testing.T) {
	steps := map[string]restfile.WorkflowStep{
		"if condition error": {
			Kind: restfile.WorkflowStepKindIf,
			Name: "Choose",
			If: &restfile.WorkflowIf{
				Then: restfile.WorkflowIfBranch{Cond: "boom", Run: "first"},
			},
		},
		"if fail branch": {
			Kind: restfile.WorkflowStepKindIf,
			Name: "Choose",
			If: &restfile.WorkflowIf{
				Then: restfile.WorkflowIfBranch{Cond: "true", Fail: "explicit failure"},
			},
		},
		"switch expression error": {
			Kind: restfile.WorkflowStepKindSwitch,
			Name: "Pick",
			Switch: &restfile.WorkflowSwitch{
				Expr:  "boom",
				Cases: []restfile.WorkflowSwitchCase{{Expr: "a", Run: "first"}},
			},
		},
		"switch fail case": {
			Kind: restfile.WorkflowStepKindSwitch,
			Name: "Pick",
			Switch: &restfile.WorkflowSwitch{
				Expr:  "a",
				Cases: []restfile.WorkflowSwitchCase{{Expr: "a", Fail: "explicit failure"}},
			},
		},
	}

	for name, step := range steps {
		t.Run(name, func(t *testing.T) {
			sent, success := runFailureWorkflow(t, step, restfile.WorkflowOnFailureContinue)
			if len(sent) != 1 || !strings.Contains(sent[0], "Next") {
				t.Fatalf("on-failure=continue sent %v, want the next step to run", sent)
			}
			if success {
				t.Fatal("a continued failure still has to mark the run unsuccessful")
			}

			sent, success = runFailureWorkflow(t, step, restfile.WorkflowOnFailureStop)
			if len(sent) != 0 {
				t.Fatalf("on-failure=stop sent %v, want nothing after the failure", sent)
			}
			if success {
				t.Fatal("a stopped failure still has to mark the run unsuccessful")
			}
		})
	}
}

// Cancellation is not a step failure and no policy may override it.
func TestRunPlanCancellationStopsUnderContinue(t *testing.T) {
	pl, err := PrepareWorkflow(failureDoc(), restfile.Workflow{
		Name:             "demo",
		DefaultOnFailure: restfile.WorkflowOnFailureContinue,
		Steps: []restfile.WorkflowStep{
			{
				Kind:      restfile.WorkflowStepKindRequest,
				Name:      "First",
				Using:     "first",
				OnFailure: restfile.WorkflowOnFailureContinue,
			},
			nextStep(),
		},
	}, RunMeta{ID: "wf-1", Env: testEnvironment("dev")})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}

	var sent []string
	canceled := false
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case ReqStart:
			sent = append(sent, v.Req.Label)
		case RunDone:
			canceled = v.Canceled
		}
		return nil
	})

	if err := RunPlan(context.Background(), &fakeDep{execCanceled: true}, sink, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("cancellation sent %v, want the run to stop after the first step", sent)
	}
	if !canceled {
		t.Fatal("run was not reported as canceled")
	}
}

func TestRunPlanReportsCanceledRunContext(t *testing.T) {
	pl, err := PrepareWorkflow(failureDoc(), restfile.Workflow{
		Name: "demo",
		Steps: []restfile.WorkflowStep{
			{Kind: restfile.WorkflowStepKindRequest, Name: "First", Using: "first"},
			nextStep(),
		},
	}, RunMeta{ID: "wf-1", Env: testEnvironment("dev")})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var done []string
	canceled := false
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case WfStepDone:
			done = append(done, v.Step.Name)
		case RunDone:
			canceled = v.Canceled
		}
		return nil
	})

	if err := RunPlan(ctx, &fakeDep{execCanceled: true, onExec: cancel}, sink, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(done) != 1 || done[0] != "First" {
		t.Fatalf("finished steps = %v, want the canceled first step", done)
	}
	if !canceled {
		t.Fatal("run was not reported as canceled")
	}
}

// Steps built outside the parser carry no failure mode of their own, so the
// workflow default has to be filled in before the run starts.
func TestPrepareWorkflowInheritsDefaultOnFailure(t *testing.T) {
	pl, err := PrepareWorkflow(failureDoc(), restfile.Workflow{
		Name:             "demo",
		DefaultOnFailure: restfile.WorkflowOnFailureContinue,
		Steps:            []restfile.WorkflowStep{nextStep()},
	}, RunMeta{ID: "wf-1", Env: testEnvironment("dev")})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}
	if got := pl.Steps[0].Step.OnFailure; got != restfile.WorkflowOnFailureContinue {
		t.Fatalf("step on-failure = %q, want %q", got, restfile.WorkflowOnFailureContinue)
	}
}

// The parser keeps a branch whose run= or fail= was written with spaces, so the
// workflow keeps its shape. Such a branch has neither, and running it would
// skip the step instead of failing it.
func TestPrepareWorkflowRejectsBranchWithoutRunOrFail(t *testing.T) {
	ok := restfile.WorkflowIfBranch{Cond: "true", Run: "first"}
	steps := map[string]restfile.WorkflowStep{
		"if": {
			Kind: restfile.WorkflowStepKindIf,
			If:   &restfile.WorkflowIf{Then: restfile.WorkflowIfBranch{Cond: "true"}},
		},
		"elif": {
			Kind: restfile.WorkflowStepKindIf,
			If:   &restfile.WorkflowIf{Then: ok, Elifs: []restfile.WorkflowIfBranch{{Cond: "true"}}},
		},
		"else": {
			Kind: restfile.WorkflowStepKindIf,
			If:   &restfile.WorkflowIf{Then: ok, Else: &restfile.WorkflowIfBranch{}},
		},
		"case": {
			Kind:   restfile.WorkflowStepKindSwitch,
			Switch: &restfile.WorkflowSwitch{Expr: "a", Cases: []restfile.WorkflowSwitchCase{{Expr: "a"}}},
		},
		"default": {
			Kind: restfile.WorkflowStepKindSwitch,
			Switch: &restfile.WorkflowSwitch{
				Expr:    "a",
				Cases:   []restfile.WorkflowSwitchCase{{Expr: "a", Fail: "stop"}},
				Default: &restfile.WorkflowSwitchCase{Fail: " "},
			},
		},
	}
	for name, step := range steps {
		t.Run(name, func(t *testing.T) {
			_, err := PrepareWorkflow(failureDoc(), restfile.Workflow{
				Name:  "demo",
				Steps: []restfile.WorkflowStep{step},
			}, RunMeta{ID: "wf-1", Env: testEnvironment("dev")})
			if err == nil || !strings.Contains(err.Error(), "missing run or fail") {
				t.Fatalf("PrepareWorkflow error = %v, want a missing run or fail error", err)
			}
		})
	}
}
