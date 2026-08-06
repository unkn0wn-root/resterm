package restfile

import "errors"

// mockQueryOps is indexed by MockQueryOp. Query values carry no HTTP validity
// constraint, so unlike headers they can hold any text the URL decodes to.
var mockQueryOps = []matcherSpec{
	MockQueryOpExact:    {name: "exact", operand: operandShorthand},
	MockQueryOpPrefix:   {name: "prefix", operand: operandString},
	MockQueryOpPresent:  {name: "present", operand: operandFlag},
	MockQueryOpAbsent:   {name: "absent", operand: operandFlag},
	MockQueryOpContains: {name: "contains", operand: operandString},
	MockQueryOpRegex:    {name: "regex", operand: operandString, verify: verifyRegex},
	MockQueryOpOneOf:    {name: "oneOf", operand: operandList},
	MockQueryOpGT:       {name: "gt", operand: operandNumber},
	MockQueryOpGTE:      {name: "gte", operand: operandNumber},
	MockQueryOpLT:       {name: "lt", operand: operandNumber},
	MockQueryOpLTE:      {name: "lte", operand: operandNumber},
}

func (op MockQueryOp) String() string {
	spec, ok := specAt(mockQueryOps, int(op))
	if !ok {
		return "unknown"
	}
	return spec.name
}

func (r MockQueryRule) Check() error {
	spec, ok := specAt(mockQueryOps, int(r.Op))
	if !ok {
		return errors.New("matcher operation is invalid")
	}
	return spec.check(r.Values)
}

func (r MockQueryRule) MarshalJSON() ([]byte, error) {
	if err := r.Check(); err != nil {
		return nil, err
	}
	spec, _ := specAt(mockQueryOps, int(r.Op))
	return spec.marshal(r.Values)
}

func (r *MockQueryRule) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return errors.New("matcher cannot be null")
	}
	// a bare string or array is the exact shorthand and carries no operator name
	var exact StringList
	if exact.UnmarshalJSON(data) == nil {
		*r = MockQueryRule{Op: MockQueryOpExact, Values: exact}
		return r.Check()
	}

	op, values, err := decodeMatcher(mockQueryOps, data)
	if err != nil {
		return err
	}
	*r = MockQueryRule{Op: MockQueryOp(op), Values: values}
	return r.Check()
}
