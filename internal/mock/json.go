package mock

import (
	"encoding/json"
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type jsonPredicate func(got any) bool

type jsonCompare uint8

const (
	jsonSubset jsonCompare = iota
	jsonExact
)

func compileJSONBody(subset, rules []byte) (jsonPredicate, error) {
	var literal jsonPredicate
	if len(subset) > 0 {
		if err := restfile.CheckMockJSONKeys(subset); err != nil {
			return nil, fmt.Errorf("invalid json matcher: %w", err)
		}
		pattern, err := decodeJSON(subset)
		if err != nil {
			return nil, fmt.Errorf("invalid json matcher: %w", err)
		}
		literal = func(got any) bool { return jsonSubset.matches(pattern, got) }
	}
	if len(rules) == 0 {
		return literal, nil
	}

	if err := restfile.CheckMockJSONKeys(rules); err != nil {
		return nil, fmt.Errorf("invalid json-rules matcher: %w", err)
	}
	match, err := compileJSONRules(rules)
	if err != nil {
		return nil, err
	}
	if literal == nil {
		return match, nil
	}
	return func(got any) bool { return literal(got) && match(got) }, nil
}

func (c jsonCompare) matches(want, got any) bool {
	switch want := want.(type) {
	case map[string]any:
		obj, ok := got.(map[string]any)
		if !ok || c == jsonExact && len(want) != len(obj) {
			return false
		}
		for name, v := range want {
			g, ok := obj[name]
			if !ok || !c.matches(v, g) {
				return false
			}
		}
		return true
	case []any:
		list, ok := got.([]any)
		if !ok || len(want) != len(list) {
			return false
		}
		for i := range want {
			if !c.matches(want[i], list[i]) {
				return false
			}
		}
		return true
	case json.Number:
		n, ok := got.(json.Number)
		return ok && equalJSONNumbers(want, n)
	default:
		return want == got
	}
}
