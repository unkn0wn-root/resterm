package vars

import (
	"errors"
	"maps"
	"slices"
	"testing"
)

func TestNewNameMapRejectsBlankAndDuplicateIdentities(t *testing.T) {
	_, err := NewNameMap(map[string]string{" ": "value"})
	var nameErr *NameError
	if !errors.As(err, &nameErr) {
		t.Fatalf("expected NameError, got %v", err)
	}

	_, err = NewNameMap(map[string]string{"Token": "a", " token ": "b"})
	var collision *NameCollisionError
	if !errors.As(err, &collision) {
		t.Fatalf("expected NameCollisionError, got %v", err)
	}
}

func TestNameMapZeroValueTakesWrites(t *testing.T) {
	var m NameMap[string]

	if !m.Set("token", "abc") {
		t.Fatalf("Set() = false, want the value stored")
	}
	if got, ok := m.Get("token"); !ok || got != "abc" {
		t.Fatalf("Get(token) = %q, %v, want %q, true", got, ok, "abc")
	}
}

func TestNameMapKeepsOneFormPerName(t *testing.T) {
	var m NameMap[string]
	m.Set("Token", "first")
	m.Set(" token ", "second")

	if m.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", m.Len())
	}
	if got, _ := m.Get("TOKEN"); got != "second" {
		t.Fatalf("Get(TOKEN) = %q, want %q", got, "second")
	}
	if got := m.Map(); !maps.Equal(got, map[string]string{"token": "second"}) {
		t.Fatalf("Map() = %v, want the last form written", got)
	}
}

func TestNameMapDropsBlankNames(t *testing.T) {
	var m NameMap[string]

	if m.Set("   ", "value") {
		t.Fatalf("Set() = true, want a blank name to be dropped")
	}
	if m.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", m.Len())
	}
	if _, ok := m.Get(""); ok {
		t.Fatalf("Get() found a value under the empty name")
	}
}

func TestNameMapDeleteMatchesAnyForm(t *testing.T) {
	var m NameMap[string]
	m.Set("Token", "abc")
	m.Delete(" TOKEN ")

	if m.Has("token") {
		t.Fatalf("Has(token) = true, want the value deleted")
	}
}

func TestNameMapSetMapFoldsFormsInOneOrder(t *testing.T) {
	src := map[string]string{"TOKEN": "upper", "token": "lower"}
	for range 8 {
		if got, _ := CollectNames(src).Get("token"); got != "lower" {
			t.Fatalf("Get(token) = %q, want %q", got, "lower")
		}
	}
}

func TestNameMapCloneIsolatesLaterWrites(t *testing.T) {
	var m NameMap[string]
	m.Set("token", "first")

	cp := m.Clone()
	m.Set("token", "second")
	m.Set("extra", "new")

	if got, _ := cp.Get("token"); got != "first" {
		t.Fatalf("Get(token) = %q, want the value held when cloned", got)
	}
	if cp.Has("extra") {
		t.Fatalf("Has(extra) = true, want a write after the clone to stay out")
	}
}

func TestNameMapMergePrefersSource(t *testing.T) {
	var dst NameMap[string]
	dst.Set("token", "old")
	dst.Set("keep", "kept")

	var src NameMap[string]
	src.Set("TOKEN", "new")
	dst.Merge(src)

	if got, _ := dst.Get("token"); got != "new" {
		t.Fatalf("Get(token) = %q, want %q", got, "new")
	}
	if got, _ := dst.Get("keep"); got != "kept" {
		t.Fatalf("Get(keep) = %q, want %q", got, "kept")
	}
	if dst.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", dst.Len())
	}
}

func TestNameMapSortedReadsByNameIdentity(t *testing.T) {
	var m NameMap[string]
	m.Set("Beta", "2")
	m.Set("alpha", "1")
	m.Set("Gamma", "3")

	var names []string
	for name := range m.Sorted() {
		names = append(names, name)
	}
	if want := []string{"alpha", "Beta", "Gamma"}; !slices.Equal(names, want) {
		t.Fatalf("Sorted() names = %v, want %v", names, want)
	}
}

func TestNameMapHoldsGlobalMutations(t *testing.T) {
	var m NameMap[GlobalMutation]
	m.Set("Token", GlobalMutation{Name: "Token", Value: "first"})
	m.Set("token", GlobalMutation{Name: "token", Value: "second", Secret: true})

	got, ok := m.Get("TOKEN")
	if !ok || got.Value != "second" || !got.Secret {
		t.Fatalf("Get(TOKEN) = %#v, %v, want the second mutation", got, ok)
	}
	if m.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", m.Len())
	}
}
