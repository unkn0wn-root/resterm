package generator

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

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
			schema := &openapi3.Schema{Type: &openapi3.Types{"string"}, Format: tc.format}
			ref := &model.SchemaRef{Payload: &openapi3.SchemaRef{Value: schema}}

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
