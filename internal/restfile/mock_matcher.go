package restfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
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

// matcherSpecs is indexed by MockMatchOp. Index 0 is the unknown operator and
// stays empty, which is how a zero or out of range value is caught.
var matcherSpecs = []matcherSpec{
	MockOpExact:    {name: "exact", operand: operandShorthand},
	MockOpPrefix:   {name: "prefix", operand: operandString},
	MockOpPresent:  {name: "present", operand: operandFlag},
	MockOpAbsent:   {name: "absent", operand: operandFlag},
	MockOpContains: {name: "contains", operand: operandString},
	MockOpRegex:    {name: "regex", operand: operandString, verify: verifyRegex},
	MockOpOneOf:    {name: "oneOf", operand: operandList},
	MockOpGT:       {name: "gt", operand: operandNumber},
	MockOpGTE:      {name: "gte", operand: operandNumber},
	MockOpLT:       {name: "lt", operand: operandNumber},
	MockOpLTE:      {name: "lte", operand: operandNumber},
}

func specOf(op MockMatchOp) (matcherSpec, bool) {
	if int(op) >= len(matcherSpecs) || matcherSpecs[op].name == "" {
		return matcherSpec{}, false
	}
	return matcherSpecs[op], true
}

func (op MockMatchOp) String() string {
	if spec, ok := specOf(op); ok {
		return spec.name
	}
	return "unknown"
}

// matcherOps is the operator set one matcher field accepts. Only one operator in
// a set can use operandShorthand, because the bare JSON form has no keyword to
// tell them apart.
type matcherOps []MockMatchOp

func (ops matcherOps) spec(op MockMatchOp) (matcherSpec, bool) {
	if !slices.Contains(ops, op) {
		return matcherSpec{}, false
	}
	return specOf(op)
}

func (ops matcherOps) named(name string) (MockMatchOp, bool) {
	for _, op := range ops {
		if spec, ok := specOf(op); ok && spec.name == name {
			return op, true
		}
	}
	return MockOpUnknown, false
}

func (r MockRule) check(ops matcherOps) error {
	spec, ok := ops.spec(r.Op)
	if !ok {
		return errors.New("matcher operation is invalid")
	}
	return spec.check(r.Values)
}

func (r MockRule) marshal(ops matcherOps) ([]byte, error) {
	spec, ok := ops.spec(r.Op)
	if !ok {
		return nil, errors.New("matcher operation is invalid")
	}
	return spec.marshal(r.Values)
}

// Validation is left to the caller, whose field may hold the operand to a
// stricter standard than the operator does.
func (r *MockRule) unmarshal(ops matcherOps, data []byte) error {
	if string(data) == "null" {
		return errors.New("matcher cannot be null")
	}
	// a bare string or array is the exact shorthand and carries no operator name
	var exact StringList
	if exact.UnmarshalJSON(data) == nil {
		*r = MockRule{Op: MockOpExact, Values: exact}
		return nil
	}

	op, values, err := ops.decode(data)
	if err != nil {
		return err
	}
	*r = MockRule{Op: op, Values: values}
	return nil
}

func (ops matcherOps) decode(data []byte) (MockMatchOp, []string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return MockOpUnknown, nil, errors.New("matcher must be a string, string array, or object")
	}
	if len(fields) != 1 {
		return MockOpUnknown, nil, errors.New("matcher must contain exactly one operator")
	}
	var name string
	var raw json.RawMessage
	for key, value := range fields {
		name, raw = key, value
	}
	op, ok := ops.named(name)
	if !ok {
		return MockOpUnknown, nil, fmt.Errorf("unknown matcher operator %q", name)
	}
	values, err := matcherSpecs[op].decode(raw)
	if err != nil {
		return MockOpUnknown, nil, err
	}
	return op, values, nil
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
