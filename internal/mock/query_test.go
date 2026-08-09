package mock

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func queryRuleOf(op restfile.MockMatchOp, values ...string) restfile.MockQueryRule {
	return restfile.MockQueryRule{Op: op, Values: values}
}

func TestCompileQueryRulePredicates(t *testing.T) {
	tests := []struct {
		name string
		rule restfile.MockQueryRule
		got  []string
		want bool
	}{
		{"exact single", queryRuleOf(restfile.MockOpExact, "a"), []string{"a"}, true},
		{"exact is ordered", queryRuleOf(restfile.MockOpExact, "a", "b"), []string{"b", "a"}, false},
		{"exact rejects extra", queryRuleOf(restfile.MockOpExact, "a"), []string{"a", "b"}, false},
		{"exact missing", queryRuleOf(restfile.MockOpExact, "a"), nil, false},

		// the string operators share value.go with headers, so one case each is
		// enough to show the query side is wired to them
		{"prefix", queryRuleOf(restfile.MockOpPrefix, "pay_"), []string{"x", "pay_1"}, true},
		{"contains", queryRuleOf(restfile.MockOpContains, "bc"), []string{"abcd"}, true},
		{"regex is unanchored", queryRuleOf(restfile.MockOpRegex, "v[0-9]"), []string{"api-v2"}, true},
		{"oneOf hit", queryRuleOf(restfile.MockOpOneOf, "A", "B"), []string{"B"}, true},
		{"oneOf is exact per value", queryRuleOf(restfile.MockOpOneOf, "A"), []string{"AA"}, false},
		{"present", queryRuleOf(restfile.MockOpPresent), []string{""}, true},
		{"absent", queryRuleOf(restfile.MockOpAbsent), nil, true},
		{"absent with an empty value", queryRuleOf(restfile.MockOpAbsent), []string{""}, false},

		{"gt", queryRuleOf(restfile.MockOpGT, "10"), []string{"11"}, true},
		{"gt at the boundary", queryRuleOf(restfile.MockOpGT, "10"), []string{"10"}, false},
		{"gte at the boundary", queryRuleOf(restfile.MockOpGTE, "10"), []string{"10"}, true},
		{"gte decimal form", queryRuleOf(restfile.MockOpGTE, "10"), []string{"10.0"}, true},
		{"lt", queryRuleOf(restfile.MockOpLT, "10"), []string{"9.999"}, true},
		{"lte at the boundary", queryRuleOf(restfile.MockOpLTE, "10"), []string{"10"}, true},
		{"negative operand", queryRuleOf(restfile.MockOpGT, "-5"), []string{"-4"}, true},
		{"text is not a number", queryRuleOf(restfile.MockOpGT, "10"), []string{"eleven"}, false},
		{"infinity never compares", queryRuleOf(restfile.MockOpGT, "0"), []string{"Inf"}, false},
		{"runaway exponent", queryRuleOf(restfile.MockOpGT, "1e999999999"), []string{"1e1000000000"}, true},
		{"runaway exponent misses", queryRuleOf(restfile.MockOpLTE, "0"), []string{"1e-999999999"}, false},
		{"numeric any repeated", queryRuleOf(restfile.MockOpGT, "10"), []string{"x", "11"}, true},
		{"numeric missing", queryRuleOf(restfile.MockOpLT, "10"), nil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, match, err := compileQueryRule(test.rule)
			if err != nil {
				t.Fatal(err)
			}
			if got := match(test.got); got != test.want {
				t.Fatalf("%s(%q) = %t, want %t", test.rule.Op, test.got, got, test.want)
			}
		})
	}
}

func TestCompileQueryRulesValidatesNamesAndRules(t *testing.T) {
	tests := []struct {
		name string
		src  map[string]restfile.MockQueryRule
		want string
	}{
		{
			name: "empty name",
			src:  map[string]restfile.MockQueryRule{"  ": queryRuleOf(restfile.MockOpPresent)},
			want: "name cannot be empty",
		},
		{
			name: "zero rule",
			src:  map[string]restfile.MockQueryRule{"page": {}},
			want: "matcher operation is invalid",
		},
		{
			name: "empty prefix",
			src:  map[string]restfile.MockQueryRule{"job": queryRuleOf(restfile.MockOpPrefix, "")},
			want: `mock query matcher "job": prefix matcher requires one non-empty value`,
		},
		{
			name: "malformed regex",
			src:  map[string]restfile.MockQueryRule{"v": queryRuleOf(restfile.MockOpRegex, "^v[0-9")},
			want: "not a valid regular expression",
		},
		{
			name: "non-numeric operand",
			src:  map[string]restfile.MockQueryRule{"n": queryRuleOf(restfile.MockOpGT, "ten")},
			want: "gt matcher requires one JSON number",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileQueryRules(test.src)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compileQueryRules() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestQueryRulesUseNamesAsWritten(t *testing.T) {
	rules, err := compileQueryRules(map[string]restfile.MockQueryRule{
		"Page": queryRuleOf(restfile.MockOpGTE, "2"),
		"page": queryRuleOf(restfile.MockOpGTE, "5"),
	})
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(rules))
	for i, r := range rules {
		names[i] = r.name
	}
	// sorted by the source key, and query names are case sensitive so both stay
	if !slices.Equal(names, []string{"Page", "page"}) {
		t.Fatalf("names = %q", names)
	}
	if !rules.matches(queryLookup(url.Values{"Page": {"3"}, "page": {"9"}})) {
		t.Fatal("both rules should hold")
	}
	if rules.matches(queryLookup(url.Values{"Page": {"3"}, "page": {"1"}})) {
		t.Fatal("the lowercase rule should have failed")
	}
}

func TestCompileQueryRulesClonesValues(t *testing.T) {
	values := []string{"A", "B"}
	src := map[string]restfile.MockQueryRule{
		"env": queryRuleOf(restfile.MockOpOneOf, values...),
	}
	rules, err := compileQueryRules(src)
	if err != nil {
		t.Fatal(err)
	}
	values[0] = "C"
	if !rules.matches(queryLookup(url.Values{"env": {"A"}})) {
		t.Fatal("mutating the caller's slice changed a compiled rule")
	}
	if got := rules.declared()["env"]; got.Values[0] != "A" {
		t.Fatalf("declared rule = %+v", got)
	}
}
