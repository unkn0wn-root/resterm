package mock

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestCompileJSONMatcher(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		body    string
		want    bool
	}{
		{"subset keeps extra fields", `{"a":1}`, `{"a":1,"b":2}`, true},
		{"missing field", `{"a":1}`, `{"b":2}`, false},
		{"nested subset", `{"u":{"id":1}}`, `{"u":{"id":1,"name":"x"}}`, true},
		{"arrays are exact", `{"a":[1,2]}`, `{"a":[1,2,3]}`, false},
		{"null matches null", `{"a":null}`, `{"a":null}`, true},
		{"null does not match missing", `{"a":null}`, `{"b":1}`, false},

		{"gt", `{"amount":{"$gt":100}}`, `{"amount":101}`, true},
		{"gt at the boundary", `{"amount":{"$gt":100}}`, `{"amount":100}`, false},
		{"gte at the boundary", `{"age":{"$gte":18}}`, `{"age":18}`, true},
		{"lt", `{"price":{"$lt":500}}`, `{"price":499.99}`, true},
		{"lte at the boundary", `{"price":{"$lte":500}}`, `{"price":500.0}`, true},
		{"exponent form", `{"n":{"$gte":100}}`, `{"n":1e2}`, true},
		{"large integers stay exact", `{"n":{"$gt":9007199254740992}}`, `{"n":9007199254740993}`, true},
		// a JSON string is never a number, so the comparison cannot coerce
		{"string is not a number", `{"amount":{"$gt":100}}`, `{"amount":"101"}`, false},
		{"numeric on a missing field", `{"amount":{"$gt":0}}`, `{"other":1}`, false},

		{"oneOf strings", `{"status":{"$oneOf":["A","B"]}}`, `{"status":"B"}`, true},
		{"oneOf miss", `{"status":{"$oneOf":["A","B"]}}`, `{"status":"C"}`, false},
		{"oneOf compares numbers by value", `{"n":{"$oneOf":[1]}}`, `{"n":1.0}`, true},
		{"oneOf null", `{"v":{"$oneOf":[null,"x"]}}`, `{"v":null}`, true},
		{"oneOf arrays", `{"v":{"$oneOf":[[1,2]]}}`, `{"v":[1,2]}`, true},
		{"oneOf mixed types", `{"v":{"$oneOf":[1,"1",true]}}`, `{"v":"1"}`, true},
		// an alternative is compared whole, unlike a bare object pattern
		{"oneOf objects are exact", `{"v":{"$oneOf":[{"a":1}]}}`, `{"v":{"a":1,"b":2}}`, false},
		{"oneOf object hit", `{"v":{"$oneOf":[{"a":1}]}}`, `{"v":{"a":1}}`, true},
		{"oneOf object value differs", `{"v":{"$oneOf":[{"a":1}]}}`, `{"v":{"a":2}}`, false},
		{"oneOf array element differs", `{"v":{"$oneOf":[[1,2]]}}`, `{"v":[1,3]}`, false},

		{"operator nested in an object", `{"u":{"age":{"$gte":18}}}`, `{"u":{"age":21}}`, true},
		{"operator inside an array", `{"a":[{"$gt":1},{"$lt":9}]}`, `{"a":[2,8]}`, true},
		{"operator inside an array misses", `{"a":[{"$gt":1}]}`, `{"a":[0]}`, false},
		{"operator on an array element field", `{"a":[{"n":{"$gt":1}}]}`, `{"a":[{"n":2,"x":0}]}`, true},
		{"operator at the top level", `{"$gt":10}`, `11`, true},
		{"oneOf at the top level", `{"$oneOf":["a"]}`, `"a"`, true},

		// a field named gt carries no $ and stays ordinary data
		{"unprefixed gt is a field", `{"gt":100}`, `{"gt":100}`, true},
		{"unprefixed gt is not an operator", `{"gt":100}`, `{"gt":101}`, false},
		{"nested unprefixed gt", `{"range":{"gt":1}}`, `{"range":{"gt":1,"lt":9}}`, true},

		// $ is a naming convention, not a reserved namespace, so dollar-prefixed
		// data stays matchable
		{
			"JSON Schema document",
			`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`,
			`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","title":"x"}`,
			true,
		},
		{"lone $ref", `{"$ref":"#/$defs/user"}`, `{"$ref":"#/$defs/user"}`, true},
		{"nested $id", `{"schema":{"$id":"urn:x"}}`, `{"schema":{"$id":"urn:x","type":"object"}}`, true},
		{"unregistered name is a field", `{"$between":[1,9]}`, `{"$between":[1,9]}`, true},
		{"a trailing space is a different name", `{"$gte ":1}`, `{"$gte ":1}`, true},
		// an operator name next to a sibling is data, which is what makes a Mongo
		// shaped body matchable
		{"operator name with a sibling", `{"$gt":1,"$lt":9}`, `{"$gt":1,"$lt":9,"x":0}`, true},
		// a sole registered name is always the operator, so $oneOf is how a body
		// field shaped exactly like one gets matched as data
		{"sole operator name reads as an operator", `{"age":{"$gt":21}}`, `{"age":22}`, true},
		{"oneOf reaches the literal form", `{"age":{"$oneOf":[{"$gt":21}]}}`, `{"age":{"$gt":21}}`, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			match, err := compileJSONMatcher([]byte(test.pattern))
			if err != nil {
				t.Fatal(err)
			}
			body, err := decodeJSON([]byte(test.body))
			if err != nil {
				t.Fatal(err)
			}
			if got := match(body); got != test.want {
				t.Fatalf("%s against %s = %t, want %t", test.pattern, test.body, got, test.want)
			}
		})
	}
}

func TestCompileJSONMatcherRejects(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{"malformed JSON", `{"a":`, "invalid JSON matcher"},
		// the message names the keyword that failed, not just its shape
		{"string operand", `{"a":{"$gt":"100"}}`, "$gt matcher requires a number"},
		{"operand out of range", `{"a":{"$gt":1e999999999}}`, "$gt matcher operand 1e999999999 is out of range"},
		{"oneOf scalar", `{"a":{"$oneOf":"x"}}`, "$oneOf matcher requires a non-empty array"},
		// the field path points at where the mistake is
		{"nested error names the field", `{"u":{"age":{"$gt":"x"}}}`, `field "u": field "age"`},
		{"array index error", `{"a":[{"$oneOf":[]}]}`, `field "a": index 0`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileJSONMatcher([]byte(test.pattern))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compileJSONMatcher(%s) error = %v, want %q", test.pattern, err, test.want)
			}
		})
	}
}

// A new operator has to use the $ naming and name itself in errors. Each one
// brings its own invalid operand so this does not have to guess which shapes a
// later operator will accept.
func TestJSONOpsFollowTheTableContract(t *testing.T) {
	invalid := map[string]string{
		"$gt":    `"100"`,
		"$gte":   `null`,
		"$lt":    `[1]`,
		"$lte":   `true`,
		"$oneOf": `[]`,
	}
	if !slices.Equal(slices.Sorted(maps.Keys(invalid)), slices.Sorted(maps.Keys(jsonOps))) {
		t.Fatalf("operators = %q, covered here = %q; add the new one",
			slices.Sorted(maps.Keys(jsonOps)), slices.Sorted(maps.Keys(invalid)))
	}
	for name, operand := range invalid {
		t.Run(name, func(t *testing.T) {
			if !strings.HasPrefix(name, "$") {
				t.Fatalf("operator %q needs the $ convention to stay clear of real field names", name)
			}
			_, err := compileJSONMatcher([]byte(`{"x":{"` + name + `":` + operand + `}}`))
			if err == nil {
				t.Fatalf("%s accepted %s", name, operand)
			}
			if !strings.Contains(err.Error(), name+" matcher") {
				t.Fatalf("%s error does not name the operator: %v", name, err)
			}
		})
	}
}

func TestJSONMatcherComparesValuesNotFormatting(t *testing.T) {
	match, err := compileJSONMatcher([]byte(`{"a":{"$oneOf":[100]}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"a":100}`, `{"a":100.0}`, `{"a":1e2}`} {
		got, err := decodeJSON([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		if !match(got) {
			t.Fatalf("%s did not match $oneOf:[100]", body)
		}
	}
}
