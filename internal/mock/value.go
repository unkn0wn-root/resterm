package mock

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"
)

// valuePredicate tests the values a request carried for one key. A key the
// request never sent arrives as nil.
type valuePredicate func(got []string) bool

func exactValues(want []string) valuePredicate {
	return func(got []string) bool { return got != nil && slices.Equal(got, want) }
}

// anyValue passes as soon as one value of a repeated key matches.
func anyValue(test func(string) bool) valuePredicate {
	return func(got []string) bool { return slices.ContainsFunc(got, test) }
}

func anyPrefix(prefix string) valuePredicate {
	return anyValue(func(v string) bool { return strings.HasPrefix(v, prefix) })
}

func anyContains(sub string) valuePredicate {
	return anyValue(func(v string) bool { return strings.Contains(v, sub) })
}

func anyRegex(pattern string) (valuePredicate, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("regex matcher is not a valid regular expression: %w", err)
	}
	return anyValue(re.MatchString), nil
}

func anyOneOf(values []string) valuePredicate {
	allowed := make(map[string]struct{}, len(values))
	for _, v := range values {
		allowed[v] = struct{}{}
	}
	return anyValue(func(v string) bool { _, ok := allowed[v]; return ok })
}

func valuePresent(got []string) bool { return len(got) > 0 }

func valueAbsent(got []string) bool { return len(got) == 0 }

// numberRelation puts the request value on the left, so relGT holds when the
// request value is the bigger one.
type numberRelation uint8

const (
	relGT numberRelation = iota + 1
	relGTE
	relLT
	relLTE
)

func (rel numberRelation) holds(cmp int) bool {
	switch rel {
	case relGT:
		return cmp > 0
	case relGTE:
		return cmp >= 0
	case relLT:
		return cmp < 0
	case relLTE:
		return cmp <= 0
	default:
		return false
	}
}

// anyNumber reads each value as a decimal number. Text that is not a number is a
// non-match rather than an error.
func anyNumber(rel numberRelation, operand string) (valuePredicate, error) {
	want, ok := parseNumber(operand)
	if !ok {
		return nil, fmt.Errorf("matcher operand %s is out of range", operand)
	}
	return anyValue(func(v string) bool {
		got, ok := parseNumber(v)
		return ok && rel.holds(got.Cmp(want))
	}), nil
}

const maxNumberText = 1024

// numberPrecision covers any number within the cap. Every value has to use the
// same width. 0.1 has no exact binary form, so parsing it at two widths gives
// two values that are not equal.
const numberPrecision = uint(maxNumberText * 4)

// parseNumber rejects Inf so a runaway exponent never compares against anything.
// big.Rat would be exact but lets a hostile body allocate 10^exp bytes.
func parseNumber(s string) (*big.Float, bool) {
	if len(s) > maxNumberText {
		return nil, false
	}
	f, _, err := big.ParseFloat(s, 10, numberPrecision, big.ToNearestEven)
	if err != nil || f.IsInf() {
		return nil, false
	}
	return f, true
}

func compareNumbers(a, b json.Number) (int, bool) {
	x, ok := parseNumber(string(a))
	if !ok {
		return 0, false
	}
	y, ok := parseNumber(string(b))
	if !ok {
		return 0, false
	}
	return x.Cmp(y), true
}

// equalJSONNumbers compares by value, so 100, 1e2 and 100.0 all match.
func equalJSONNumbers(want, got json.Number) bool {
	if want == got {
		return true
	}
	cmp, ok := compareNumbers(want, got)
	return ok && cmp == 0
}
