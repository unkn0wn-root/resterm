package model

import "testing"

func TestInferSchemaTypeCaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   SchemaType
		want SchemaType
	}{
		{name: "array uppercase", in: "ARRAY", want: TypeArray},
		{name: "object mixed case", in: "ObjEcT", want: TypeObject},
		{name: "number padded", in: "  Number  ", want: TypeNumber},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := InferSchemaType(&Schema{Types: []SchemaType{tc.in}}, "")
			if got != tc.want {
				t.Fatalf("unexpected inferred type: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSchemaTypesFromStringsNormalizes(t *testing.T) {
	t.Parallel()

	got := SchemaTypesFromStrings([]string{" ARRAY ", "ObjEcT", "number"})
	want := []SchemaType{TypeArray, TypeObject, TypeNumber}
	if len(got) != len(want) {
		t.Fatalf("unexpected type count: got %d want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected schema type at %d: got %q want %q", i, got[i], want[i])
		}
	}
}
