package mock

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type headerRule struct {
	name  string
	rule  restfile.MockHeaderRule
	match valuePredicate
}

// headerRules is a compiled @match headers block. Every rule has to hold.
type headerRules []headerRule

func (rs headerRules) matches(h http.Header, host string) bool {
	for _, r := range rs {
		if !r.match(headerOrHost(h, host, r.name)) {
			return false
		}
	}
	return true
}

// declared rebuilds the rule map, which RequestPattern returns and serializes.
func (rs headerRules) declared() map[string]restfile.MockHeaderRule {
	if len(rs) == 0 {
		return nil
	}

	out := make(map[string]restfile.MockHeaderRule, len(rs))
	for _, r := range rs {
		out[r.name] = r.rule
	}
	return out
}

// compileHeaderRules stores each rule under the canonical header name. Sorted
// order means a block with several problems always reports the same one.
func compileHeaderRules(src map[string]restfile.MockHeaderRule) (headerRules, error) {
	if len(src) == 0 {
		return nil, nil
	}

	out := make(headerRules, 0, len(src))
	for _, key := range slices.Sorted(maps.Keys(src)) {
		name := strings.TrimSpace(key)
		if !httpguts.ValidHeaderFieldName(name) {
			return nil, fmt.Errorf("invalid mock header matcher %q", name)
		}

		rule := src[key]
		rule.Values = slices.Clone(rule.Values)
		match, err := compileHeaderRule(rule)
		if err != nil {
			return nil, fmt.Errorf("mock header matcher %q: %w", name, err)
		}

		name = http.CanonicalHeaderKey(name)
		if slices.ContainsFunc(out, func(r headerRule) bool { return r.name == name }) {
			return nil, fmt.Errorf("mock header matcher %q is repeated with different casing", name)
		}
		out = append(out, headerRule{name: name, rule: rule, match: match})
	}
	return out, nil
}

// compileHeaderRule builds a predicate that keeps the values after this returns,
// so callers clone them first.
func compileHeaderRule(rule restfile.MockHeaderRule) (valuePredicate, error) {
	if err := rule.Check(); err != nil {
		return nil, err
	}
	switch rule.Op {
	case restfile.MockHeaderOpExact:
		return exactValues(rule.Values), nil
	case restfile.MockHeaderOpPrefix:
		return anyPrefix(rule.Values[0]), nil
	case restfile.MockHeaderOpContains:
		return anyContains(rule.Values[0]), nil
	case restfile.MockHeaderOpRegex:
		return anyRegex(rule.Values[0])
	case restfile.MockHeaderOpOneOf:
		return anyOneOf(rule.Values), nil
	case restfile.MockHeaderOpPresent:
		return valuePresent, nil
	case restfile.MockHeaderOpAbsent:
		return valueAbsent, nil
	default:
		return nil, fmt.Errorf("%s matcher is not supported", rule.Op)
	}
}
