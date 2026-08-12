// Package httpheader defines HTTP header-name identity independently of RTS.
package httpheader

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Values is a header block. Every entry is represented as a list, including
// entries with zero or one value.
type Values map[string][]string

// Name is a validated, case-insensitive HTTP field name.
// Its zero value is not a valid name.
type Name struct {
	key string
}

// NameError reports a string that is not an HTTP field name.
type NameError struct {
	Name string
}

func (e *NameError) Error() string {
	return fmt.Sprintf("%q is not an HTTP header name", e.Name)
}

// CollisionError reports header names with the same case-insensitive identity.
type CollisionError struct {
	First  string
	Second string
}

func (e *CollisionError) Error() string {
	return fmt.Sprintf("%q and %q are the same HTTP header", e.First, e.Second)
}

// Valid reports whether name is an HTTP field-name token. Whitespace is never
// trimmed: callers must decide whether trimming belongs at their own boundary.
func Valid(name string) bool {
	if name == "" {
		return false
	}
	for i := range len(name) {
		if !token[name[i]] {
			return false
		}
	}
	return true
}

// Parse validates name and returns its case-insensitive identity.
func Parse(name string) (Name, error) {
	if !Valid(name) {
		return Name{}, &NameError{Name: name}
	}
	return Name{key: strings.ToLower(name)}, nil
}

// Key returns the normalized identity of n.
func (n Name) Key() string { return n.key }

// Key returns the case-insensitive identity of a valid header name.
func Key(name string) (string, error) {
	n, err := Parse(name)
	if err != nil {
		return "", err
	}
	return n.Key(), nil
}

// Named contains a header name and its case-insensitive identity.
type Named struct {
	Source string
	Name   Name
}

// Keys validates and sorts the names in src. It rejects names with the same
// case-insensitive identity so map iteration cannot choose which value wins.
// Sorting also makes validation errors deterministic.
func Keys[V any](src map[string]V) ([]Named, error) {
	out := make([]Named, 0, len(src))
	seen := make(map[string]string, len(src))
	for _, name := range slices.Sorted(maps.Keys(src)) {
		n, err := Parse(name)
		if err != nil {
			return nil, err
		}
		key := n.Key()
		if first, ok := seen[key]; ok {
			return nil, &CollisionError{First: first, Second: name}
		}
		seen[key] = name
		out = append(out, Named{Source: name, Name: n})
	}
	return out, nil
}

// Normalize validates names, rejects equivalent forms, copies every value
// list, and keys the result by header identity.
func Normalize(src map[string][]string) (Values, error) {
	keys, err := Keys(src)
	if err != nil {
		return nil, err
	}
	out := make(Values, len(src))
	for _, k := range keys {
		out[k.Name.Key()] = append([]string(nil), src[k.Source]...)
	}
	return out, nil
}

// Clone returns a deep-enough copy for independently mutating the map and its
// value lists.
func Clone(src map[string][]string) Values {
	out := make(Values, len(src))
	for name, vals := range src {
		out[name] = append([]string(nil), vals...)
	}
	return out
}

var token = tokenTable()

func tokenTable() [256]bool {
	var out [256]bool
	for _, c := range "!#$%&'*+-.^_`|~" {
		out[c] = true
	}
	for c := byte('0'); c <= '9'; c++ {
		out[c] = true
	}
	for c := byte('A'); c <= 'Z'; c++ {
		out[c] = true
	}
	for c := byte('a'); c <= 'z'; c++ {
		out[c] = true
	}
	return out
}
