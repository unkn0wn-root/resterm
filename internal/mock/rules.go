package mock

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// The source rule travels with the predicate because RequestPattern reads it
// back and serializes it.
type rule[R any] struct {
	name  string
	rule  R
	match valuePredicate
}

// ruleSet is a compiled @match block for one field. Every rule has to hold.
type ruleSet[R any] []rule[R]

type (
	queryRules  = ruleSet[restfile.MockQueryRule]
	headerRules = ruleSet[restfile.MockHeaderRule]
)

func (rs ruleSet[R]) matches(values func(name string) []string) bool {
	for _, r := range rs {
		if !r.match(values(r.name)) {
			return false
		}
	}
	return true
}

func (rs ruleSet[R]) declared() map[string]R {
	if len(rs) == 0 {
		return nil
	}

	out := make(map[string]R, len(rs))
	for _, r := range rs {
		out[r.name] = r.rule
	}
	return out
}

// Names are visited in sorted order so a block with several problems always
// reports the same one.
func compileRules[R any](
	kind string,
	src map[string]R,
	canon func(name string) (string, error),
	compile func(R) (R, valuePredicate, error),
) (ruleSet[R], error) {
	if len(src) == 0 {
		return nil, nil
	}

	out := make(ruleSet[R], 0, len(src))
	for _, key := range slices.Sorted(maps.Keys(src)) {
		name, err := canon(key)
		if err != nil {
			return nil, err
		}
		kept, match, err := compile(src[key])
		if err != nil {
			return nil, fmt.Errorf("mock %s matcher %q: %w", kind, name, err)
		}
		// canon can fold two spellings onto one name, which the source map could
		// not catch.
		if slices.ContainsFunc(out, func(r rule[R]) bool { return r.name == name }) {
			return nil, fmt.Errorf("mock %s matcher %q is repeated with different casing", kind, name)
		}
		out = append(out, rule[R]{name: name, rule: kept, match: match})
	}
	return out, nil
}

// compileRule keeps the values after it returns, so callers pass a copy.
func compileRule(op restfile.MockMatchOp, values []string) (valuePredicate, error) {
	switch op {
	case restfile.MockOpExact:
		return exactValues(values), nil
	case restfile.MockOpPrefix:
		return anyPrefix(values[0]), nil
	case restfile.MockOpContains:
		return anyContains(values[0]), nil
	case restfile.MockOpRegex:
		return anyRegex(values[0])
	case restfile.MockOpOneOf:
		return anyOneOf(values), nil
	case restfile.MockOpPresent:
		return valuePresent, nil
	case restfile.MockOpAbsent:
		return valueAbsent, nil
	case restfile.MockOpGT:
		return anyNumber(relGT, values[0]), nil
	case restfile.MockOpGTE:
		return anyNumber(relGTE, values[0]), nil
	case restfile.MockOpLT:
		return anyNumber(relLT, values[0]), nil
	case restfile.MockOpLTE:
		return anyNumber(relLTE, values[0]), nil
	default:
		return nil, fmt.Errorf("%s matcher is not supported", op)
	}
}

func queryLookup(q url.Values) func(string) []string {
	return func(name string) []string { return q[name] }
}

func headerLookup(h http.Header, host string) func(string) []string {
	return func(name string) []string { return headerOrHost(h, host, name) }
}
