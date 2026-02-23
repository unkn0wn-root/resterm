package parser

import (
	"testing"

	hbase "github.com/pb33f/libopenapi/datamodel/high/base"
	"github.com/pb33f/libopenapi/orderedmap"
	yaml "go.yaml.in/yaml/v4"
)

func TestSchMapToRefCyclic(t *testing.T) {
	t.Parallel()

	root := &hbase.Schema{
		Type:       []string{"object"},
		Properties: orderedmap.New[string, *hbase.SchemaProxy](),
	}
	ref := hbase.CreateSchemaProxy(root)
	root.Properties.Set("next", ref)

	sm := newSchMap()
	got := sm.toRef(ref)
	if got == nil || got.Node == nil {
		t.Fatalf("expected converted schema ref")
	}

	next := got.Node.Properties["next"]
	if next == nil {
		t.Fatalf("expected next property")
	}
	if next != got {
		t.Fatalf("expected recursive property to point to same ref")
	}

	got2 := sm.toRef(ref)
	if got2 != got {
		t.Fatalf("expected memoized schema ref")
	}
}

func TestSchMapToRefCopiesSchemaFields(t *testing.T) {
	t.Parallel()

	min := 3.25
	max := 9.5
	minLen := int64(2)
	maxLen := int64(8)
	nullable := true
	readOnly := false
	writeOnly := true
	src := hbase.CreateSchemaProxy(&hbase.Schema{
		Title:       "EmailAddress",
		Description: "Primary login email",
		Type:        []string{"string"},
		Format:      "email",
		Pattern:     ".+@.+",
		Example:     node("dev@example.com"),
		Default:     node("default@example.com"),
		Enum:        []*yaml.Node{node("dev@example.com"), node("ops@example.com")},
		Minimum:     &min,
		Maximum:     &max,
		MinLength:   &minLen,
		MaxLength:   &maxLen,
		Required:    []string{"address"},
		Nullable:    &nullable,
		ReadOnly:    &readOnly,
		WriteOnly:   &writeOnly,
	})

	sm := newSchMap()
	got := sm.toRef(src)
	if got == nil || got.Node == nil {
		t.Fatalf("expected converted schema ref")
	}

	sch := got.Node
	if sch.Title != "EmailAddress" {
		t.Fatalf("unexpected title: %q", sch.Title)
	}
	if sch.Description != "Primary login email" {
		t.Fatalf("unexpected description: %q", sch.Description)
	}
	if len(sch.Types) != 1 || sch.Types[0] != "string" {
		t.Fatalf("unexpected types: %#v", sch.Types)
	}
	if sch.Format != "email" {
		t.Fatalf("unexpected format: %s", sch.Format)
	}
	if sch.Pattern != ".+@.+" {
		t.Fatalf("unexpected pattern: %s", sch.Pattern)
	}
	if sch.Example != "dev@example.com" {
		t.Fatalf("unexpected example: %#v", sch.Example)
	}
	if sch.Default != "default@example.com" {
		t.Fatalf("unexpected default: %#v", sch.Default)
	}
	if len(sch.Enum) != 2 {
		t.Fatalf("unexpected enum: %#v", sch.Enum)
	}
	if sch.Min == nil || *sch.Min != min {
		t.Fatalf("unexpected minimum: %#v", sch.Min)
	}
	if sch.Max == nil || *sch.Max != max {
		t.Fatalf("unexpected maximum: %#v", sch.Max)
	}
	if sch.MinLen == nil || *sch.MinLen != minLen {
		t.Fatalf("unexpected minLength: %#v", sch.MinLen)
	}
	if sch.MaxLen == nil || *sch.MaxLen != maxLen {
		t.Fatalf("unexpected maxLength: %#v", sch.MaxLen)
	}
	if len(sch.Required) != 1 || sch.Required[0] != "address" {
		t.Fatalf("unexpected required list: %#v", sch.Required)
	}
	if sch.Nullable == nil || *sch.Nullable != nullable {
		t.Fatalf("unexpected nullable: %#v", sch.Nullable)
	}
	if sch.ReadOnly == nil || *sch.ReadOnly != readOnly {
		t.Fatalf("unexpected readOnly: %#v", sch.ReadOnly)
	}
	if sch.WriteOnly == nil || *sch.WriteOnly != writeOnly {
		t.Fatalf("unexpected writeOnly: %#v", sch.WriteOnly)
	}

	*sch.Min = 10
	if min != 3.25 {
		t.Fatalf("expected minimum to be copied by value, got %f", min)
	}
}
