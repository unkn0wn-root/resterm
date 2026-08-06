package mock

import (
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// headerPredicate reports whether one header's values satisfy a rule. A header
// the request never sent arrives as nil.
type headerPredicate func(got []string) bool

type headerRule struct {
	name  string
	rule  restfile.MockHeaderRule
	match headerPredicate
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

// declared rebuilds the rule map. RequestPattern is returned to callers and
// serialized, so the normalized rules live next to the predicates.
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

// compileHeaderRules validates each rule, rekeys it by canonical header name,
// and compiles it. Names are visited in sorted order so a block with several
// problems always reports the same one.
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

// compileHeaderRule turns a rule into a predicate. The predicate captures the
// values and outlives the caller's map, so callers clone them first.
func compileHeaderRule(rule restfile.MockHeaderRule) (headerPredicate, error) {
	if err := rule.Check(); err != nil {
		return nil, err
	}
	switch rule.Op {
	case restfile.MockHeaderOpExact:
		want := rule.Values
		return func(got []string) bool { return got != nil && slices.Equal(got, want) }, nil
	case restfile.MockHeaderOpPrefix:
		prefix := rule.Values[0]
		return anyValue(func(v string) bool { return strings.HasPrefix(v, prefix) }), nil
	case restfile.MockHeaderOpContains:
		sub := rule.Values[0]
		return anyValue(func(v string) bool { return strings.Contains(v, sub) }), nil
	case restfile.MockHeaderOpRegex:
		re, err := regexp.Compile(rule.Values[0])
		if err != nil {
			return nil, fmt.Errorf("%s matcher is not a valid regular expression: %w", rule.Op, err)
		}
		return anyValue(re.MatchString), nil
	case restfile.MockHeaderOpOneOf:
		allowed := make(map[string]struct{}, len(rule.Values))
		for _, v := range rule.Values {
			allowed[v] = struct{}{}
		}
		return anyValue(func(v string) bool { _, ok := allowed[v]; return ok }), nil
	case restfile.MockHeaderOpPresent:
		return func(got []string) bool { return len(got) > 0 }, nil
	case restfile.MockHeaderOpAbsent:
		return func(got []string) bool { return len(got) == 0 }, nil
	default:
		return nil, fmt.Errorf("%s matcher is not supported", rule.Op)
	}
}

// anyValue turns a single-value test into one that a repeated header passes as
// soon as any of its values matches.
func anyValue(test func(string) bool) headerPredicate {
	return func(got []string) bool { return slices.ContainsFunc(got, test) }
}
