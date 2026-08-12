package native

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

func TestFn2ChecksArityBeforeDecoding(t *testing.T) {
	called := false
	def := Fn2("test.join", "test.join(a, b)", String, String,
		func(_ Call, a, b string) (rts.Value, error) {
			called = true
			return rts.Str(a + b), nil
		},
	)
	ctx := rts.NewCtx(context.Background(), rts.Limits{})
	_, err := def.Func()(ctx, rts.Pos{Line: 2, Col: 3}, []rts.Value{rts.Num(1)})
	if err == nil || !strings.Contains(err.Error(), "expects 2 arguments, got 1") {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("implementation ran after an arity error")
	}
}

func TestStringIsStrict(t *testing.T) {
	def := Fn1("test.echo", "test.echo(value)", String,
		func(_ Call, value string) (rts.Value, error) { return rts.Str(value), nil },
	)
	ctx := rts.NewCtx(context.Background(), rts.Limits{})
	_, err := def.Func()(ctx, rts.Pos{}, []rts.Value{rts.Num(1)})
	if err == nil || !strings.Contains(err.Error(), "expects string") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValueAddsNamedNativeFrame(t *testing.T) {
	def := Fn0("test.fail", "test.fail()", func(call Call) (rts.Value, error) {
		return rts.Null(), call.Errorf("failed")
	})
	ctx := rts.NewCtx(context.Background(), rts.Limits{})
	_, err := def.Value().NF(ctx, rts.Pos{}, nil)
	var stack *rts.StackError
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !errors.As(err, &stack) || !strings.Contains(stack.Pretty(), "test.fail") {
		t.Fatalf("missing native frame: %v", err)
	}
}
