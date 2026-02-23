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
			if got.PrimaryType != tc.want {
				t.Fatalf("unexpected inferred type: got %q want %q", got.PrimaryType, tc.want)
			}
		})
	}
}

func TestSchemaTypesFromStringsNormalizes(t *testing.T) {
	t.Parallel()

	got := SchemaTypesFromStrings([]string{" ARRAY ", "ObjEcT", "number", " null ", "garbage"})
	want := []SchemaType{TypeArray, TypeObject, TypeNumber, TypeNull}
	if len(got) != len(want) {
		t.Fatalf("unexpected type count: got %d want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected schema type at %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestInferSchemaTypeNullUnions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sch  *Schema
		def  SchemaType
		want SchemaType
	}{
		{
			name: "null then string uses string",
			sch:  &Schema{Types: []SchemaType{"null", "string"}},
			want: TypeString,
		},
		{
			name: "string then null uses string",
			sch:  &Schema{Types: []SchemaType{"string", "null"}},
			want: TypeString,
		},
		{
			name: "garbage then integer uses integer",
			sch:  &Schema{Types: []SchemaType{"garbage", "integer"}},
			want: TypeInteger,
		},
		{
			name: "null only returns null",
			sch:  &Schema{Types: []SchemaType{"null"}},
			want: TypeNull,
		},
		{
			name: "garbage and null returns null",
			sch:  &Schema{Types: []SchemaType{"garbage", "null"}},
			def:  TypeString,
			want: TypeNull,
		},
		{
			name: "garbage only falls back to default",
			sch:  &Schema{Types: []SchemaType{"garbage"}},
			def:  TypeString,
			want: TypeString,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := InferSchemaType(tc.sch, tc.def)
			if got.PrimaryType != tc.want {
				t.Fatalf("unexpected inferred type: got %q want %q", got.PrimaryType, tc.want)
			}
		})
	}
}

func TestInferSchemaType(t *testing.T) {
	t.Parallel()

	nullable := true
	tests := []struct {
		name string
		sch  *Schema
		def  SchemaType
		want SchemaTypeInfo
	}{
		{
			name: "nil schema uses default",
			def:  TypeString,
			want: SchemaTypeInfo{PrimaryType: TypeString},
		},
		{
			name: "null string union keeps string and marks nullable",
			sch:  &Schema{Types: []SchemaType{"null", "string"}},
			want: SchemaTypeInfo{PrimaryType: TypeString, Nullable: true, Explicit: true},
		},
		{
			name: "string null union keeps string and marks nullable",
			sch:  &Schema{Types: []SchemaType{"string", "null"}},
			want: SchemaTypeInfo{PrimaryType: TypeString, Nullable: true, Explicit: true},
		},
		{
			name: "null only remains explicit null",
			sch:  &Schema{Types: []SchemaType{"null"}},
			def:  TypeString,
			want: SchemaTypeInfo{PrimaryType: TypeNull, Nullable: true, Explicit: true},
		},
		{
			name: "oas30 nullable field sets nullable",
			sch:  &Schema{Types: []SchemaType{TypeInteger}, Nullable: &nullable},
			want: SchemaTypeInfo{PrimaryType: TypeInteger, Nullable: true, Explicit: true},
		},
		{
			name: "structural object fallback remains non-explicit",
			sch: &Schema{
				Properties: map[string]*SchemaRef{
					"id": {Node: &Schema{Types: []SchemaType{TypeString}}},
				},
			},
			want: SchemaTypeInfo{PrimaryType: TypeObject},
		},
		{
			name: "garbage and nullable keeps default plus nullable",
			sch:  &Schema{Types: []SchemaType{"garbage"}, Nullable: &nullable},
			def:  TypeNumber,
			want: SchemaTypeInfo{PrimaryType: TypeNumber, Nullable: true},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := InferSchemaType(tc.sch, tc.def)
			if got != tc.want {
				t.Fatalf("unexpected type info: got %#v want %#v", got, tc.want)
			}
		})
	}
}
