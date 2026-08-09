package mock

import (
	"errors"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func compileQueryRules(src map[string]restfile.MockQueryRule) (queryRules, error) {
	return compileRules("query", src, canonQueryName, compileQueryRule)
}

// Query names are case sensitive, so unlike headers they are used as written.
func canonQueryName(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("mock query matcher name cannot be empty")
	}
	return name, nil
}

func compileQueryRule(r restfile.MockQueryRule) (restfile.MockQueryRule, valuePredicate, error) {
	r.Values = slices.Clone(r.Values)
	if err := r.Check(); err != nil {
		return r, nil, err
	}
	match, err := compileRule(r.Op, r.Values)
	return r, match, err
}
