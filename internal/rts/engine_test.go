package rts_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/stdlib"
)

// A host function may evaluate again while its caller runs, as a request
// getter does when it renders a {{= }} expression.
func TestEngHostFunctionCanEvaluateAgain(t *testing.T) {
	e := rts.NewEng(stdlib.New)
	nested := rts.NativeNamed("nested", func(cx *rts.Ctx, _ rts.Pos, _ []rts.Value) (rts.Value, error) {
		return e.Eval(cx.Ctx, rts.EvalConfig{}, "1 + 2", rts.Pos{})
	})
	cfg := rts.EvalConfig{Bindings: []rts.Extensions{rts.Extension("nested", nested)}}

	done := make(chan error, 1)
	go func() {
		got, err := e.EvalStr(context.Background(), cfg, "nested()", rts.Pos{})
		if err == nil && got != "3" {
			t.Errorf("nested() = %q, want 3", got)
		}
		if err == nil {
			_, err = e.ExecModule(context.Background(), cfg, "let x = nested()", rts.Pos{})
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("nested evaluation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nested evaluation waited on the engine lock")
	}
}

// Evaluations no longer hold the lock while they run, so they may overlap.
func TestEngEvaluatesInParallel(t *testing.T) {
	e := rts.NewEng(stdlib.New)
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				got, err := e.EvalStr(ctx, rts.EvalConfig{}, `json.stringify({"n": len([1, 2])})`, rts.Pos{})
				if err != nil || got != `{"n":2}` {
					t.Errorf("EvalStr = %q, %v", got, err)
					return
				}
				if _, err := e.ExecModule(
					ctx,
					rts.EvalConfig{},
					"let d = {a: 1}\nlet s = str(d.a)",
					rts.Pos{},
				); err != nil {
					t.Errorf("ExecModule: %v", err)
					return
				}
			}
		})
	}
	wg.Wait()
}
