package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

func TestWorkflowRunVarFailureShowsFailedRun(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := parser.Parse("run.http", []byte(`# @workflow broken
# @run var suffix = {{$randomInt(x)}}
# @step Create using=Create

### Create
# @name Create
POST https://example.invalid/{{suffix}}
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %v", doc.Errors)
	}
	pl, err := core.PrepareWorkflow(doc, doc.Workflows[0], core.RunMeta{ID: "wf-run-var", Env: testEnv("dev")})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}
	m.workflowRun = workflowStateFromPlan(pl)

	var evts []core.Evt
	sink := core.SinkFunc(func(_ context.Context, e core.Evt) error {
		evts = append(evts, e)
		return nil
	})
	if err := core.RunPlan(context.Background(), m.requestSvc(m.runOptions()), sink, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	for _, e := range evts {
		applyRunEvt(t, &m, e)
	}

	msg := m.statusMessage
	if msg.level != statusWarn || !strings.Contains(msg.text, "failed at step Create: @run var suffix: ") {
		t.Fatalf("status = %+v, want the failed @run var step", msg)
	}
}
