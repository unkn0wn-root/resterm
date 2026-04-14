package core

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func TestRunPlanWorkflowEmitsBranchAndLoopEventsInOrder(t *testing.T) {
	doc := &restfile.Document{
		Path: "demo.http",
		Variables: []restfile.Variable{{
			Name:  "items",
			Value: `["a","b"]`,
		}},
		Requests: []*restfile.Request{
			{
				Method: "GET",
				URL:    "https://example.com/login",
				Metadata: restfile.RequestMetadata{
					Name: "login",
				},
			},
			{
				Method: "GET",
				URL:    "https://example.com/items/{{vars.request.item}}",
				Metadata: restfile.RequestMetadata{
					Name: "loop",
				},
			},
		},
	}
	pl, err := PrepareWorkflow(doc, restfile.Workflow{
		Name: "demo",
		Steps: []restfile.WorkflowStep{
			{
				Kind: restfile.WorkflowStepKindIf,
				Name: "Choose",
				If: &restfile.WorkflowIf{
					Then: restfile.WorkflowIfBranch{
						Cond: "true",
						Run:  "login",
					},
				},
			},
			{
				Kind:  restfile.WorkflowStepKindForEach,
				Name:  "Each",
				Using: "loop",
				ForEach: &restfile.WorkflowForEach{
					Expr: "items",
					Var:  "item",
				},
			},
		},
	}, RunMeta{ID: "wf-1", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}

	dep := &fakeDep{
		each: map[string][]rts.Value{
			"items": {rts.Str("a"), rts.Str("b")},
		},
	}

	var got []string
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case RunStart:
			got = append(got, "run-start:"+v.Meta.Run.Mode.String())
		case WfStepStart:
			got = append(got, "wf-step-start:"+stepName(v.Step))
		case ReqStart:
			got = append(got, "req-start:"+v.Req.Label)
		case ReqDone:
			got = append(got, "req-done:"+v.Req.Label)
		case WfStepDone:
			got = append(got, "wf-step-done:"+stepName(v.Step))
		case RunDone:
			got = append(got, "run-done:"+v.Meta.Run.Mode.String())
			if !v.Success {
				t.Fatalf("expected workflow success, got %+v", v)
			}
		default:
			t.Fatalf("unexpected event %T", e)
		}
		return nil
	})

	if err := RunPlan(context.Background(), dep, sink, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}

	want := []string{
		"run-start:workflow",
		"wf-step-start:Choose -> login",
		"req-start:Choose -> login",
		"req-done:Choose -> login",
		"wf-step-done:Choose -> login",
		"wf-step-start:Each (1/2)",
		"req-start:Each (1/2)",
		"req-done:Each (1/2)",
		"wf-step-done:Each (1/2)",
		"wf-step-start:Each (2/2)",
		"req-start:Each (2/2)",
		"req-done:Each (2/2)",
		"wf-step-done:Each (2/2)",
		"run-done:workflow",
	}
	if len(got) != len(want) {
		t.Fatalf("events: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestRunPlanForEachMarksRequestRunsRecorded(t *testing.T) {
	doc := &restfile.Document{
		Path: "each.http",
		Variables: []restfile.Variable{{
			Name:  "items",
			Value: `["x","y"]`,
		}},
	}
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/items/{{vars.request.item}}",
		Metadata: restfile.RequestMetadata{
			Name: "each",
			ForEach: &restfile.ForEachSpec{
				Expression: "items",
				Var:        "item",
			},
		},
	}

	pl, err := PrepareForEach(doc, req, RunMeta{ID: "each-1", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareForEach: %v", err)
	}

	dep := &fakeDep{
		each: map[string][]rts.Value{
			"items": {rts.Str("x"), rts.Str("y")},
		},
	}
	var modes []string
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case RunStart:
			modes = append(modes, v.Meta.Run.Mode.String())
		case RunDone:
			modes = append(modes, v.Meta.Run.Mode.String())
		}
		return nil
	})

	if err := RunPlan(context.Background(), dep, sink, pl); err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if len(dep.rec) != 2 {
		t.Fatalf("expected 2 recorded request runs, got %v", dep.rec)
	}
	for i, rec := range dep.rec {
		if !rec {
			t.Fatalf("expected request %d to record history", i)
		}
	}
	if strings.Join(modes, ",") != "for-each,for-each" {
		t.Fatalf("unexpected run modes %v", modes)
	}
}

func TestRunPlanExecutionErrorMarksRunDoneUnsuccessful(t *testing.T) {
	doc := &restfile.Document{
		Path: "err.http",
		Requests: []*restfile.Request{{
			Method: "GET",
			URL:    "https://example.com/fail",
			Metadata: restfile.RequestMetadata{
				Name: "fail",
			},
		}},
	}
	pl, err := PrepareWorkflow(doc, restfile.Workflow{
		Name: "err",
		Steps: []restfile.WorkflowStep{{
			Kind:  restfile.WorkflowStepKindRequest,
			Using: "fail",
		}},
	}, RunMeta{ID: "wf-err", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}

	wantErr := errors.New("transport failed")
	dep := &fakeDep{execErr: wantErr}

	var done RunDone
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		if evt, ok := e.(RunDone); ok {
			done = evt
		}
		return nil
	})

	err = RunPlan(context.Background(), dep, sink, pl)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected execution error %v, got %v", wantErr, err)
	}
	if done.Success {
		t.Fatalf("expected run-done to be unsuccessful, got %+v", done)
	}
	if done.Canceled || done.Skipped {
		t.Fatalf("expected plain failure without canceled/skipped, got %+v", done)
	}
}

type fakeDep struct {
	rec     []bool
	each    map[string][]rts.Value
	execErr error
}

func (d *fakeDep) CollectVariables(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	extra ...map[string]string,
) map[string]string {
	out := make(map[string]string)
	if doc != nil {
		for _, v := range doc.Variables {
			out[v.Name] = v.Value
		}
	}
	for _, x := range extra {
		for k, v := range x {
			out[k] = v
		}
	}
	return out
}

func (d *fakeDep) ExecuteWith(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	opt request.ExecOptions,
) (engine.RequestResult, error) {
	d.rec = append(d.rec, opt.Record)
	if d.execErr != nil {
		return engine.RequestResult{}, d.execErr
	}
	body := `{"ok":true}`
	if req != nil && strings.Contains(req.URL, "items") {
		body = `{"item":true}`
	}
	return engine.RequestResult{
		Response: &httpclient.Response{
			Status:       "200 OK",
			StatusCode:   http.StatusOK,
			Body:         []byte(body),
			Duration:     5 * time.Millisecond,
			EffectiveURL: req.URL,
		},
		Executed:    req,
		RequestText: request.RenderRequestText(req),
		Environment: env,
	}, nil
}

func (d *fakeDep) EvalCondition(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	base string,
	spec *restfile.ConditionSpec,
	vv map[string]string,
	extra map[string]rts.Value,
) (bool, string, error) {
	if spec == nil || strings.TrimSpace(spec.Expression) == "" {
		return true, "", nil
	}
	switch strings.TrimSpace(spec.Expression) {
	case "true":
		return true, "", nil
	case "false":
		return false, "@when evaluated to false: false", nil
	default:
		return false, "", errors.New("unsupported condition")
	}
}

func (d *fakeDep) EvalForEachItems(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	base string,
	spec request.ForEachSpec,
	vv map[string]string,
	extra map[string]rts.Value,
) ([]rts.Value, error) {
	if items := d.each[strings.TrimSpace(spec.Expr)]; len(items) > 0 {
		return append([]rts.Value(nil), items...), nil
	}
	return nil, errors.New("missing list")
}

func (d *fakeDep) EvalValue(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	base string,
	expr string,
	site string,
	pos rts.Pos,
	vv map[string]string,
	extra map[string]rts.Value,
) (rts.Value, error) {
	switch strings.TrimSpace(expr) {
	case "true":
		return rts.Bool(true), nil
	case "false":
		return rts.Bool(false), nil
	default:
		return rts.Str(expr), nil
	}
}

func (d *fakeDep) PosForLine(doc *restfile.Document, req *restfile.Request, line int) rts.Pos {
	return rts.Pos{Path: "test", Line: line, Col: 1}
}

func (d *fakeDep) ValueString(ctx context.Context, pos rts.Pos, v rts.Value) (string, error) {
	if v.K == rts.VStr {
		return v.S, nil
	}
	return "", errors.New("not a string")
}

func stepName(meta StepMeta) string {
	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = "step"
	}
	if meta.Branch != "" {
		name += " -> " + meta.Branch
	}
	if meta.Iter > 0 && meta.Total > 0 {
		name += " (" + strconv.Itoa(meta.Iter) + "/" + strconv.Itoa(meta.Total) + ")"
	}
	return name
}
