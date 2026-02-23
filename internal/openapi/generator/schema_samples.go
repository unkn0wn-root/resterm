package generator

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/openapi/model"
	"github.com/unkn0wn-root/resterm/internal/util"
)

const (
	sampleDateValue     = "2000-01-02"
	sampleDateTimeValue = "2000-01-02T15:04:05Z"
)

type stringFormat string

const (
	fmtUUID          stringFormat = "uuid"
	fmtDate          stringFormat = "date"
	fmtDateTime      stringFormat = "date-time"
	fmtDateTimeAlias stringFormat = "datetime"
	fmtEmail         stringFormat = "email"
	fmtURI           stringFormat = "uri"
	fmtURL           stringFormat = "url"
	fmtHostname      stringFormat = "hostname"
	fmtIPv4          stringFormat = "ipv4"
	fmtIPv6          stringFormat = "ipv6"
)

var strFmtSamples = map[stringFormat]string{
	fmtUUID:          "00000000-0000-4000-8000-000000000000",
	fmtDate:          sampleDateValue,
	fmtDateTime:      sampleDateTimeValue,
	fmtDateTimeAlias: sampleDateTimeValue,
	fmtEmail:         "user@example.com",
	fmtURI:           "https://example.com/resource",
	fmtURL:           "https://example.com/resource",
	fmtHostname:      "example.com",
	fmtIPv4:          "127.0.0.1",
	fmtIPv6:          "2001:db8::1",
}

type schemaSampler struct {
	maxDepth int
}

func newSchemaSampler() *schemaSampler {
	return &schemaSampler{maxDepth: 6}
}

// schemaSampler synthesizes deterministic sample values from schemas when
// explicit OpenAPI examples/defaults/enums are missing.
func (b *schemaSampler) FromSchema(ref *model.SchemaRef) (any, bool) {
	if ref == nil || ref.Node == nil {
		return nil, false
	}
	return b.build(ref, 0)
}

// Depth limit prevents infinite recursion from circular schema references.
// AllOf merges all schemas together since the result must satisfy all of them.
// OneOf/AnyOf just pick the first option since we can't guess which variant to use.
func (b *schemaSampler) build(ref *model.SchemaRef, depth int) (any, bool) {
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

	switch model.InferSchemaType(sch, model.TypeString) {
	case model.TypeString:
		return sampleForString(sch), true
	case model.TypeInteger:
		return sampleForInteger(sch), true
	case model.TypeNumber:
		return sampleForNumber(sch), true
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
		return b.sampleForObject(sch, depth+1)
	default:
		return nil, false
	}
}

func (b *schemaSampler) sampleForObject(sch *model.Schema, depth int) (any, bool) {
	if sch == nil {
		return nil, false
	}
	result := make(map[string]any)

	keys := util.SortedKeys(sch.Properties)
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

func sampleForString(sch *model.Schema) string {
	if sch == nil {
		return ""
	}
	k := stringFormat(strings.ToLower(strings.TrimSpace(sch.Format)))
	if v, ok := strFmtSamples[k]; ok {
		return v
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

type numericSample interface {
	~int64 | ~float64
}

func sampleNumeric[T numericSample](sch *model.Schema, cvt func(float64) T) T {
	var z T
	if sch == nil {
		return z
	}
	if sch.Min != nil {
		return cvt(*sch.Min)
	}
	if sch.Max != nil {
		return cvt(*sch.Max)
	}
	return z
}

func sampleForInteger(sch *model.Schema) int64 {
	return sampleNumeric(sch, func(f float64) int64 { return int64(f) })
}

func sampleForNumber(sch *model.Schema) float64 {
	return sampleNumeric(sch, func(f float64) float64 { return f })
}

func defaultForType(ref *model.SchemaRef) any {
	if ref == nil || ref.Node == nil {
		return nil
	}
	sch := ref.Node
	// Keep fallback precedence aligned with build(): example -> default -> enum.
	if sch.Example != nil {
		return sch.Example
	}
	if sch.Default != nil {
		return sch.Default
	}
	if len(sch.Enum) > 0 {
		return sch.Enum[0]
	}
	t := model.InferSchemaType(sch, "")
	if t == "" {
		return nil
	}
	switch t {
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
