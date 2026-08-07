package restfile

// Query values carry no HTTP validity constraint, so unlike headers they can
// hold any text the URL decodes to.
var mockQueryOps = matcherOps{
	MockOpExact,
	MockOpPrefix,
	MockOpPresent,
	MockOpAbsent,
	MockOpContains,
	MockOpRegex,
	MockOpOneOf,
	MockOpGT,
	MockOpGTE,
	MockOpLT,
	MockOpLTE,
}

func (r MockQueryRule) Check() error {
	return MockRule(r).check(mockQueryOps)
}

func (r MockQueryRule) MarshalJSON() ([]byte, error) {
	if err := r.Check(); err != nil {
		return nil, err
	}
	return MockRule(r).marshal(mockQueryOps)
}

func (r *MockQueryRule) UnmarshalJSON(data []byte) error {
	if err := (*MockRule)(r).unmarshal(mockQueryOps, data); err != nil {
		return err
	}
	return r.Check()
}
