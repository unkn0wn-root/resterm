package vars

import (
	"iter"
	"maps"
	"slices"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

// NameKey returns the case-insensitive, whitespace-trimmed identity of name.
func NameKey(name string) string { return norm(name) }

// NameMap stores one value per case-insensitive, whitespace-trimmed name. It
// preserves the most recently written form for display, and its zero value is
// ready to use.
type NameMap[V any] struct {
	entries map[string]namedValue[V]
}

type namedValue[V any] struct {
	name  string
	value V
}

// Set stores value and reports whether name was non-blank.
func (m *NameMap[V]) Set(name string, value V) bool {
	key := NameKey(name)
	if key == "" {
		return false
	}
	if m.entries == nil {
		m.entries = make(map[string]namedValue[V])
	}
	m.entries[key] = namedValue[V]{name: str.Trim(name), value: value}
	return true
}

func (m NameMap[V]) Get(name string) (V, bool) {
	e, ok := m.entries[NameKey(name)]
	return e.value, ok
}

func (m NameMap[V]) Has(name string) bool {
	_, ok := m.entries[NameKey(name)]
	return ok
}

func (m *NameMap[V]) Delete(name string) {
	delete(m.entries, NameKey(name))
}

func (m NameMap[V]) Len() int { return len(m.entries) }

// All iterates over entries in unspecified order.
func (m NameMap[V]) All() iter.Seq2[string, V] {
	return func(yield func(string, V) bool) {
		for _, e := range m.entries {
			if !yield(e.name, e.value) {
				return
			}
		}
	}
}

// Sorted iterates over entries by normalized name.
func (m NameMap[V]) Sorted() iter.Seq2[string, V] {
	return func(yield func(string, V) bool) {
		for _, key := range slices.Sorted(maps.Keys(m.entries)) {
			e := m.entries[key]
			if !yield(e.name, e.value) {
				return
			}
		}
	}
}

// Merge overwrites entries in m with entries from src.
func (m *NameMap[V]) Merge(src NameMap[V]) {
	for name, value := range src.All() {
		m.Set(name, value)
	}
}

// SetMap adds src in name order so equivalent keys resolve deterministically.
func (m *NameMap[V]) SetMap(src map[string]V) {
	for _, name := range slices.Sorted(maps.Keys(src)) {
		m.Set(name, src[name])
	}
}

func (m *NameMap[V]) DeleteFunc(del func(name string, value V) bool) {
	maps.DeleteFunc(m.entries, func(_ string, e namedValue[V]) bool {
		return del(e.name, e.value)
	})
}

// Clone returns an independent copy of m.
func (m NameMap[V]) Clone() NameMap[V] {
	return NameMap[V]{entries: maps.Clone(m.entries)}
}

// Map returns a writable plain map using each entry's preserved name.
func (m NameMap[V]) Map() map[string]V {
	out := make(map[string]V, len(m.entries))
	for _, e := range m.entries {
		out[e.name] = e.value
	}
	return out
}

// CollectNames builds a NameMap using SetMap's deterministic ordering.
func CollectNames[V any](src map[string]V) NameMap[V] {
	var out NameMap[V]
	out.SetMap(src)
	return out
}

// Upsert writes value under name and drops any other form of that name, so a
// plain map contains at most one case or whitespace variant.
func Upsert(m map[string]string, name, value string) {
	key := NameKey(name)
	for cur := range m {
		if cur != name && NameKey(cur) == key {
			delete(m, cur)
		}
	}
	m[name] = value
}

// MergeInto copies src into dst, replacing any equivalent names already present.
func MergeInto(dst map[string]string, src NameMap[string]) {
	for name, value := range src.All() {
		Upsert(dst, name, value)
	}
}
