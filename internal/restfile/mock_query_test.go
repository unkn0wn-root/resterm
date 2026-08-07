package restfile

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestMockQueryRuleJSONRoundTrip(t *testing.T) {
	tests := []struct {
		rule MockQueryRule
		json string
	}{
		{MockQueryRule{Op: MockOpExact, Values: []string{"one"}}, `"one"`},
		{MockQueryRule{Op: MockOpExact, Values: []string{"one", "two"}}, `["one","two"]`},
		{MockQueryRule{Op: MockOpPrefix, Values: []string{"pay_"}}, `{"prefix":"pay_"}`},
		{MockQueryRule{Op: MockOpPresent}, `{"present":true}`},
		{MockQueryRule{Op: MockOpAbsent}, `{"absent":true}`},
		{MockQueryRule{Op: MockOpContains, Values: []string{"abc"}}, `{"contains":"abc"}`},
		{MockQueryRule{Op: MockOpRegex, Values: []string{"^v[0-9]+$"}}, `{"regex":"^v[0-9]+$"}`},
		{MockQueryRule{Op: MockOpOneOf, Values: []string{"A", "B"}}, `{"oneOf":["A","B"]}`},
		{MockQueryRule{Op: MockOpGT, Values: []string{"10"}}, `{"gt":10}`},
		{MockQueryRule{Op: MockOpGTE, Values: []string{"-1.5"}}, `{"gte":-1.5}`},
		{MockQueryRule{Op: MockOpLTE, Values: []string{"0"}}, `{"lte":0}`},
	}
	for _, test := range tests {
		t.Run(test.rule.Op.String()+"_"+test.json, func(t *testing.T) {
			data, err := json.Marshal(test.rule)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != test.json {
				t.Fatalf("marshal = %s, want %s", data, test.json)
			}
			var got MockQueryRule
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got.Op != test.rule.Op || !slices.Equal(got.Values, test.rule.Values) {
				t.Fatalf("round trip = %+v, want %+v", got, test.rule)
			}
		})
	}
}

func TestMockQueryRuleUnmarshalRejects(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"null", `null`, "cannot be null"},
		{"empty exact array", `[]`, "exact matcher requires at least one value"},
		{"boolean", `true`, "must be a string, string array, or object"},
		{"empty object", `{}`, "exactly one operator"},
		{"two operators", `{"gt":1,"lt":9}`, "exactly one operator"},
		{"unknown operator", `{"between":[1,9]}`, `unknown matcher operator "between"`},
		{"quoted number", `{"gt":"10"}`, "gt matcher must be a number, not a quoted string"},
		{"empty prefix", `{"prefix":""}`, "prefix matcher requires one non-empty value"},
		{"malformed regex", `{"regex":"^v[0-9"}`, "not a valid regular expression"},
		{"empty oneOf", `{"oneOf":[]}`, "oneOf matcher requires at least one value"},
		{"oneOf scalar", `{"oneOf":"A"}`, "oneOf matcher must be a non-empty string array"},
		{"present false", `{"present":false}`, "present matcher must be true"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var rule MockQueryRule
			err := json.Unmarshal([]byte(test.json), &rule)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unmarshal(%s) error = %v, want %q", test.json, err, test.want)
			}
		})
	}
}

func TestMockQueryRuleMarshalRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name string
		rule MockQueryRule
	}{
		{"unknown op", MockQueryRule{}},
		{"exact without values", MockQueryRule{Op: MockOpExact}},
		{"present with values", MockQueryRule{Op: MockOpPresent, Values: []string{"yes"}}},
		{"gt without values", MockQueryRule{Op: MockOpGT}},
		// an empty operand would otherwise be written out as the number 0
		{"gt with an empty value", MockQueryRule{Op: MockOpGT, Values: []string{""}}},
		{"gt with text", MockQueryRule{Op: MockOpLT, Values: []string{"ten"}}},
		{"malformed regex", MockQueryRule{Op: MockOpRegex, Values: []string{"^v[0-9"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if data, err := json.Marshal(test.rule); err == nil {
				t.Fatalf("marshal = %s, want an error", data)
			}
		})
	}
}

// TestMockQueryRuleAcceptsValuesHeadersReject keeps the two rule types from
// drifting into one: a query value has no HTTP validity constraint.
// the bare operand also has an explicit spelling, which normalizes back to the
// bare form on the way out
func TestMatcherShorthandHasAnExplicitSpelling(t *testing.T) {
	var rule MockQueryRule
	if err := json.Unmarshal([]byte(`{"exact":["a","b"]}`), &rule); err != nil {
		t.Fatal(err)
	}
	if rule.Op != MockOpExact || !slices.Equal(rule.Values, []string{"a", "b"}) {
		t.Fatalf("explicit rule = %+v", rule)
	}
	data, err := json.Marshal(rule)
	if err != nil || string(data) != `["a","b"]` {
		t.Fatalf("marshal = %s, %v", data, err)
	}
	if err := json.Unmarshal([]byte(`{"exact":1}`), &rule); err == nil {
		t.Fatal("a non-string exact operand was accepted")
	}
}

func TestMockQueryRuleAcceptsValuesHeadersReject(t *testing.T) {
	raw := `"a\nb"`
	var query MockQueryRule
	if err := json.Unmarshal([]byte(raw), &query); err != nil {
		t.Fatalf("query rule rejected %s: %v", raw, err)
	}
	if query.Values[0] != "a\nb" {
		t.Fatalf("query value = %q", query.Values[0])
	}
	var header MockHeaderRule
	if err := json.Unmarshal([]byte(raw), &header); err == nil {
		t.Fatalf("header rule accepted %s", raw)
	}
}
