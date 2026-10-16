package rts

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestFromIfaceChecksDictionaryKeyLimit(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 1, MaxDict: 8})
	_, err := FromIface(ctx, Pos{}, map[string]any{"ab": true})
	if err == nil || !strings.Contains(err.Error(), "string too long") {
		t.Fatalf("FromIface error = %v, want string limit", err)
	}
}

func TestFromIfaceValidatesDictionaryKeysDeterministically(t *testing.T) {
	ctx := NewCtx(context.Background(), Limits{MaxStr: 2, MaxDict: 8})
	in := map[string]any{"zzzz": true, "aaaa": true}
	for range 8 {
		_, err := FromIface(ctx, Pos{}, in)
		if err == nil || !strings.Contains(err.Error(), `dictionary key "aaaa": string too long`) {
			t.Fatalf("FromIface error = %v, want the first key in lexical order", err)
		}
	}
}

func doubled(n int) Value {
	v := List([]Value{Num(0)})
	for range n {
		v = List([]Value{v, v})
	}
	return v
}

// Shared children expand to 2^60 lists unless encoding stops at the size limit.
func TestToStrStopsAtStringLimit(t *testing.T) {
	ctx := NewCtx(t.Context(), Limits{MaxStr: defaultMaxStr})
	got, err := ToStr(ctx, Pos{}, doubled(2))
	if err != nil || got != "[[[0],[0]],[[0],[0]]]" {
		t.Fatalf("ToStr(doubled(2)) = %q, %v", got, err)
	}
	for _, v := range []Value{doubled(60), newResult(true, doubled(60), nil)} {
		if _, err := ToStr(ctx, Pos{}, v); err == nil || !strings.Contains(err.Error(), "string too long") {
			t.Fatalf("ToStr error = %v, want string too long", err)
		}
	}
}

func TestToStrWritesResultsAndRejectsNaN(t *testing.T) {
	ctx := NewCtx(t.Context(), Limits{MaxStr: defaultMaxStr})
	got, err := ToStr(ctx, Pos{}, newResult(false, Null(), errors.New("bad <id>")))
	if want := `{"error":"bad \u003cid\u003e","ok":false,"value":null}`; err != nil || got != want {
		t.Fatalf("ToStr(result) = %s, %v, want %s", got, err, want)
	}
	_, err = ToStr(ctx, Pos{}, List([]Value{Num(math.NaN())}))
	if err == nil || !strings.Contains(err.Error(), "json encode failed") {
		t.Fatalf("ToStr([NaN]) error = %v, want json encode failed", err)
	}
}

func TestToStrWritesNumbersAsJSONDoes(t *testing.T) {
	tests := []struct {
		n    float64
		want string
	}{
		{0, "0"},
		{math.Copysign(0, -1), "-0"},
		{1, "1"},
		{42.5, "42.5"},
		{-3.25, "-3.25"},
		{0.30000000000000004, "0.30000000000000004"},
		{1e6, "1000000"},
		{1234567, "1234567"},
		{1234567.89, "1234567.89"},
		{1792164490814, "1792164490814"},
		{1 << 53, "9007199254740992"},
		{1 << 60, "1152921504606847000"},
		{1e20, "100000000000000000000"},
		{1e21, "1e+21"},
		{0.000001, "0.000001"},
		{0.00001, "0.00001"},
		{1e-7, "1e-7"},
		{math.MaxFloat64, "1.7976931348623157e+308"},
		{math.NaN(), "NaN"},
		{math.Inf(1), "+Inf"},
		{math.Inf(-1), "-Inf"},
	}
	for _, tt := range tests {
		got, err := ToStr(nil, Pos{}, Num(tt.n))
		if err != nil || got != tt.want {
			t.Errorf("ToStr(%v) = %q, %v, want %q", tt.n, got, err, tt.want)
		}
	}
}
