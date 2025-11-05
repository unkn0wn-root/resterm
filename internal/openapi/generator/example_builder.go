package generator

import (
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

const (
	sampleDateValue     = "2000-01-02"
	sampleDateTimeValue = "2000-01-02T15:04:05Z"
)

type ExampleBuilder struct {
	maxDepth int
}

// NewExampleBuilder creates an example builder with a sane recursion depth.
func NewExampleBuilder() *ExampleBuilder {
	return &ExampleBuilder{maxDepth: 6}
}

// FromSchema produces an example value for the provided schema reference.
func (b *ExampleBuilder) FromSchema(ref *model.SchemaRef) (any, bool) {
	if ref == nil {
		return nil, false
	}
	raw, ok := ref.Payload.(*openapi3.SchemaRef)
	if !ok {
		return nil, false
	}
	return b.build(raw, 0)
}

// build recursively chooses example values honoring explicit samples and basic
// type hints while preventing infinite recursion via maxDepth.
func (b *ExampleBuilder) build(ref *openapi3.SchemaRef, depth int) (any, bool) {
	if ref == nil || ref.Value == nil {
		return nil, false
	}
	if depth >= b.maxDepth {
		return nil, false
	}
	schema := ref.Value
	if schema.Example != nil {
		return schema.Example, true
	}
	if schema.Default != nil {
		return schema.Default, true
	}
	if len(schema.Enum) > 0 {
		return schema.Enum[0], true
	}

	if len(schema.OneOf) > 0 {
		if value, ok := b.build(schema.OneOf[0], depth+1); ok {
			return value, true
		}
	}
	if len(schema.AnyOf) > 0 {
		if value, ok := b.build(schema.AnyOf[0], depth+1); ok {
			return value, true
		}
	}
	if len(schema.AllOf) > 0 {
		composed := make(map[string]any)
		for _, candidate := range schema.AllOf {
			value, ok := b.build(candidate, depth+1)
			if !ok {
				continue
			}
			if fragment, ok := value.(map[string]any); ok {
				for k, v := range fragment {
					composed[k] = v
				}
			}
		}
		if len(composed) > 0 {
			return composed, true
		}
	}

	types := schema.Type.Slice()
	if len(types) == 0 {
		// assume object if properties defined, otherwise fallback to string
		if len(schema.Properties) > 0 || schema.AdditionalProperties.Schema != nil {
			types = []string{"object"}
		} else if schema.Items != nil {
			types = []string{"array"}
		} else {
			types = []string{"string"}
		}
	}

	switch types[0] {
	case "string":
		return exampleForString(schema), true
	case "integer":
		return exampleForInteger(schema), true
	case "number":
		return exampleForNumber(schema), true
	case "boolean":
		return false, true
	case "array":
		if schema.Items == nil {
			return []any{}, true
		}
		value, ok := b.build(schema.Items, depth+1)
		if !ok {
			value = defaultForType(schema.Items)
		}
		return []any{value}, true
	case "object":
		return b.exampleForObject(schema, depth+1)
	default:
		return nil, false
	}
}

// exampleForObject iterates the object properties and generates nested samples.
func (b *ExampleBuilder) exampleForObject(schema *openapi3.Schema, depth int) (any, bool) {
	if schema == nil {
		return nil, false
	}
	result := make(map[string]any)

	keys := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		prop := schema.Properties[name]
		if prop == nil {
			continue
		}
		value, ok := b.build(prop, depth)
		if !ok {
			value = defaultForType(prop)
		}
		result[name] = value
	}

	if schema.AdditionalProperties.Schema != nil {
		value, ok := b.build(schema.AdditionalProperties.Schema, depth)
		if !ok {
			value = defaultForType(schema.AdditionalProperties.Schema)
		}
		result["additionalProperty"] = value
	}

	if len(result) == 0 {
		return map[string]any{}, true
	}
	return result, true
}

// exampleForString returns a representative string honoring format hints and
// enum values when present.
func exampleForString(schema *openapi3.Schema) string {
	if schema == nil {
		return ""
	}
	switch strings.ToLower(schema.Format) {
	case "uuid":
		return "00000000-0000-4000-8000-000000000000"
	case "date":
		return sampleDateValue
	case "date-time", "datetime":
		return sampleDateTimeValue
	case "email":
		return "user@example.com"
	case "uri", "url":
		return "https://example.com/resource"
	case "hostname":
		return "example.com"
	case "ipv4":
		return "127.0.0.1"
	case "ipv6":
		return "2001:db8::1"
	}
	if len(schema.Enum) > 0 {
		if value, ok := schema.Enum[0].(string); ok {
			return value
		}
	}
	if schema.Pattern != "" {
		return schema.Pattern
	}
	return "sample"
}

// exampleForInteger prefers the minimum constraint when provided.
func exampleForInteger(schema *openapi3.Schema) int64 {
	if schema == nil {
		return 0
	}
	if schema.Min != nil {
		return int64(*schema.Min)
	}
	return 0
}

// exampleForNumber mirrors exampleForInteger but for floating point numbers.
func exampleForNumber(schema *openapi3.Schema) float64 {
	if schema == nil {
		return 0
	}
	if schema.Min != nil {
		return *schema.Min
	}
	return 0
}

// defaultForType returns an empty value for the schema type when no better
// information exists.
func defaultForType(ref *openapi3.SchemaRef) any {
	if ref == nil || ref.Value == nil {
		return nil
	}
	schema := ref.Value
	if schema.Default != nil {
		return schema.Default
	}
	if len(schema.Enum) > 0 {
		return schema.Enum[0]
	}
	types := schema.Type.Slice()
	if len(types) == 0 {
		return nil
	}
	switch types[0] {
	case "string":
		return "sample"
	case "integer":
		return 0
	case "number":
		return 0
	case "boolean":
		return false
	case "array":
		return []any{}
	case "object":
		return map[string]any{}
	default:
		return nil
	}
}
