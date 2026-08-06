package mock

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// jsonPredicate reports whether one decoded JSON value satisfies a pattern node.
type jsonPredicate func(got any) bool

// jsonOpCompiler gets the keyword so it can name it when the operand is wrong.
type jsonOpCompiler func(name string, operand any) (jsonPredicate, error)

// jsonOps holds every @match json operator. The $ belongs to each name and is
// not a reserved prefix. JSON gives $ no special meaning and plenty of payloads
// use it, so reserving it would make JSON Schema and MongoDB bodies impossible
// to match.
var jsonOps = map[string]jsonOpCompiler{
	"$gt":    jsonCompareOp(relGT),
	"$gte":   jsonCompareOp(relGTE),
	"$lt":    jsonCompareOp(relLT),
	"$lte":   jsonCompareOp(relLTE),
	"$oneOf": jsonOneOfOp,
}

type jsonField struct {
	name  string
	match jsonPredicate
}

// compileJSONPredicate turns a decoded @match json pattern into a predicate.
// Objects match as a subset, arrays element for element.
func compileJSONPredicate(pattern any) (jsonPredicate, error) {
	switch pattern := pattern.(type) {
	case map[string]any:
		return compileJSONObject(pattern)
	case []any:
		return compileJSONArray(pattern)
	default:
		return func(got any) bool { return equalJSON(pattern, got) }, nil
	}
}

func compileJSONObject(pattern map[string]any) (jsonPredicate, error) {
	// an operator only counts when it is the only member, so a keyword with
	// siblings falls through to the field loop. Sorting also keeps errors stable.
	names := slices.Sorted(maps.Keys(pattern))
	if len(names) == 1 {
		name := names[0]
		if compile, ok := jsonOps[name]; ok {
			return compile(name, pattern[name])
		}
	}

	fields := make([]jsonField, 0, len(names))
	for _, name := range names {
		match, err := compileJSONPredicate(pattern[name])
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}
		fields = append(fields, jsonField{name: name, match: match})
	}
	return func(got any) bool {
		obj, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for _, f := range fields {
			v, ok := obj[f.name]
			if !ok || !f.match(v) {
				return false
			}
		}
		return true
	}, nil
}

func compileJSONArray(pattern []any) (jsonPredicate, error) {
	items := make([]jsonPredicate, len(pattern))
	for i, item := range pattern {
		match, err := compileJSONPredicate(item)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		items[i] = match
	}
	return func(got any) bool {
		list, ok := got.([]any)
		if !ok || len(list) != len(items) {
			return false
		}
		for i, match := range items {
			if !match(list[i]) {
				return false
			}
		}
		return true
	}, nil
}

// jsonCompareOp needs JSON numbers on both sides, so "101" does not satisfy
// {"$gt": 100}.
func jsonCompareOp(rel numberRelation) jsonOpCompiler {
	return func(name string, operand any) (jsonPredicate, error) {
		text, ok := operand.(json.Number)
		if !ok {
			return nil, fmt.Errorf("%s matcher requires a number", name)
		}
		want, ok := parseNumber(string(text))
		if !ok {
			return nil, fmt.Errorf("%s matcher operand %s is out of range", name, text)
		}
		return func(got any) bool {
			n, ok := got.(json.Number)
			if !ok {
				return false
			}
			v, ok := parseNumber(string(n))
			return ok && rel.holds(v.Cmp(want))
		}, nil
	}
}

// jsonOneOfOp compares each alternative whole instead of as a subset. That is
// also how you match a body field that looks like an operator.
func jsonOneOfOp(name string, operand any) (jsonPredicate, error) {
	allowed, ok := operand.([]any)
	if !ok || len(allowed) == 0 {
		return nil, fmt.Errorf("%s matcher requires a non-empty array", name)
	}
	return func(got any) bool {
		return slices.ContainsFunc(allowed, func(want any) bool { return equalJSON(want, got) })
	}, nil
}

func equalJSON(want, got any) bool {
	switch want := want.(type) {
	case map[string]any:
		got, ok := got.(map[string]any)
		if !ok || len(want) != len(got) {
			return false
		}
		for k, v := range want {
			g, ok := got[k]
			if !ok || !equalJSON(v, g) {
				return false
			}
		}
		return true
	case []any:
		got, ok := got.([]any)
		if !ok || len(want) != len(got) {
			return false
		}
		for i := range want {
			if !equalJSON(want[i], got[i]) {
				return false
			}
		}
		return true
	case json.Number:
		got, ok := got.(json.Number)
		return ok && equalJSONNumbers(want, got)
	default:
		return want == got
	}
}
