package restfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// operand is the JSON shape a matcher operand takes. Decoding, encoding, and the
// cardinality check all switch on it.
type operand uint8

const (
	operandFlag      operand = iota // {"present": true}
	operandString                   // {"prefix": "Bearer "}
	operandList                     // {"oneOf": ["dev", "prod"]}
	operandNumber                   // {"gt": 10}
	operandShorthand                // the bare form, "demo" or ["a", "b"]
)

// matcherSpec describes one operator. verify runs last, once the shape and count
// are known good.
type matcherSpec struct {
	name    string
	operand operand
	verify  func(name string, values []string) error
}

// specAt indexes a table by the operator enum. Index 0 is the unknown operator
// and stays empty, which is how a zero or out of range value is caught.
func specAt(table []matcherSpec, op int) (matcherSpec, bool) {
	if op < 0 || op >= len(table) || table[op].name == "" {
		return matcherSpec{}, false
	}
	return table[op], true
}

func specNamed(table []matcherSpec, name string) (int, bool) {
	for op, spec := range table {
		if spec.name != "" && spec.name == name {
			return op, true
		}
	}
	return 0, false
}

func (s matcherSpec) check(values []string) error {
	switch s.operand {
	case operandFlag:
		if len(values) != 0 {
			return fmt.Errorf("%s matcher cannot have values", s.name)
		}
	case operandString:
		if len(values) != 1 || values[0] == "" {
			return fmt.Errorf("%s matcher requires one non-empty value", s.name)
		}
	case operandNumber:
		if len(values) != 1 || !isJSONNumber(values[0]) {
			return fmt.Errorf("%s matcher requires one JSON number", s.name)
		}
	case operandList, operandShorthand:
		if len(values) == 0 {
			return fmt.Errorf("%s matcher requires at least one value", s.name)
		}
	}
	if s.verify != nil {
		return s.verify(s.name, values)
	}
	return nil
}

func (s matcherSpec) marshal(values []string) ([]byte, error) {
	switch s.operand {
	case operandFlag:
		return json.Marshal(map[string]bool{s.name: true})
	case operandString:
		return json.Marshal(map[string]string{s.name: values[0]})
	case operandNumber:
		return json.Marshal(map[string]json.Number{s.name: json.Number(values[0])})
	case operandList:
		return json.Marshal(map[string][]string{s.name: values})
	case operandShorthand:
		// the bare form has no keyword to emit, so a single value is just a string
		if len(values) == 1 {
			return json.Marshal(values[0])
		}
		return json.Marshal(values)
	default:
		return nil, fmt.Errorf("%s matcher has an unsupported operand", s.name)
	}
}

func (s matcherSpec) decode(raw json.RawMessage) ([]string, error) {
	switch s.operand {
	case operandFlag:
		var enabled bool
		if err := json.Unmarshal(raw, &enabled); err != nil || !enabled {
			return nil, fmt.Errorf("%s matcher must be true", s.name)
		}
		return nil, nil
	case operandString:
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("%s matcher must be a non-empty string", s.name)
		}
		return []string{value}, nil
	case operandNumber:
		// encoding/json happily reads "10" into a json.Number, so the operand is
		// checked as raw text to keep the quoted form out
		if !isJSONNumber(string(raw)) {
			return nil, fmt.Errorf("%s matcher must be a number, not a quoted string", s.name)
		}
		return []string{string(raw)}, nil
	case operandList:
		var values []string
		if err := json.Unmarshal(raw, &values); err != nil || values == nil {
			return nil, fmt.Errorf("%s matcher must be a non-empty string array", s.name)
		}
		return values, nil
	case operandShorthand:
		var values StringList
		if err := values.UnmarshalJSON(raw); err != nil {
			return nil, fmt.Errorf("%s matcher must be a string or non-empty string array", s.name)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("%s matcher has an unsupported operand", s.name)
	}
}

// decodeMatcher resolves the single operator key of a matcher object. The bare
// shorthand has no operator name, so callers try that form first.
func decodeMatcher(table []matcherSpec, data []byte) (int, []string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return 0, nil, errors.New("matcher must be a string, string array, or object")
	}
	if len(fields) != 1 {
		return 0, nil, errors.New("matcher must contain exactly one operator")
	}
	var name string
	var raw json.RawMessage
	for key, value := range fields {
		name, raw = key, value
	}
	op, ok := specNamed(table, name)
	if !ok {
		return 0, nil, fmt.Errorf("unknown matcher operator %q", name)
	}
	values, err := table[op].decode(raw)
	if err != nil {
		return 0, nil, err
	}
	return op, values, nil
}

func verifyRegex(name string, values []string) error {
	if _, err := regexp.Compile(values[0]); err != nil {
		return fmt.Errorf("%s matcher is not a valid regular expression: %w", name, err)
	}
	return nil
}

// isJSONNumber guards marshal, which writes the operand back out as a bare
// literal: an empty operand would otherwise become 0.
func isJSONNumber(s string) bool {
	var n json.Number
	return json.Unmarshal([]byte(s), &n) == nil && string(n) == s
}
