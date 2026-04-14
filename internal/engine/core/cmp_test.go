package core

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRunCompareEmitsRowsInOrderAndStopsOnCanceledResult(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/items",
		Metadata: restfile.RequestMetadata{
			Name: "items",
		},
	}
	pl, err := PrepareCompare(&restfile.Document{Path: "cmp.http"}, req, &restfile.CompareSpec{
		Environments: []string{"dev", "stage", "prod"},
		Baseline:     "dev",
	}, RunMeta{ID: "cmp-1", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareCompare: %v", err)
	}

	dep := &cmpDep{
		res: map[string]engine.RequestResult{
			"dev": {
				Response: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Duration:   5 * time.Millisecond,
				},
			},
			"stage": {
				Err: context.Canceled,
			},
			"prod": {
				Response: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Duration:   5 * time.Millisecond,
				},
			},
		},
	}

	var got []string
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case RunStart:
			got = append(got, "run-start:"+v.Meta.Run.Mode.String())
		case CmpRowStart:
			got = append(got, "row-start:"+v.Row.Env)
		case CmpRowDone:
			got = append(got, "row-done:"+v.Row.Env)
		case RunDone:
			got = append(got, "run-done:"+v.Meta.Run.Mode.String())
			if !v.Canceled {
				t.Fatalf("expected compare run to be canceled, got %+v", v)
			}
		default:
			t.Fatalf("unexpected event %T", e)
		}
		return nil
	})

	if err := RunCompare(context.Background(), dep, sink, pl); err != nil {
		t.Fatalf("RunCompare: %v", err)
	}

	want := []string{
		"run-start:compare",
		"row-start:dev",
		"row-done:dev",
		"row-start:stage",
		"row-done:stage",
		"run-done:compare",
	}
	if len(got) != len(want) {
		t.Fatalf("events: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: got %q want %q", i, got[i], want[i])
		}
	}
	if len(dep.call) != 2 {
		t.Fatalf("expected 2 compare calls, got %v", dep.call)
	}
	if dep.call[0] != "dev:false" || dep.call[1] != "stage:false" {
		t.Fatalf("unexpected compare calls %v", dep.call)
	}
}

func TestCompareBaselineHelpers(t *testing.T) {
	rows := []engine.CompareRow{
		{Environment: "one"},
		{Environment: "two"},
	}
	if got := CompareBaseIndex(nil, ""); got != 0 {
		t.Fatalf("expected empty rows base 0, got %d", got)
	}
	if got := CompareBaseline(nil, ""); got != "" {
		t.Fatalf("expected empty rows empty baseline, got %q", got)
	}
	if got := CompareBaseIndex(rows, ""); got != 0 {
		t.Fatalf("expected default base 0, got %d", got)
	}
	if got := CompareBaseline(rows, ""); got != "one" {
		t.Fatalf("expected default baseline one, got %q", got)
	}
	if got := CompareBaseIndex(rows, "two"); got != 1 {
		t.Fatalf("expected named base 1, got %d", got)
	}
	if got := CompareBaseline(rows, "two"); got != "two" {
		t.Fatalf("expected named baseline two, got %q", got)
	}
	if got := CompareBaseIndex(rows[:1], "missing"); got != 0 {
		t.Fatalf("expected missing base fallback 0, got %d", got)
	}
	if got := CompareBaseline(rows[:1], "missing"); got != "missing" {
		t.Fatalf("expected missing baseline to stay requested, got %q", got)
	}
}

func TestBuildCompareSpecNormalizesTargetsAndBaseline(t *testing.T) {
	spec := BuildCompareSpec([]string{" dev ", "DEV", "", "stage"}, "STAGE")
	if spec == nil {
		t.Fatalf("expected spec")
	}
	expect := []string{"dev", "stage"}
	if !reflect.DeepEqual(expect, spec.Environments) {
		t.Fatalf("unexpected environments: %#v", spec.Environments)
	}
	if spec.Baseline != "stage" {
		t.Fatalf("expected canonical baseline stage, got %q", spec.Baseline)
	}
}

func TestPrepareCompareNormalizesPlanAndPreservesMissingBaseline(t *testing.T) {
	req := &restfile.Request{Method: "GET", URL: "https://example.com"}
	pl, err := PrepareCompare(nil, req, &restfile.CompareSpec{
		Environments: []string{" dev ", "", "DEV", "stage"},
		Baseline:     "missing",
	}, RunMeta{})
	if err != nil {
		t.Fatalf("PrepareCompare: %v", err)
	}
	expect := []string{"dev", "stage"}
	if !reflect.DeepEqual(expect, pl.Spec.Environments) {
		t.Fatalf("unexpected environments: %#v", pl.Spec.Environments)
	}
	if pl.Spec.Baseline != "missing" {
		t.Fatalf("expected missing baseline to be preserved, got %q", pl.Spec.Baseline)
	}
}

type cmpDep struct {
	fakeDep
	res  map[string]engine.RequestResult
	call []string
}

func (d *cmpDep) ExecuteWith(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	opt request.ExecOptions,
) (engine.RequestResult, error) {
	d.call = append(d.call, env+":"+boolString(opt.Record))
	out, ok := d.res[env]
	if !ok {
		return engine.RequestResult{}, nil
	}
	out.Executed = request.CloneRequest(req)
	out.RequestText = request.RenderRequestText(req)
	out.Environment = env
	return out, nil
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
