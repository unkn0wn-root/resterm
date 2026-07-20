package rts

import (
	"reflect"
	"testing"
)

func TestToIfaceStrictConvertsDataAndRejectsOpaqueValues(t *testing.T) {
	value := Dict(map[string]Value{
		"null":   Null(),
		"bool":   Bool(true),
		"num":    Num(1.5),
		"str":    Str("text"),
		"list":   List([]Value{Num(1), Str("two")}),
		"nested": Dict(map[string]Value{"key": Str("value")}),
	})
	got, err := ToIfaceStrict(value)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"null":   nil,
		"bool":   true,
		"num":    1.5,
		"str":    "text",
		"list":   []any{float64(1), "two"},
		"nested": map[string]any{"key": "value"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToIfaceStrict() = %#v", got)
	}

	for name, opaque := range map[string]Value{
		"native": {K: VNative},
		"nested": Dict(map[string]Value{"fn": {K: VFunc}}),
		"listed": List([]Value{{K: VObj}}),
	} {
		if _, err := ToIfaceStrict(opaque); err == nil {
			t.Fatalf("ToIfaceStrict(%s) accepted an opaque value", name)
		}
	}
}
