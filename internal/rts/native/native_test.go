package native

import (
	"context"
	"errors"
	"slices"
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

func TestListOfDecodesElements(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{
		MaxStr:  8,
		MaxList: 8,
	})
	call := Call{Ctx: ctx, Sig: "test(values)"}
	decode := ListOf(String)
	tests := []struct {
		name string
		in   rts.Value
		want []string
	}{
		{name: "empty", in: rts.List([]rts.Value{}), want: []string{}},
		{name: "one", in: rts.List([]rts.Value{rts.Str("a")}), want: []string{"a"}},
		{name: "multiple", in: rts.List([]rts.Value{rts.Str("a"), rts.Str("b")}), want: []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decode(call, tt.in)
			if err != nil {
				t.Fatalf("ListOf(String): %v", err)
			}
			if got == nil {
				t.Fatal("ListOf(String) returned a nil slice")
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("ListOf(String) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListOfRejectsInvalidInput(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{
		MaxStr:  1,
		MaxList: 1,
	})
	call := Call{Ctx: ctx, Sig: "test(values)"}
	decode := ListOf(String)
	tests := []struct {
		name string
		in   rts.Value
		want string
	}{
		{name: "not a list", in: rts.Str("a"), want: "expects list"},
		{name: "list too large", in: rts.List([]rts.Value{rts.Str("a"), rts.Str("b")}), want: "list too large"},
		{name: "wrong element type", in: rts.List([]rts.Value{rts.Num(1)}), want: "expects string"},
		{name: "element exceeds limit", in: rts.List([]rts.Value{rts.Str("ab")}), want: "string too long"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decode(call, tt.in)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ListOf(String) error = %v, want %q", err, tt.want)
			}
			if got != nil {
				t.Fatalf("ListOf(String) = %q after error, want nil", got)
			}
		})
	}
}

func TestListOfChecksOuterLimitBeforeElements(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxList: 1})
	call := Call{Ctx: ctx, Sig: "test(values)"}
	called := false
	decode := ListOf(func(_ Call, _ rts.Value) (string, error) {
		called = true
		return "", nil
	})

	_, err := decode(call, rts.List([]rts.Value{rts.Str("a"), rts.Str("b")}))
	if err == nil || !strings.Contains(err.Error(), "list too large") {
		t.Fatalf("ListOf error = %v, want list limit error", err)
	}
	if called {
		t.Fatal("ListOf decoded an element after the outer list failed validation")
	}
}

func TestStringListReportsCollectionType(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{})
	call := Call{Ctx: ctx, Sig: "test(values)"}

	_, err := StringList(call, rts.List([]rts.Value{rts.Num(1)}))
	if err == nil || !strings.Contains(err.Error(), "expects list<string>") {
		t.Fatalf("StringList error = %v, want list<string> error", err)
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
