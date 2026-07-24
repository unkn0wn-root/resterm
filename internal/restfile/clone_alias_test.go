package restfile

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// The type graph is finite today. This is here so a self-referential type added
// later fails a test instead of hanging it.
const maxCloneDepth = 16

// Add a map or slice field and forget to touch clone.go and nothing complains.
// Two runs just share the same backing memory until one of them writes. So fill
// a Document completely, clone it, and check every node came out separate.
func TestCloneSharesNoMemoryWithTheOriginal(t *testing.T) {
	doc := &Document{}
	fillValue(reflect.ValueOf(doc).Elem(), 0)

	var shared []string
	walkShared(
		"Document",
		reflect.ValueOf(doc).Elem(),
		reflect.ValueOf(doc.Clone()).Elem(),
		0,
		&shared,
	)
	if len(shared) > 0 {
		t.Fatalf(
			"Clone shares memory with the original at:\n  %s\nadd the field to clone.go",
			strings.Join(shared, "\n  "),
		)
	}
}

// One element per slice and one entry per map is enough to touch every type in
// the graph, and the walk needs a non-zero value at each node to compare.
func fillValue(v reflect.Value, depth int) {
	if depth > maxCloneDepth {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		fillValue(v.Elem(), depth+1)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		fillValue(s.Index(0), depth+1)
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		key := reflect.New(v.Type().Key()).Elem()
		fillValue(key, depth+1)
		val := reflect.New(v.Type().Elem()).Elem()
		fillValue(val, depth+1)
		m.SetMapIndex(key, val)
		v.Set(m)
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Field(i).CanSet() {
				fillValue(v.Field(i), depth+1)
			}
		}
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	}
}

func walkShared(path string, a, b reflect.Value, depth int, shared *[]string) {
	if depth > maxCloneDepth || !a.IsValid() || !b.IsValid() {
		return
	}
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return
		}
		if a.Pointer() == b.Pointer() {
			*shared = append(*shared, path+" (pointer)")
			return
		}
		walkShared(path, a.Elem(), b.Elem(), depth+1, shared)
	case reflect.Slice:
		if a.IsNil() || b.IsNil() || a.Len() == 0 {
			return
		}
		if a.Pointer() == b.Pointer() {
			*shared = append(*shared, path+" (slice)")
			return
		}
		for i := range a.Len() {
			walkShared(fmt.Sprintf("%s[%d]", path, i), a.Index(i), b.Index(i), depth+1, shared)
		}
	case reflect.Map:
		if a.IsNil() || b.IsNil() {
			return
		}
		if a.Pointer() == b.Pointer() {
			*shared = append(*shared, path+" (map)")
			return
		}
		for _, key := range a.MapKeys() {
			other := b.MapIndex(key)
			if !other.IsValid() {
				continue
			}
			walkShared(
				fmt.Sprintf("%s[%v]", path, key),
				a.MapIndex(key),
				other,
				depth+1,
				shared,
			)
		}
	case reflect.Struct:
		for i := range a.NumField() {
			walkShared(path+"."+a.Type().Field(i).Name, a.Field(i), b.Field(i), depth+1, shared)
		}
	}
}
