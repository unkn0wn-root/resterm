package mock

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

type jsonRuleCompiler func(operand any) (jsonPredicate, error)

// Add operators here so the traversal does not need operator-specific branches.
var jsonRuleOps = map[string]jsonRuleCompiler{
	"gt":    numberRule(relGT),
	"gte":   numberRule(relGTE),
	"lt":    numberRule(relLT),
	"lte":   numberRule(relLTE),
	"oneOf": oneOfRule,
}

type jsonRuleField struct {
	name  string
	match jsonPredicate
}

func compileJSONRules(raw []byte) (jsonPredicate, error) {
	node, err := decodeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid json-rules matcher: %w", err)
	}
	obj, ok := node.(map[string]any)
	if !ok {
		return nil, ruleErr("", "must be a JSON object")
	}
	return compileRuleObject(obj, "")
}

func compileRuleNode(node any, path string) (jsonPredicate, error) {
	obj, ok := node.(map[string]any)
	if !ok {
		return nil, ruleErr(path, "expected a rule object or a known operator")
	}
	return compileRuleObject(obj, path)
}

// A rule object uses operator syntax only when every entry is valid. Otherwise
// its keys can identify nested body fields, including fields named gt or oneOf.
func compileRuleObject(obj map[string]any, path string) (jsonPredicate, error) {
	if len(obj) == 0 {
		return nil, ruleErr(path, "rule object cannot be empty")
	}

	// Stable order keeps diagnostics consistent.
	names := slices.Sorted(maps.Keys(obj))
	leaf, isLeaf, err := operatorLeaf(obj, names, path)
	if err != nil {
		return nil, err
	}
	if isLeaf {
		return leaf, nil
	}

	fields := make([]jsonRuleField, 0, len(names))
	for _, name := range names {
		match, err := compileRuleNode(obj[name], childPath(path, name))
		if err != nil {
			return nil, err
		}
		fields = append(fields, jsonRuleField{name: name, match: match})
	}
	return func(got any) bool {
		body, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for _, f := range fields {
			v, ok := body[f.name]
			if !ok || !f.match(v) {
				return false
			}
		}
		return true
	}, nil
}

func operatorLeaf(obj map[string]any, names []string, path string) (jsonPredicate, bool, error) {
	ops := make([]jsonPredicate, 0, len(names))
	for _, name := range names {
		compile, known := jsonRuleOps[name]
		if !known {
			return nil, false, nil
		}
		match, err := compile(obj[name])
		if err != nil {
			// An object operand may instead describe a nested field.
			if _, nested := obj[name].(map[string]any); nested {
				return nil, false, nil
			}
			return nil, false, ruleErr(childPath(path, name), err.Error())
		}
		ops = append(ops, match)
	}
	return allOf(ops), true, nil
}

func allOf(tests []jsonPredicate) jsonPredicate {
	if len(tests) == 1 {
		return tests[0]
	}
	return func(got any) bool {
		for _, test := range tests {
			if !test(got) {
				return false
			}
		}
		return true
	}
}

func numberRule(rel numberRelation) jsonRuleCompiler {
	return func(operand any) (jsonPredicate, error) {
		text, ok := operand.(json.Number)
		if !ok {
			return nil, errors.New("operand must be a JSON number")
		}
		want, _ := parseNumber(string(text))
		return func(got any) bool {
			n, ok := got.(json.Number)
			if !ok {
				return false
			}
			v, ok := parseNumber(string(n))
			return ok && rel.holds(v.cmp(want))
		}, nil
	}
}

// oneOf compares objects and arrays exactly, unlike the JSON subset matcher.
func oneOfRule(operand any) (jsonPredicate, error) {
	allowed, ok := operand.([]any)
	if !ok {
		return nil, errors.New("operand must be an array")
	}
	if len(allowed) == 0 {
		return nil, errors.New("operand cannot be an empty array")
	}
	return func(got any) bool {
		return slices.ContainsFunc(allowed, func(want any) bool { return jsonExact.matches(want, got) })
	}, nil
}

func childPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func ruleErr(path, msg string) error {
	if path == "" {
		return errors.New("invalid json-rules matcher: " + msg)
	}
	return errors.New("invalid json-rules matcher at " + path + ": " + msg)
}
