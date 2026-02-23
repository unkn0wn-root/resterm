package generator

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

func TestExampleBuilderStringFormatsDeterministic(t *testing.T) {
	t.Parallel()

	builder := NewExampleBuilder()

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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sch := &model.Schema{Types: []string{"string"}, Format: tc.format}
			ref := &model.SchemaRef{Node: sch}

			value, ok := builder.FromSchema(ref)
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

			value2, ok := builder.FromSchema(ref)
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

func TestExampleBuilderHandlesRecursiveSchema(t *testing.T) {
	t.Parallel()

	ref := &model.SchemaRef{
		Node: &model.Schema{
			Types:      []string{"object"},
			Properties: map[string]*model.SchemaRef{},
		},
	}
	ref.Node.Properties["next"] = ref

	builder := NewExampleBuilder()
	got, ok := builder.FromSchema(ref)
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
