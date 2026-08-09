package mock

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// An operator with a restfile table row but no predicate here would only fail
// once somebody wrote it in a file, so a new operator goes in this list by hand.
func TestCompileRuleCoversEveryOperator(t *testing.T) {
	ops := []restfile.MockMatchOp{
		restfile.MockOpExact,
		restfile.MockOpPrefix,
		restfile.MockOpPresent,
		restfile.MockOpAbsent,
		restfile.MockOpContains,
		restfile.MockOpRegex,
		restfile.MockOpOneOf,
		restfile.MockOpGT,
		restfile.MockOpGTE,
		restfile.MockOpLT,
		restfile.MockOpLTE,
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
				if _, err := compileRule(op, values); err != nil {
					t.Fatalf("%s matcher does not compile: %v; add a case for it", op, err)
				}
				return
			}
			t.Fatalf("no operand shape satisfied the %s matcher", op)
		})
	}

	if _, err := compileRule(restfile.MockMatchOp(99), nil); err == nil {
		t.Fatal("an out of range operator compiled")
	}
}
