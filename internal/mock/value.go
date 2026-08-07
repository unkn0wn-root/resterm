package mock

import (
	"fmt"
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

func anyNumber(rel numberRelation, operand string) valuePredicate {
	want, _ := parseNumber(operand)
	return anyValue(func(v string) bool {
		got, ok := parseNumber(v)
		return ok && rel.holds(got.cmp(want))
	})
}
