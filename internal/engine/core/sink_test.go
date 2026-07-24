package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
