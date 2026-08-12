package native

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

func TestStringValues(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{
		MaxStr:  8,
		MaxList: 8,
	})
	call := Call{Ctx: ctx, Sig: "test(values)"}
	tests := []struct {
		name string
		in   rts.Value
		want []string
	}{
		{name: "one", in: rts.Str("a"), want: []string{"a"}},
		{name: "multiple", in: rts.List([]rts.Value{rts.Str("a"), rts.Str("b")}), want: []string{"a", "b"}},
		{name: "zero", in: rts.List([]rts.Value{}), want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := StringValues(call, tt.in)
			if err != nil {
				t.Fatalf("StringValues: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("StringValues() = %q, want %q", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("StringValues()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestStringValuesRejectsOtherShapes(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{})
	call := Call{Ctx: ctx, Sig: "test(values)"}
	tests := []struct {
		name string
		in   rts.Value
		want string
	}{
		{name: "null", in: rts.Null(), want: "expects string or list<string>"},
		{name: "number", in: rts.Num(1), want: "expects string or list<string>"},
		{name: "mixed list", in: rts.List([]rts.Value{rts.Str("a"), rts.Num(1)}), want: "expects list<string>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := StringValues(call, tt.in)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("StringValues error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEncodeStringsUsesCardinality(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{
		MaxStr:  8,
		MaxList: 8,
		MaxDict: 8,
	})
	pos := rts.Pos{}
	tests := []struct {
		name string
		in   []string
		kind rts.VKind
		n    int
	}{
		{name: "one", in: []string{"a"}, kind: rts.VStr},
		{name: "multiple", in: []string{"a", "b"}, kind: rts.VList, n: 2},
		{name: "zero", in: []string{}, kind: rts.VList},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeStrings(ctx, pos, tt.in)
			if err != nil {
				t.Fatalf("encodeStrings: %v", err)
			}
			if got.K != tt.kind || len(got.L) != tt.n {
				t.Fatalf("encodeStrings() = %+v, want kind %v and len %d", got, tt.kind, tt.n)
			}
		})
	}
}

func TestStringValueChecksLimit(t *testing.T) {
	ctx := rts.NewCtx(context.Background(), rts.Limits{MaxStr: 1})
	if _, err := StringValue(ctx, rts.Pos{}, "ab"); err == nil {
		t.Fatal("StringValue accepted an oversized string")
	}
}
