package mock

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type queryRule struct {
	name  string
	rule  restfile.MockQueryRule
	match valuePredicate
}

// queryRules is a compiled @match query block. Every rule has to hold.
type queryRules []queryRule

func (rs queryRules) matches(q url.Values) bool {
	for _, r := range rs {
		if !r.match(q[r.name]) {
			return false
		}
	}
	return true
}

func (rs queryRules) declared() map[string]restfile.MockQueryRule {
	if len(rs) == 0 {
		return nil
	}

	out := make(map[string]restfile.MockQueryRule, len(rs))
	for _, r := range rs {
		out[r.name] = r.rule
	}
	return out
}

// compileQueryRules sorts names so a block with several problems always reports
// the same one. Query names are case sensitive, so unlike headers they are used
// as written.
func compileQueryRules(src map[string]restfile.MockQueryRule) (queryRules, error) {
	if len(src) == 0 {
		return nil, nil
	}

	out := make(queryRules, 0, len(src))
	for _, name := range slices.Sorted(maps.Keys(src)) {
		if strings.TrimSpace(name) == "" {
			return nil, errors.New("mock query matcher name cannot be empty")
		}

		rule := src[name]
		rule.Values = slices.Clone(rule.Values)
		match, err := compileQueryRule(rule)
		if err != nil {
			return nil, fmt.Errorf("mock query matcher %q: %w", name, err)
		}
		out = append(out, queryRule{name: name, rule: rule, match: match})
	}
	return out, nil
}

// compileQueryRule builds a predicate that keeps the values after this returns,
// so callers clone them first.
func compileQueryRule(rule restfile.MockQueryRule) (valuePredicate, error) {
	if err := rule.Check(); err != nil {
		return nil, err
	}
	switch rule.Op {
	case restfile.MockQueryOpExact:
		return exactValues(rule.Values), nil
	case restfile.MockQueryOpPrefix:
		return anyPrefix(rule.Values[0]), nil
	case restfile.MockQueryOpContains:
		return anyContains(rule.Values[0]), nil
	case restfile.MockQueryOpRegex:
		return anyRegex(rule.Values[0])
	case restfile.MockQueryOpOneOf:
		return anyOneOf(rule.Values), nil
	case restfile.MockQueryOpPresent:
		return valuePresent, nil
	case restfile.MockQueryOpAbsent:
		return valueAbsent, nil
	case restfile.MockQueryOpGT:
		return anyNumber(relGT, rule.Values[0]), nil
	case restfile.MockQueryOpGTE:
		return anyNumber(relGTE, rule.Values[0]), nil
	case restfile.MockQueryOpLT:
		return anyNumber(relLT, rule.Values[0]), nil
	case restfile.MockQueryOpLTE:
		return anyNumber(relLTE, rule.Values[0]), nil
	default:
		return nil, fmt.Errorf("%s matcher is not supported", rule.Op)
	}
}
