package restfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"golang.org/x/net/http/httpguts"
)

// headerOperand is the JSON shape an operator's operand takes. Decoding,
// encoding, and the cardinality check all switch on it.
type headerOperand uint8

const (
	operandFlag      headerOperand = iota // {"present": true}
	operandString                         // {"prefix": "Bearer "}
	operandList                           // {"oneOf": ["dev", "prod"]}
	operandShorthand                      // the bare form, "demo" or ["a", "b"]
)

type mockHeaderOpSpec struct {
	name    string
	operand headerOperand
	// verify checks the operand value once its shape is known good. Most
	// operators do not need one.
	verify func(name string, values []string) error
}

// mockHeaderOps is indexed by MockHeaderOp. Index 0 is MockHeaderOpUnknown and
// stays empty, which is how spec detects an unknown operator. Only one operator
// can use operandShorthand, because the bare JSON form has no keyword to tell
// them apart.
var mockHeaderOps = [...]mockHeaderOpSpec{
	MockHeaderOpExact:    {name: "exact", operand: operandShorthand},
	MockHeaderOpPrefix:   {name: "prefix", operand: operandString},
	MockHeaderOpPresent:  {name: "present", operand: operandFlag},
	MockHeaderOpAbsent:   {name: "absent", operand: operandFlag},
	MockHeaderOpContains: {name: "contains", operand: operandString},
	MockHeaderOpRegex:    {name: "regex", operand: operandString, verify: verifyRegex},
	MockHeaderOpOneOf:    {name: "oneOf", operand: operandList},
}

func verifyRegex(name string, values []string) error {
	if _, err := regexp.Compile(values[0]); err != nil {
		return fmt.Errorf("%s matcher is not a valid regular expression: %w", name, err)
	}
	return nil
}

func (op MockHeaderOp) spec() (mockHeaderOpSpec, bool) {
	if int(op) >= len(mockHeaderOps) || mockHeaderOps[op].name == "" {
		return mockHeaderOpSpec{}, false
	}
	return mockHeaderOps[op], true
}

// String returns the operator keyword as it is written in @match headers.
func (op MockHeaderOp) String() string {
	spec, ok := op.spec()
	if !ok {
		return "unknown"
	}
	return spec.name
}

func mockHeaderOpNamed(name string) (MockHeaderOp, bool) {
	for op, spec := range mockHeaderOps {
		if spec.name != "" && spec.name == name {
			return MockHeaderOp(op), true
		}
	}
	return MockHeaderOpUnknown, false
}

// Check reports whether the rule is usable. Errors leave out the header name so
// each caller can add the context it has.
func (r MockHeaderRule) Check() error {
	spec, ok := r.Op.spec()
	if !ok {
		return errors.New("matcher operation is invalid")
	}
	for _, value := range r.Values {
		if !httpguts.ValidHeaderFieldValue(value) {
			return fmt.Errorf("%s matcher value %q is not a valid header value", spec.name, value)
		}
	}
	switch spec.operand {
	case operandFlag:
		if len(r.Values) != 0 {
			return fmt.Errorf("%s matcher cannot have values", spec.name)
		}
	case operandString:
		if len(r.Values) != 1 || r.Values[0] == "" {
			return fmt.Errorf("%s matcher requires one non-empty value", spec.name)
		}
	case operandList, operandShorthand:
		if len(r.Values) == 0 {
			return fmt.Errorf("%s matcher requires at least one value", spec.name)
		}
	}
	if spec.verify != nil {
		return spec.verify(spec.name, r.Values)
	}
	return nil
}

func (r MockHeaderRule) MarshalJSON() ([]byte, error) {
	if err := r.Check(); err != nil {
		return nil, err
	}
	spec, _ := r.Op.spec()
	switch spec.operand {
	case operandFlag:
		return json.Marshal(map[string]bool{spec.name: true})
	case operandString:
		return json.Marshal(map[string]string{spec.name: r.Values[0]})
	case operandList:
		return json.Marshal(map[string][]string{spec.name: r.Values})
	case operandShorthand:
		// the bare form has no keyword to emit, so a single value is just a string
		if len(r.Values) == 1 {
			return json.Marshal(r.Values[0])
		}
		return json.Marshal(r.Values)
	default:
		return nil, fmt.Errorf("%s matcher has an unsupported operand", spec.name)
	}
}

func (r *MockHeaderRule) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return errors.New("matcher cannot be null")
	}
	// a bare string or array is the exact shorthand and carries no operator name
	var exact StringList
	if exact.UnmarshalJSON(data) == nil {
		*r = MockHeaderRule{Op: MockHeaderOpExact, Values: exact}
		return r.Check()
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("matcher must be a string, string array, or object")
	}
	if len(fields) != 1 {
		return errors.New("matcher must contain exactly one operator")
	}
	var name string
	var raw json.RawMessage
	for key, value := range fields {
		name, raw = key, value
	}
	op, ok := mockHeaderOpNamed(name)
	if !ok {
		return fmt.Errorf("unknown matcher operator %q", name)
	}
	spec, _ := op.spec()
	values, err := spec.decode(raw)
	if err != nil {
		return err
	}
	*r = MockHeaderRule{Op: op, Values: values}
	return r.Check()
}

func (s mockHeaderOpSpec) decode(raw json.RawMessage) ([]string, error) {
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
