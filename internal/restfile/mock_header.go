package restfile

import (
	"errors"
	"fmt"

	"golang.org/x/net/http/httpguts"
)

// mockHeaderOps is indexed by MockHeaderOp. Only one operator can use
// operandShorthand, because the bare JSON form has no keyword to tell them apart.
var mockHeaderOps = []matcherSpec{
	MockHeaderOpExact:    {name: "exact", operand: operandShorthand},
	MockHeaderOpPrefix:   {name: "prefix", operand: operandString},
	MockHeaderOpPresent:  {name: "present", operand: operandFlag},
	MockHeaderOpAbsent:   {name: "absent", operand: operandFlag},
	MockHeaderOpContains: {name: "contains", operand: operandString},
	MockHeaderOpRegex:    {name: "regex", operand: operandString, verify: verifyRegex},
	MockHeaderOpOneOf:    {name: "oneOf", operand: operandList},
}

func (op MockHeaderOp) String() string {
	spec, ok := specAt(mockHeaderOps, int(op))
	if !ok {
		return "unknown"
	}
	return spec.name
}

func (r MockHeaderRule) Check() error {
	spec, ok := specAt(mockHeaderOps, int(r.Op))
	if !ok {
		return errors.New("matcher operation is invalid")
	}
	for _, value := range r.Values {
		if !httpguts.ValidHeaderFieldValue(value) {
			return fmt.Errorf("%s matcher value %q is not a valid header value", spec.name, value)
		}
	}
	return spec.check(r.Values)
}

func (r MockHeaderRule) MarshalJSON() ([]byte, error) {
	if err := r.Check(); err != nil {
		return nil, err
	}
	spec, _ := specAt(mockHeaderOps, int(r.Op))
	return spec.marshal(r.Values)
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

	op, values, err := decodeMatcher(mockHeaderOps, data)
	if err != nil {
		return err
	}
	*r = MockHeaderRule{Op: MockHeaderOp(op), Values: values}
	return r.Check()
}
