package restfile

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestCompileMockPathMapsSourceWildcards(t *testing.T) {
	pattern, params, err := CompileMockPath("/users/{id}/files/{path...}")
	if err != nil {
		t.Fatal(err)
	}
	if pattern != "/users/{p1}/files/{p3...}" {
		t.Fatalf("pattern = %q", pattern)
	}
	if params["id"] != "p1" || params["path"] != "p3" || len(params) != 2 {
		t.Fatalf("params = %#v", params)
	}

	other, _, err := CompileMockPath("/users/{userID}/files/{rest...}")
	if err != nil {
		t.Fatal(err)
	}
	if other != pattern {
		t.Fatalf("equivalent pattern = %q, want %q", other, pattern)
	}
}

func TestMockHeaderRuleJSONRoundTrip(t *testing.T) {
	tests := []struct {
		rule MockHeaderRule
		json string
	}{
		{MockHeaderRule{Op: MockHeaderOpExact, Values: []string{"one"}}, `"one"`},
		{MockHeaderRule{Op: MockHeaderOpExact, Values: []string{"one", "two"}}, `["one","two"]`},
		{MockHeaderRule{Op: MockHeaderOpPrefix, Values: []string{"Bearer "}}, `{"prefix":"Bearer "}`},
		{MockHeaderRule{Op: MockHeaderOpPresent}, `{"present":true}`},
		{MockHeaderRule{Op: MockHeaderOpAbsent}, `{"absent":true}`},
		{MockHeaderRule{Op: MockHeaderOpContains, Values: []string{"Chrome"}}, `{"contains":"Chrome"}`},
		{MockHeaderRule{Op: MockHeaderOpRegex, Values: []string{"^v[0-9]+$"}}, `{"regex":"^v[0-9]+$"}`},
		{
			MockHeaderRule{Op: MockHeaderOpOneOf, Values: []string{"dev", "prod"}},
			`{"oneOf":["dev","prod"]}`,
		},
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
			var got MockHeaderRule
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if got.Op != test.rule.Op || !slices.Equal(got.Values, test.rule.Values) {
				t.Fatalf("round trip = %+v, want %+v", got, test.rule)
			}
		})
	}
}

func TestMockHeaderRuleUnmarshalRejects(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"null", `null`, "cannot be null"},
		{"empty exact array", `[]`, "exact matcher requires at least one value"},
		{"number", `1`, "must be a string, string array, or object"},
		{"empty object", `{}`, "exactly one operator"},
		{"two operators", `{"present":true,"absent":true}`, "exactly one operator"},
		{"unknown operator", `{"matches":"v1"}`, `unknown matcher operator "matches"`},
		{"unnamed operator", `{"":true}`, `unknown matcher operator ""`},
		{"present false", `{"present":false}`, "present matcher must be true"},
		{"present with a value", `{"present":"yes"}`, "present matcher must be true"},
		{"empty prefix", `{"prefix":""}`, "prefix matcher requires one non-empty value"},
		{"empty contains", `{"contains":""}`, "contains matcher requires one non-empty value"},
		{"contains array", `{"contains":["a"]}`, "contains matcher must be a non-empty string"},
		{"empty regex", `{"regex":""}`, "regex matcher requires one non-empty value"},
		{"malformed regex", `{"regex":"^v[0-9"}`, "not a valid regular expression"},
		{"empty oneOf", `{"oneOf":[]}`, "oneOf matcher requires at least one value"},
		{"oneOf scalar", `{"oneOf":"dev"}`, "oneOf matcher must be a non-empty string array"},
		{"oneOf null", `{"oneOf":null}`, "oneOf matcher must be a non-empty string array"},
		{"value with a newline", `"a\nb"`, "is not a valid header value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var rule MockHeaderRule
			err := json.Unmarshal([]byte(test.json), &rule)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unmarshal(%s) error = %v, want %q", test.json, err, test.want)
			}
		})
	}
}

func TestMockHeaderRuleMarshalRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name string
		rule MockHeaderRule
	}{
		{"unknown op", MockHeaderRule{}},
		{"out of range op", MockHeaderRule{Op: MockHeaderOp(99)}},
		{"exact without values", MockHeaderRule{Op: MockHeaderOpExact}},
		{"prefix with two values", MockHeaderRule{Op: MockHeaderOpPrefix, Values: []string{"a", "b"}}},
		{"present with values", MockHeaderRule{Op: MockHeaderOpPresent, Values: []string{"yes"}}},
		{"oneOf without values", MockHeaderRule{Op: MockHeaderOpOneOf}},
		{"malformed regex", MockHeaderRule{Op: MockHeaderOpRegex, Values: []string{"^v[0-9"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if data, err := json.Marshal(test.rule); err == nil {
				t.Fatalf("marshal = %s, want an error", data)
			}
		})
	}
}

func TestMockHeaderOpNames(t *testing.T) {
	names := map[MockHeaderOp]string{
		MockHeaderOpExact:    "exact",
		MockHeaderOpPrefix:   "prefix",
		MockHeaderOpPresent:  "present",
		MockHeaderOpAbsent:   "absent",
		MockHeaderOpContains: "contains",
		MockHeaderOpRegex:    "regex",
		MockHeaderOpOneOf:    "oneOf",
	}
	declared := 0
	for index, spec := range mockHeaderOps {
		if spec.name == "" {
			continue
		}
		declared++
		op := MockHeaderOp(index)
		want, ok := names[op]
		if !ok {
			t.Fatalf("operator %d has no expected name; add it here and to the docs", op)
		}
		if got := op.String(); got != want {
			t.Fatalf("op %d String() = %q, want %q", op, got, want)
		}
		if back, ok := mockHeaderOpNamed(want); !ok || back != op {
			t.Fatalf("mockHeaderOpNamed(%q) = %v, %t", want, back, ok)
		}
	}
	if declared != len(names) {
		t.Fatalf("mockHeaderOps declares %d operators, want %d", declared, len(names))
	}
	if got := MockHeaderOpUnknown.String(); got != "unknown" {
		t.Fatalf("unknown String() = %q", got)
	}
	// index 0 has no name, so an empty JSON key must not resolve to it
	if _, ok := mockHeaderOpNamed(""); ok {
		t.Fatal("the empty name resolved to an operator")
	}
}

func TestCompileMockPathRejectsRepeatedWildcardNames(t *testing.T) {
	_, _, err := CompileMockPath("/compare/{id}/{id}")
	if err == nil || !strings.Contains(err.Error(), "is repeated") {
		t.Fatalf("CompileMockPath() error = %v", err)
	}
}
