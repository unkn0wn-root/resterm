package generator

import (
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

const (
	sampleDateValue     = "2000-01-02"
	sampleDateTimeValue = "2000-01-02T15:04:05Z"
)

type ExampleBuilder struct {
	maxDepth int
}

func NewExampleBuilder() *ExampleBuilder {
	return &ExampleBuilder{maxDepth: 6}
}

func (b *ExampleBuilder) FromSchema(ref *model.SchemaRef) (any, bool) {
	if ref == nil || ref.Node == nil {
		return nil, false
	}
	return b.build(ref, 0)
}

// Depth limit prevents infinite recursion from circular schema references.
// AllOf merges all schemas together since the result must satisfy all of them.
// OneOf/AnyOf just pick the first option since we can't guess which variant to use.
func (b *ExampleBuilder) build(ref *model.SchemaRef, depth int) (any, bool) {
	if ref == nil || ref.Node == nil {
		return nil, false
	}
	if depth >= b.maxDepth {
		return nil, false
	}
	sch := ref.Node
	if sch.Example != nil {
		return sch.Example, true
	}
	if sch.Default != nil {
		return sch.Default, true
	}
	if len(sch.Enum) > 0 {
		return sch.Enum[0], true
	}

	if len(sch.OneOf) > 0 {
		if value, ok := b.build(sch.OneOf[0], depth+1); ok {
			return value, true
		}
	}
	if len(sch.AnyOf) > 0 {
		if value, ok := b.build(sch.AnyOf[0], depth+1); ok {
			return value, true
		}
	}
	if len(sch.AllOf) > 0 {
		composed := make(map[string]any)
		for _, candidate := range sch.AllOf {
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

	types := sch.Types
	if len(types) == 0 {
		// assume object if properties defined, otherwise fallback to string
		if len(sch.Properties) > 0 || sch.AdditionalProperties != nil {
			types = []string{model.TypeObject}
		} else if sch.Items != nil {
			types = []string{model.TypeArray}
		} else {
			types = []string{model.TypeString}
		}
	}

	switch types[0] {
	case model.TypeString:
		return exampleForString(sch), true
	case model.TypeInteger:
		return exampleForInteger(sch), true
	case model.TypeNumber:
		return exampleForNumber(sch), true
	case model.TypeBoolean:
		return false, true
	case model.TypeArray:
		if sch.Items == nil {
			return []any{}, true
		}
		value, ok := b.build(sch.Items, depth+1)
		if !ok {
			value = defaultForType(sch.Items)
		}
		return []any{value}, true
	case model.TypeObject:
		return b.exampleForObject(sch, depth+1)
	default:
		return nil, false
	}
}

func (b *ExampleBuilder) exampleForObject(sch *model.Schema, depth int) (any, bool) {
	if sch == nil {
		return nil, false
	}
	result := make(map[string]any)

	keys := make([]string, 0, len(sch.Properties))
	for name := range sch.Properties {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		prop := sch.Properties[name]
		if prop == nil {
			continue
		}
		value, ok := b.build(prop, depth)
		if !ok {
			value = defaultForType(prop)
		}
		result[name] = value
	}

	if sch.AdditionalProperties != nil {
		value, ok := b.build(sch.AdditionalProperties, depth)
		if !ok {
			value = defaultForType(sch.AdditionalProperties)
		}
		result["additionalProperty"] = value
	}

	if len(result) == 0 {
		return map[string]any{}, true
	}
	return result, true
}

func exampleForString(sch *model.Schema) string {
	if sch == nil {
		return ""
	}
	switch strings.ToLower(sch.Format) {
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
	if len(sch.Enum) > 0 {
		if value, ok := sch.Enum[0].(string); ok {
			return value
		}
	}
	if sch.Pattern != "" {
		return sch.Pattern
	}
	return defaultSampleValue
}

func exampleForInteger(sch *model.Schema) int64 {
	if sch == nil {
		return 0
	}
	if sch.Min != nil {
		return int64(*sch.Min)
	}
	if sch.Max != nil {
		return int64(*sch.Max)
	}
	return 0
}

func exampleForNumber(sch *model.Schema) float64 {
	if sch == nil {
		return 0
	}
	if sch.Min != nil {
		return *sch.Min
	}
	if sch.Max != nil {
		return *sch.Max
	}
	return 0
}

func defaultForType(ref *model.SchemaRef) any {
	if ref == nil || ref.Node == nil {
		return nil
	}
	sch := ref.Node
	if sch.Default != nil {
		return sch.Default
	}
	if len(sch.Enum) > 0 {
		return sch.Enum[0]
	}
	types := sch.Types
	if len(types) == 0 {
		return nil
	}
	switch types[0] {
	case model.TypeString:
		return defaultSampleValue
	case model.TypeInteger, model.TypeNumber:
		return 0
	case model.TypeBoolean:
		return false
	case model.TypeArray:
		return []any{}
	case model.TypeObject:
		return map[string]any{}
	default:
		return nil
	}
}
