package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestEmitAllPreservesOrder(t *testing.T) {
	run := RunMeta{
		ID:   "run-1",
		Mode: ModeWorkflow,
		Name: "demo",
		Env:  "dev",
	}
	at := time.Date(2026, time.April, 13, 10, 11, 12, 0, time.UTC)
	es := []Evt{
		RunStart{Meta: NewMeta(run, at)},
		WfStepStart{
			Meta: NewMeta(run, at.Add(time.Second)),
			Step: StepMeta{
				Index:  0,
				Name:   "Login",
				Kind:   restfile.WorkflowStepKindRequest,
				Target: "login",
			},
		},
		ReqStart{
			Meta: NewMeta(run, at.Add(2*time.Second)),
			Req: ReqMeta{
				Index: 0,
				Label: "Login",
				Env:   "dev",
			},
		},
		RunDone{
			Meta:    NewMeta(run, at.Add(3*time.Second)),
			Success: true,
		},
	}

	var got []string
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch v := e.(type) {
		case RunStart:
			got = append(got, "run-start:"+v.Meta.Run.ID)
		case WfStepStart:
			got = append(got, "wf-step-start:"+v.Step.Name)
		case ReqStart:
			got = append(got, "req-start:"+v.Req.Label)
		case RunDone:
			got = append(got, "run-done:"+v.Meta.Run.Mode.String())
		default:
			t.Fatalf("unexpected event %T", e)
		}

		meta := MetaOf(e)
		if meta.Run.ID != "run-1" {
			t.Fatalf("expected run id run-1, got %q", meta.Run.ID)
		}
		if meta.Run.Mode != ModeWorkflow {
			t.Fatalf("expected mode workflow, got %v", meta.Run.Mode)
		}
		return nil
	})

	if err := EmitAll(context.Background(), sink, es...); err != nil {
		t.Fatalf("EmitAll: %v", err)
	}

	want := []string{
		"run-start:run-1",
		"wf-step-start:Login",
		"req-start:Login",
		"run-done:workflow",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d events, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestEmitAllStopsOnError(t *testing.T) {
	run := RunMeta{ID: "run-2", Mode: ModeCompare}
	es := []Evt{
		RunStart{Meta: NewMeta(run, time.Time{})},
		CmpRowStart{Meta: NewMeta(run, time.Time{}), Row: RowMeta{Index: 0, Env: "dev"}},
		CmpRowDone{Meta: NewMeta(run, time.Time{}), Row: RowMeta{Index: 0, Env: "dev"}},
	}

	var got []string
	wantErr := errors.New("boom")
	sink := SinkFunc(func(_ context.Context, e Evt) error {
		switch e.(type) {
		case RunStart:
			got = append(got, "run-start")
		case CmpRowStart:
			got = append(got, "cmp-row-start")
			return wantErr
		case CmpRowDone:
			got = append(got, "cmp-row-done")
		}
		return nil
	})

	err := EmitAll(context.Background(), sink, es...)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
	if len(got) != 2 {
		t.Fatalf("expected emission to stop after 2 events, got %v", got)
	}
	if got[0] != "run-start" || got[1] != "cmp-row-start" {
		t.Fatalf("unexpected order: %v", got)
	}
}

func TestEmitRespectsContextAndDiscard(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Emit(ctx, Discard, RunStart{
		Meta: NewMeta(RunMeta{ID: "run-3", Mode: ModeProfile}, time.Time{}),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if err := Emit(context.Background(), nil, RunDone{}); err != nil {
		t.Fatalf("expected nil sink to be ignored, got %v", err)
	}

	if err := Emit(context.Background(), SinkFunc(nil), RunDone{}); err != nil {
		t.Fatalf("expected nil SinkFunc to be ignored, got %v", err)
	}
}
