package restfile

import (
	"errors"
	"fmt"

	"golang.org/x/net/http/httpguts"
)

// The numeric comparisons are left out because a header value is text.
var mockHeaderOps = matcherOps{
	MockOpExact,
	MockOpPrefix,
	MockOpPresent,
	MockOpAbsent,
	MockOpContains,
	MockOpRegex,
	MockOpOneOf,
}

func (r MockHeaderRule) Check() error {
	spec, ok := mockHeaderOps.spec(r.Op)
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
	return MockRule(r).marshal(mockHeaderOps)
}

func (r *MockHeaderRule) UnmarshalJSON(data []byte) error {
	if err := (*MockRule)(r).unmarshal(mockHeaderOps, data); err != nil {
		return err
	}
	return r.Check()
}
