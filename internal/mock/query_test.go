package mock

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func queryRuleOf(op restfile.MockQueryOp, values ...string) restfile.MockQueryRule {
	return restfile.MockQueryRule{Op: op, Values: values}
}

func TestCompileQueryRulePredicates(t *testing.T) {
	tests := []struct {
		name string
		rule restfile.MockQueryRule
		got  []string
		want bool
	}{
		{"exact single", queryRuleOf(restfile.MockQueryOpExact, "a"), []string{"a"}, true},
		{"exact is ordered", queryRuleOf(restfile.MockQueryOpExact, "a", "b"), []string{"b", "a"}, false},
		{"exact rejects extra", queryRuleOf(restfile.MockQueryOpExact, "a"), []string{"a", "b"}, false},
		{"exact missing", queryRuleOf(restfile.MockQueryOpExact, "a"), nil, false},

		// the string operators share value.go with headers, so one case each is
		// enough to show the query side is wired to them
		{"prefix", queryRuleOf(restfile.MockQueryOpPrefix, "pay_"), []string{"x", "pay_1"}, true},
		{"contains", queryRuleOf(restfile.MockQueryOpContains, "bc"), []string{"abcd"}, true},
		{"regex is unanchored", queryRuleOf(restfile.MockQueryOpRegex, "v[0-9]"), []string{"api-v2"}, true},
		{"oneOf hit", queryRuleOf(restfile.MockQueryOpOneOf, "A", "B"), []string{"B"}, true},
		{"oneOf is exact per value", queryRuleOf(restfile.MockQueryOpOneOf, "A"), []string{"AA"}, false},
		{"present", queryRuleOf(restfile.MockQueryOpPresent), []string{""}, true},
		{"absent", queryRuleOf(restfile.MockQueryOpAbsent), nil, true},
		{"absent with an empty value", queryRuleOf(restfile.MockQueryOpAbsent), []string{""}, false},

		{"gt", queryRuleOf(restfile.MockQueryOpGT, "10"), []string{"11"}, true},
		{"gt at the boundary", queryRuleOf(restfile.MockQueryOpGT, "10"), []string{"10"}, false},
		{"gte at the boundary", queryRuleOf(restfile.MockQueryOpGTE, "10"), []string{"10"}, true},
		{"gte decimal form", queryRuleOf(restfile.MockQueryOpGTE, "10"), []string{"10.0"}, true},
		{"lt", queryRuleOf(restfile.MockQueryOpLT, "10"), []string{"9.999"}, true},
		{"lte at the boundary", queryRuleOf(restfile.MockQueryOpLTE, "10"), []string{"10"}, true},
		{"negative operand", queryRuleOf(restfile.MockQueryOpGT, "-5"), []string{"-4"}, true},
		{"text is not a number", queryRuleOf(restfile.MockQueryOpGT, "10"), []string{"eleven"}, false},
		{"infinity never compares", queryRuleOf(restfile.MockQueryOpGT, "0"), []string{"Inf"}, false},
		{"numeric any repeated", queryRuleOf(restfile.MockQueryOpGT, "10"), []string{"x", "11"}, true},
		{"numeric missing", queryRuleOf(restfile.MockQueryOpLT, "10"), nil, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			match, err := compileQueryRule(test.rule)
			if err != nil {
				t.Fatal(err)
			}
			if got := match(test.got); got != test.want {
				t.Fatalf("%s(%q) = %t, want %t", test.rule.Op, test.got, got, test.want)
			}
		})
	}
}

// Fails when an operator is added to the restfile table without a case here. The
// list is manual so a new operator has to be added in both packages.
func TestCompileQueryRuleCoversEveryOperator(t *testing.T) {
	ops := []restfile.MockQueryOp{
		restfile.MockQueryOpExact,
		restfile.MockQueryOpPrefix,
		restfile.MockQueryOpPresent,
		restfile.MockQueryOpAbsent,
		restfile.MockQueryOpContains,
		restfile.MockQueryOpRegex,
		restfile.MockQueryOpOneOf,
		restfile.MockQueryOpGT,
		restfile.MockQueryOpGTE,
		restfile.MockQueryOpLT,
		restfile.MockQueryOpLTE,
	}
	for _, op := range ops {
		t.Run(op.String(), func(t *testing.T) {
			// the operand shape differs per operator, so try each until the rule
			// is one the operator accepts
			for _, values := range [][]string{nil, {"1"}, {"1", "2"}} {
				rule := restfile.MockQueryRule{Op: op, Values: values}
				if rule.Check() != nil {
					continue
				}
				if _, err := compileQueryRule(rule); err != nil {
					t.Fatalf("%s matcher does not compile: %v; add a case for it", op, err)
				}
				return
			}
			t.Fatalf("no operand shape satisfied the %s matcher", op)
		})
	}

	if _, err := compileQueryRule(restfile.MockQueryRule{Op: restfile.MockQueryOp(99)}); err == nil {
		t.Fatal("an out of range operator compiled")
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
			src:  map[string]restfile.MockQueryRule{"  ": queryRuleOf(restfile.MockQueryOpPresent)},
			want: "name cannot be empty",
		},
		{
			name: "zero rule",
			src:  map[string]restfile.MockQueryRule{"page": {}},
			want: "matcher operation is invalid",
		},
		{
			name: "empty prefix",
			src:  map[string]restfile.MockQueryRule{"job": queryRuleOf(restfile.MockQueryOpPrefix, "")},
			want: `mock query matcher "job": prefix matcher requires one non-empty value`,
		},
		{
			name: "malformed regex",
			src:  map[string]restfile.MockQueryRule{"v": queryRuleOf(restfile.MockQueryOpRegex, "^v[0-9")},
			want: "not a valid regular expression",
		},
		{
			name: "operand out of range",
			src:  map[string]restfile.MockQueryRule{"n": queryRuleOf(restfile.MockQueryOpGT, "1e999999999")},
			want: "out of range",
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
		"Page": queryRuleOf(restfile.MockQueryOpGTE, "2"),
		"page": queryRuleOf(restfile.MockQueryOpGTE, "5"),
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
	if !rules.matches(url.Values{"Page": {"3"}, "page": {"9"}}) {
		t.Fatal("both rules should hold")
	}
	if rules.matches(url.Values{"Page": {"3"}, "page": {"1"}}) {
		t.Fatal("the lowercase rule should have failed")
	}
}

func TestCompileQueryRulesClonesValues(t *testing.T) {
	values := []string{"A", "B"}
	src := map[string]restfile.MockQueryRule{
		"env": queryRuleOf(restfile.MockQueryOpOneOf, values...),
	}
	rules, err := compileQueryRules(src)
	if err != nil {
		t.Fatal(err)
	}
	values[0] = "C"
	if !rules.matches(url.Values{"env": {"A"}}) {
		t.Fatal("mutating the caller's slice changed a compiled rule")
	}
	if got := rules.declared()["env"]; got.Values[0] != "A" {
		t.Fatalf("declared rule = %+v", got)
	}
}
