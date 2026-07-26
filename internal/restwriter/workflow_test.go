package restwriter

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRenderWorkflowDeterministic(t *testing.T) {
	wf := restfile.Workflow{
		Name:             "demo",
		Description:      "first\nsecond",
		Tags:             []string{"smoke", "fast"},
		DefaultOnFailure: restfile.WorkflowOnFailureContinue,
		Options: map[string]string{
			"vars.z": "last",
			"vars.a": "hello world",
			"other":  "ignored",
		},
		Steps: []restfile.WorkflowStep{{
			Name:      "Fetch",
			Using:     "request one",
			OnFailure: restfile.WorkflowOnFailureStop,
			When:      &restfile.ConditionSpec{Expression: "ready"},
			Expect: restfile.WorkflowExpect{
				Status: "200 OK",
				Extra:  map[string]string{"b": "two", "a": "one"},
			},
			Vars:    map[string]string{"vars.z": "z", "vars.a": "a value"},
			Options: map[string]string{"z": "two", "a": "one"},
		}},
	}

	want := strings.Join([]string{
		`# @workflow demo on-failure=continue vars.a="hello world" vars.z=last`,
		"# @description first",
		"# @description second",
		"# @tag smoke fast",
		"# @when ready",
		`# @step Fetch using="request one" on-failure=stop expect.status="200 OK" expect.a=one expect.b=two vars.a="a value" vars.z=z a=one z=two`,
	}, "\n")

	for range 20 {
		if got := RenderWorkflow(wf, "fallback"); got != want {
			t.Fatalf("RenderWorkflow() mismatch:\nwant:\n%s\n\ngot:\n%s", want, got)
		}
	}
}

func TestRenderIncludesWorkflows(t *testing.T) {
	got, err := Render(&restfile.Document{
		Workflows: []restfile.Workflow{{
			Name:  "demo",
			Steps: []restfile.WorkflowStep{{Using: "request"}},
		}},
	}, Options{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(got, "# @workflow demo\n# @step using=request") {
		t.Fatalf("rendered document missing workflow:\n%s", got)
	}
}

// The writer quotes any value holding a space, so the reader has to keep it in
// one piece. Branches used to come back with the tail of a quoted value parsed
// as extra options and the quotes stripped off the expression.
func TestRenderWorkflowBranchRoundTrip(t *testing.T) {
	wf := restfile.Workflow{
		Name: "deploy",
		Steps: []restfile.WorkflowStep{
			{
				Kind: restfile.WorkflowStepKindIf,
				If: &restfile.WorkflowIf{
					Then: restfile.WorkflowIfBranch{
						Cond: `response.body.name == "John Doe"`,
						Run:  "create",
					},
					Else: &restfile.WorkflowIfBranch{Fail: "nothing matched"},
				},
			},
			{
				Kind: restfile.WorkflowStepKindSwitch,
				Switch: &restfile.WorkflowSwitch{
					Expr:  "response.body.role",
					Cases: []restfile.WorkflowSwitchCase{{Expr: `"site admin"`, Fail: "not allowed here"}},
				},
			},
		},
	}

	src := RenderWorkflow(wf, "")
	doc := parser.Parse("workflow.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("rendered workflow did not parse: %v\n%s", doc.Errors, src)
	}
	if len(doc.Workflows) != 1 || len(doc.Workflows[0].Steps) != 2 {
		t.Fatalf("parsed %#v, want 1 workflow with 2 steps\n%s", doc.Workflows, src)
	}

	got := doc.Workflows[0].Steps
	if branch := got[0].If; branch == nil ||
		branch.Then.Cond != wf.Steps[0].If.Then.Cond ||
		branch.Then.Run != "create" ||
		branch.Else == nil ||
		branch.Else.Fail != "nothing matched" {
		t.Fatalf("@if changed after round trip:\nwant: %#v\ngot:  %#v\n%s", wf.Steps[0].If, branch, src)
	}
	if sw := got[1].Switch; sw == nil ||
		len(sw.Cases) != 1 ||
		sw.Cases[0].Expr != `"site admin"` ||
		sw.Cases[0].Fail != "not allowed here" {
		t.Fatalf("@switch changed after round trip:\nwant: %#v\ngot:  %#v\n%s", wf.Steps[1].Switch, sw, src)
	}
}

func TestRenderWorkflowRoundTrip(t *testing.T) {
	code := 201
	wf := restfile.Workflow{
		Name:             "deploy",
		Description:      "deploy service",
		Tags:             []string{"smoke", "release"},
		DefaultOnFailure: restfile.WorkflowOnFailureContinue,
		Options:          map[string]string{"vars.workflow.region": "eu north"},
		Steps: []restfile.WorkflowStep{{
			Name:      "Create",
			Using:     "create",
			OnFailure: restfile.WorkflowOnFailureStop,
			When:      &restfile.ConditionSpec{Expression: "ready"},
			Expect: restfile.WorkflowExpect{
				StatusCode: &code,
				Extra:      map[string]string{"body": "created"},
			},
			Vars: map[string]string{"vars.workflow.id": "service one"},
		}},
	}

	src := RenderWorkflow(wf, "")
	doc := parser.Parse("workflow.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("rendered workflow did not parse: %v\n%s", doc.Errors, src)
	}
	if len(doc.Workflows) != 1 {
		t.Fatalf("parsed %d workflows, want 1\n%s", len(doc.Workflows), src)
	}
	got := doc.Workflows[0]
	if got.Name != wf.Name ||
		got.Description != wf.Description ||
		strings.Join(got.Tags, ",") != strings.Join(wf.Tags, ",") ||
		got.DefaultOnFailure != wf.DefaultOnFailure ||
		got.Options["vars.workflow.region"] != "eu north" ||
		len(got.Steps) != 1 {
		t.Fatalf("workflow metadata changed after round trip:\nwant: %#v\ngot:  %#v", wf, got)
	}
	step := got.Steps[0]
	if step.Name != "Create" ||
		step.Using != "create" ||
		step.OnFailure != restfile.WorkflowOnFailureStop ||
		step.When == nil ||
		step.When.Expression != "ready" ||
		step.Expect.StatusCode == nil ||
		*step.Expect.StatusCode != code ||
		step.Expect.Extra["body"] != "created" ||
		step.Vars["vars.workflow.id"] != "service one" {
		t.Fatalf("workflow step changed after round trip:\nwant: %#v\ngot:  %#v", wf.Steps[0], step)
	}
}
