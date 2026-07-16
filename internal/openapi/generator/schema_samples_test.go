package generator

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

func TestSchemaSamplerStringFormatsDeterministic(t *testing.T) {
	t.Parallel()

	sampler := newSchemaSampler()

	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{name: "date", format: "date", expected: sampleDateValue},
		{name: "date-time", format: "date-time", expected: sampleDateTimeValue},
		{name: "datetime alias", format: "datetime", expected: sampleDateTimeValue},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sch := &model.Schema{Types: []model.SchemaType{model.TypeString}, Format: tc.format}
			ref := &model.SchemaRef{Node: sch}

			value, ok := sampler.sample(ref, sampleAll)
			if !ok {
				t.Fatalf("expected example for format %s", tc.format)
			}

			got, ok := value.(string)
			if !ok {
				t.Fatalf("expected string example, got %T", value)
			}

			if got != tc.expected {
				t.Fatalf("unexpected example for %s: %s", tc.format, got)
			}

			value2, ok := sampler.sample(ref, sampleAll)
			if !ok {
				t.Fatalf("second retrieval failed for %s", tc.format)
			}
			got2, ok := value2.(string)
			if !ok {
				t.Fatalf("expected string on second retrieval for %s, got %T", tc.format, value2)
			}
			if got2 != tc.expected {
				t.Fatalf("non-deterministic example for %s: %s", tc.format, got2)
			}
		})
	}
}

func TestSchemaSamplerHandlesRecursiveSchema(t *testing.T) {
	t.Parallel()

	ref := &model.SchemaRef{
		Node: &model.Schema{
			Types:      []model.SchemaType{model.TypeObject},
			Properties: map[string]*model.SchemaRef{},
		},
	}
	ref.Node.Properties["next"] = ref

	sampler := newSchemaSampler()
	got, ok := sampler.sample(ref, sampleAll)
	if !ok {
		t.Fatalf("expected example for recursive schema")
	}

	obj, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object example, got %T", got)
	}
	next, ok := obj["next"]
	if !ok {
		t.Fatalf("expected next property in example")
	}
	if _, ok := next.(map[string]any); !ok {
		t.Fatalf("expected next fallback object, got %T", next)
	}
}

func TestSchemaSamplerNullTypeProducesNull(t *testing.T) {
	t.Parallel()

	sampler := newSchemaSampler()
	ref := &model.SchemaRef{
		Node: &model.Schema{
			Types: []model.SchemaType{model.TypeNull},
		},
	}

	got, ok := sampler.sample(ref, sampleAll)
	if !ok {
		t.Fatalf("expected sample for null schema")
	}
	if got != nil {
		t.Fatalf("expected nil sample for null schema, got %T (%v)", got, got)
	}
}

func TestSchemaSamplerNullStringUnionPrefersConcreteType(t *testing.T) {
	t.Parallel()

	sampler := newSchemaSampler()
	ref := &model.SchemaRef{
		Node: &model.Schema{
			Types: []model.SchemaType{model.TypeNull, model.TypeString},
		},
	}

	got, ok := sampler.sample(ref, sampleAll)
	if !ok {
		t.Fatalf("expected sample for null/string union")
	}
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string sample for null/string union, got %T", got)
	}
	if s == "" {
		t.Fatalf("expected non-empty string sample for null/string union")
	}
}

func TestSchemaSamplerRespectsReadWriteVisibility(t *testing.T) {
	t.Parallel()

	readOnly := true
	writeOnly := true
	ref := &model.SchemaRef{Node: &model.Schema{
		Types: []model.SchemaType{model.TypeObject},
		Properties: map[string]*model.SchemaRef{
			"id": {
				Node: &model.Schema{Types: []model.SchemaType{model.TypeString}, ReadOnly: &readOnly},
			},
			"name": {
				Node: &model.Schema{Types: []model.SchemaType{model.TypeString}},
			},
			"password": {
				Node: &model.Schema{Types: []model.SchemaType{model.TypeString}, WriteOnly: &writeOnly},
			},
		},
	}}

	tests := []struct {
		name     string
		context  sampleContext
		included string
		excluded string
	}{
		{
			name:     "request omits read-only properties",
			context:  sampleRequest,
			included: "password",
			excluded: "id",
		},
		{
			name:     "response omits write-only properties",
			context:  sampleResponse,
			included: "id",
			excluded: "password",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := newSchemaSampler().sample(ref, tc.context)
			if !ok {
				t.Fatal("expected object sample")
			}
			object, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("sample type = %T, want map[string]any", value)
			}
			if _, ok := object["name"]; !ok {
				t.Fatalf("ordinary property missing: %#v", object)
			}
			if _, ok := object[tc.included]; !ok {
				t.Fatalf("property %q missing: %#v", tc.included, object)
			}
			if _, ok := object[tc.excluded]; ok {
				t.Fatalf("property %q should be omitted: %#v", tc.excluded, object)
			}
		})
	}
}
