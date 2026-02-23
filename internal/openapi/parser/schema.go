package parser

import (
	"fmt"
	"strings"

	hbase "github.com/pb33f/libopenapi/datamodel/high/base"
	"go.yaml.in/yaml/v4"

	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

// schMap caches converted schema refs during a single parse pass.
type schMap struct {
	refs map[*hbase.SchemaProxy]*model.SchemaRef
	warn func(string)
}

func newSchMap() *schMap {
	return &schMap{refs: make(map[*hbase.SchemaProxy]*model.SchemaRef)}
}

func (m *schMap) setWarn(w func(string)) {
	m.warn = w
}

func (m *schMap) toRef(src *hbase.SchemaProxy) *model.SchemaRef {
	if src == nil {
		return nil
	}

	if ref, ok := m.refs[src]; ok {
		return ref
	}

	ref := &model.SchemaRef{}
	m.refs[src] = ref
	if src.IsReference() {
		ref.Identifier = src.GetReference()
	}

	sch := src.Schema()
	if sch == nil {
		return ref
	}

	ref.Node = m.toSch(sch)
	return ref
}

func (m *schMap) toSch(src *hbase.Schema) *model.Schema {
	if src == nil {
		return nil
	}

	out := &model.Schema{
		Title:       src.Title,
		Description: src.Description,
		Types:       model.CloneStrs(src.Type),
		Format:      src.Format,
		Pattern:     src.Pattern,
		Example:     nodeAny(src.Example, m.warn, "schema example"),
		Default:     nodeAny(src.Default, m.warn, "schema default"),
		Enum:        nodesAny(src.Enum, m.warn, "schema enum"),
		Min:         cpNum(src.Minimum),
		Max:         cpNum(src.Maximum),
		MinLen:      cpI64(src.MinLength),
		MaxLen:      cpI64(src.MaxLength),
		Required:    model.CloneStrs(src.Required),
		Nullable:    cpBool(src.Nullable),
		ReadOnly:    cpBool(src.ReadOnly),
		WriteOnly:   cpBool(src.WriteOnly),
	}

	if item := dynRef(src.Items); item != nil {
		out.Items = m.toRef(item)
	}

	if src.Properties != nil && src.Properties.Len() > 0 {
		props := make(map[string]*model.SchemaRef, src.Properties.Len())
		for key, val := range src.Properties.FromOldest() {
			props[key] = m.toRef(val)
		}
		out.Properties = props
	}

	if ap := dynRef(src.AdditionalProperties); ap != nil {
		out.AdditionalProperties = m.toRef(ap)
	}

	if len(src.OneOf) > 0 {
		out.OneOf = m.toRefs(src.OneOf)
	}
	if len(src.AnyOf) > 0 {
		out.AnyOf = m.toRefs(src.AnyOf)
	}
	if len(src.AllOf) > 0 {
		out.AllOf = m.toRefs(src.AllOf)
	}

	return out
}

func (m *schMap) toRefs(src []*hbase.SchemaProxy) []*model.SchemaRef {
	if len(src) == 0 {
		return nil
	}

	out := make([]*model.SchemaRef, 0, len(src))
	for _, ref := range src {
		out = append(out, m.toRef(ref))
	}
	return out
}

func nodesAny(src []*yaml.Node, warn func(string), ctx string) []any {
	if len(src) == 0 {
		return nil
	}

	out := make([]any, 0, len(src))
	for i, node := range src {
		loc := fmt.Sprintf("%s[%d]", ctx, i)
		out = append(out, nodeAny(node, warn, loc))
	}
	return out
}

func cpNum(src *float64) *float64 {
	if src == nil {
		return nil
	}
	out := *src
	return &out
}

func cpI64(src *int64) *int64 {
	if src == nil {
		return nil
	}
	out := *src
	return &out
}

func cpBool(src *bool) *bool {
	if src == nil {
		return nil
	}
	out := *src
	return &out
}

func dynRef(src *hbase.DynamicValue[*hbase.SchemaProxy, bool]) *hbase.SchemaProxy {
	if src == nil || !src.IsA() || src.A == nil {
		return nil
	}
	return src.A
}

func nodeAny(src *yaml.Node, warn func(string), ctx string) any {
	if src == nil {
		return nil
	}
	var out any
	if err := src.Decode(&out); err != nil {
		noteWarn(
			warn,
			fmt.Sprintf("OpenAPI compatibility: unable to decode %s; value ignored: %v", ctx, err),
		)
		return nil
	}
	return out
}

func noteWarn(warn func(string), msg string) {
	if warn == nil {
		return
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	warn(msg)
}
