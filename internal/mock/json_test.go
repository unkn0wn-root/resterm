package mock

import (
	"strings"
	"testing"
)

func TestJSONSubsetMatchesLiteralData(t *testing.T) {
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
		{"arrays are ordered", `{"a":[1,2]}`, `{"a":[2,1]}`, false},
		{"array elements match as subsets", `{"a":[{"id":1}]}`, `{"a":[{"id":1,"x":0}]}`, true},
		{"null matches null", `{"a":null}`, `{"a":null}`, true},
		{"null does not match missing", `{"a":null}`, `{"b":1}`, false},
		{"empty object matches any object", `{}`, `{"a":1}`, true},
		{"empty object does not match a scalar", `{}`, `1`, false},
		{"a bare scalar pattern", `5`, `5`, true},
		{"a bare array pattern", `[1]`, `[1]`, true},
		{"booleans", `{"ok":true}`, `{"ok":true}`, true},
		{"a boolean is not a number", `{"ok":true}`, `{"ok":1}`, false},

		{"numbers compare by value", `{"n":1}`, `{"n":1.0}`, true},
		{"exponent form", `{"n":100}`, `{"n":1e2}`, true},
		{"large integers stay exact", `{"n":9007199254740993}`, `{"n":9007199254740992}`, false},
		{"a quoted number is a string", `{"n":1}`, `{"n":"1"}`, false},

		{"lone $gt is data", `{"amount":{"$gt":100}}`, `{"amount":{"$gt":100}}`, true},
		{"lone $gt does not compare", `{"amount":{"$gt":100}}`, `{"amount":101}`, false},
		{"$gt subset keeps siblings", `{"a":{"$gt":1}}`, `{"a":{"$gt":1,"$lt":9}}`, true},
		{"$oneOf is data", `{"v":{"$oneOf":["a"]}}`, `{"v":{"$oneOf":["a"]}}`, true},
		{
			"JSON Schema document",
			`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`,
			`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","title":"x"}`,
			true,
		},
		{"$ref", `{"$ref":"#/$defs/user"}`, `{"$ref":"#/$defs/user"}`, true},
		{"nested $id", `{"schema":{"$id":"urn:x"}}`, `{"schema":{"$id":"urn:x","type":"object"}}`, true},

		{"gt is data", `{"range":{"gt":125}}`, `{"range":{"gt":125}}`, true},
		{"gt does not compare", `{"range":{"gt":100}}`, `{"range":{"gt":125}}`, false},
		{"oneOf is data", `{"v":{"oneOf":["a","b"]}}`, `{"v":{"oneOf":["a","b"]}}`, true},

		{"dotted name", `{"a.b":1}`, `{"a.b":1,"a":{"b":2}}`, true},
		{"slashed name", `{"a/b":1}`, `{"a/b":1}`, true},
		{"unicode name", `{"ключ":"значение"}`, `{"ключ":"значение"}`, true},
		{"empty name", `{"":1}`, `{"":1}`, true},
		{"trailing space is a different name", `{"gt ":1}`, `{"gt ":1}`, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			match := mustCompileBody(t, test.pattern, "")
			if got := match(decodeBody(t, test.body)); got != test.want {
				t.Fatalf("%s against %s = %t, want %t", test.pattern, test.body, got, test.want)
			}
		})
	}
}

func TestJSONSubsetRejectsMalformedPatterns(t *testing.T) {
	for _, pattern := range []string{`{"a":`, `{"a":1}{"b":2}`, `nope`} {
		t.Run(pattern, func(t *testing.T) {
			_, err := compileJSONBody([]byte(pattern), nil)
			if err == nil || !strings.Contains(err.Error(), "invalid json matcher") {
				t.Fatalf("compileJSONBody(%s) error = %v", pattern, err)
			}
		})
	}
}

func TestJSONBodyCombinesLiteralsAndRules(t *testing.T) {
	tests := []struct {
		name   string
		subset string
		rules  string
		body   string
		want   bool
	}{
		{
			name: "both match", subset: `{"type":"order"}`, rules: `{"amount":{"gt":100}}`,
			body: `{"type":"order","amount":101}`, want: true,
		},
		{
			name: "the literal half fails", subset: `{"type":"order"}`, rules: `{"amount":{"gt":100}}`,
			body: `{"type":"refund","amount":101}`, want: false,
		},
		{
			name: "the rule half fails", subset: `{"type":"order"}`, rules: `{"amount":{"gt":100}}`,
			body: `{"type":"order","amount":100}`, want: false,
		},
		{
			name: "rules alone", rules: `{"amount":{"gt":100}}`,
			body: `{"amount":101}`, want: true,
		},
		{
			name: "the same field on both sides", subset: `{"amount":150}`, rules: `{"amount":{"gt":100}}`,
			body: `{"amount":150}`, want: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			match := mustCompileBody(t, test.subset, test.rules)
			if got := match(decodeBody(t, test.body)); got != test.want {
				t.Fatalf("body match = %t, want %t", got, test.want)
			}
		})
	}
}

func TestJSONBodyWithoutConditionsIsNil(t *testing.T) {
	match, err := compileJSONBody(nil, nil)
	if err != nil || match != nil {
		t.Fatalf("compileJSONBody(nil, nil) = %v, %v", match, err)
	}
}

func mustCompileBody(t *testing.T, subset, rules string) jsonPredicate {
	t.Helper()
	match, err := compileJSONBody([]byte(subset), []byte(rules))
	if err != nil {
		t.Fatal(err)
	}
	return match
}

func decodeBody(t *testing.T, body string) any {
	t.Helper()
	got, err := decodeJSON([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return got
}
