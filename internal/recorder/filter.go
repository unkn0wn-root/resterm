package recorder

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/mock"
)

// A rule is "[METHOD ]path" with the path in mock route syntax.
type rule struct {
	method string
	path   *mock.PathMatcher
}

type filter struct {
	skip []rule
	only []rule
}

func newFilter(c Config) (filter, error) {
	skip, err := parseRules(c.Skip)
	if err != nil {
		return filter{}, err
	}
	only, err := parseRules(c.Only)
	if err != nil {
		return filter{}, err
	}
	return filter{skip: skip, only: only}, nil
}

func parseRules(raws []string) ([]rule, error) {
	rules := make([]rule, 0, len(raws))
	for _, raw := range raws {
		r, err := parseRule(raw)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

func parseRule(raw string) (rule, error) {
	method, path, ok := strings.Cut(strings.TrimSpace(raw), " ")
	if !ok {
		method, path = "", method
	}
	method = strings.ToUpper(method)
	if method != "" && !httpguts.ValidHeaderFieldName(method) {
		return rule{}, fmt.Errorf("capture rule %q: invalid method", raw)
	}
	m, err := mock.NewPathMatcher(strings.TrimSpace(path))
	if err != nil {
		return rule{}, fmt.Errorf("capture rule %q: %w", raw, err)
	}
	return rule{method: method, path: m}, nil
}

func (f filter) captures(req *http.Request) bool {
	match := func(r rule) bool { return r.matches(req) }
	if len(f.only) > 0 && !slices.ContainsFunc(f.only, match) {
		return false
	}
	return !slices.ContainsFunc(f.skip, match)
}

func (r rule) matches(req *http.Request) bool {
	return (r.method == "" || r.method == req.Method) && r.path.Matches(req.URL.Path, req.URL.RawPath)
}
