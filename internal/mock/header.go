package mock

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func compileHeaderRules(src map[string]restfile.MockHeaderRule) (headerRules, error) {
	return compileRules("header", src, canonHeaderName, compileHeaderRule)
}

func canonHeaderName(key string) (string, error) {
	name := strings.TrimSpace(key)
	if !httpguts.ValidHeaderFieldName(name) {
		return "", fmt.Errorf("invalid mock header matcher %q", name)
	}
	return http.CanonicalHeaderKey(name), nil
}

func compileHeaderRule(r restfile.MockHeaderRule) (restfile.MockHeaderRule, valuePredicate, error) {
	r.Values = slices.Clone(r.Values)
	if err := r.Check(); err != nil {
		return r, nil, err
	}
	match, err := compileRule(r.Op, r.Values)
	return r, match, err
}

// net/http strips Host out of the header map, so every header lookup needs the
// same special case.
func headerValues(r *http.Request, name string) []string {
	return headerOrHost(r.Header, r.Host, name)
}

func headerOrHost(h http.Header, host, name string) []string {
	if strings.EqualFold(name, "Host") {
		if host == "" {
			return nil
		}
		return []string{host}
	}
	return h.Values(name)
}
