package mock

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type RequestPattern struct {
	Method  string                             `json:"method,omitempty"`
	Path    string                             `json:"path,omitempty"`
	Query   map[string]restfile.StringList     `json:"query,omitempty"`
	Headers map[string]restfile.MockHeaderRule `json:"headers,omitempty"`
	JSON    json.RawMessage                    `json:"json,omitempty"`
}

type compiledPattern struct {
	pattern RequestPattern
	path    *pathMatcher
	json    any
	hasJSON bool
}

type pathMatcher struct {
	mux *http.ServeMux
}

func compileRequestPattern(pattern RequestPattern) (*compiledPattern, error) {
	normalized, err := normalizeRequestPattern(pattern)
	if err != nil {
		return nil, err
	}
	compiled := &compiledPattern{pattern: normalized}
	if normalized.Path != "" {
		compiled.path, err = newPathMatcher(normalized.Path)
		if err != nil {
			return nil, err
		}
	}
	if len(normalized.JSON) > 0 {
		compiled.hasJSON = true
		compiled.json, err = decodeJSON(normalized.JSON)
		if err != nil {
			return nil, fmt.Errorf("invalid request pattern JSON: %w", err)
		}
	}
	return compiled, nil
}

func normalizeRequestPattern(pattern RequestPattern) (RequestPattern, error) {
	out := RequestPattern{
		Method: strings.ToUpper(strings.TrimSpace(pattern.Method)),
		Path:   strings.TrimSpace(pattern.Path),
		Query:  cloneValues(pattern.Query),
		JSON:   slices.Clone(pattern.JSON),
	}
	if out.Method != "" && !httpguts.ValidHeaderFieldName(out.Method) {
		return RequestPattern{}, fmt.Errorf("invalid request pattern method %q", pattern.Method)
	}
	if out.Path != "" {
		if err := restfile.ValidateMockPath(out.Path); err != nil {
			return RequestPattern{}, err
		}
	}
	if err := checkQueryRules(out.Query); err != nil {
		return RequestPattern{}, err
	}
	headers, err := canonHeaderRules(pattern.Headers)
	if err != nil {
		return RequestPattern{}, err
	}
	out.Headers = headers
	return out, nil
}

func checkQueryRules(query map[string]restfile.StringList) error {
	for name, values := range query {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("mock query matcher name cannot be empty")
		}
		if values == nil {
			return fmt.Errorf("mock query matcher %q cannot be null", name)
		}
	}
	return nil
}

// canonHeaderRules validates every rule and rekeys the map by canonical header name.
func canonHeaderRules(src map[string]restfile.MockHeaderRule) (map[string]restfile.MockHeaderRule, error) {
	out := make(map[string]restfile.MockHeaderRule, len(src))
	for name, rule := range src {
		name = strings.TrimSpace(name)
		if !httpguts.ValidHeaderFieldName(name) {
			return nil, fmt.Errorf("invalid mock header matcher %q", name)
		}
		if err := validateHeaderRule(name, rule); err != nil {
			return nil, err
		}
		rule.Values = slices.Clone(rule.Values)
		canonical := http.CanonicalHeaderKey(name)
		if _, exists := out[canonical]; exists {
			return nil, fmt.Errorf("mock header matcher %q is repeated with different casing", canonical)
		}
		out[canonical] = rule
	}
	return out, nil
}

func validateHeaderRule(name string, rule restfile.MockHeaderRule) error {
	for _, value := range rule.Values {
		if !httpguts.ValidHeaderFieldValue(value) {
			return fmt.Errorf("invalid value for mock header matcher %q", name)
		}
	}
	switch rule.Op {
	case restfile.MockHeaderOpExact:
		if len(rule.Values) == 0 {
			return fmt.Errorf("mock header exact matcher %q requires at least one value", name)
		}
		return nil
	case restfile.MockHeaderOpPrefix:
		if len(rule.Values) != 1 || rule.Values[0] == "" {
			return fmt.Errorf("mock header prefix matcher %q requires one non-empty value", name)
		}
		return nil
	case restfile.MockHeaderOpPresent, restfile.MockHeaderOpAbsent:
		if len(rule.Values) != 0 {
			return fmt.Errorf("mock header presence matcher %q cannot have values", name)
		}
		return nil
	default:
		return fmt.Errorf("mock header matcher %q has an invalid operation", name)
	}
}

func newPathMatcher(path string) (*pathMatcher, error) {
	pattern, _, err := restfile.CompileMockPath(path)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
	return &pathMatcher{mux: mux}, nil
}

func (m *pathMatcher) matches(path, rawPath string) bool {
	if m == nil || m.mux == nil {
		return true
	}
	req := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Path: path, RawPath: rawPath},
	}
	escaped := req.URL.EscapedPath()
	if !cleanPath(escaped) {
		return false
	}
	_, pattern := m.mux.Handler(req)
	return pattern != "" && !missingRouteSlash(pattern, escaped)
}

func (p *compiledPattern) matches(entry requestRecord) (bool, error) {
	if p.pattern.Method != "" && entry.method != p.pattern.Method {
		return false, nil
	}
	if p.path != nil && !p.path.matches(entry.path, entry.rawPath) {
		return false, nil
	}
	for name, values := range p.pattern.Query {
		got, ok := entry.query[name]
		if !ok || !slices.Equal(got, []string(values)) {
			return false, nil
		}
	}
	for name, rule := range p.pattern.Headers {
		if !matchHeaderRule(entry.headerValues(name), rule) {
			return false, nil
		}
	}
	if !p.hasJSON {
		return true, nil
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(entry.headers.Get("Content-Type")))
	if err != nil || mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return false, nil
	}
	if entry.bodyTruncated {
		return false, &IncompleteError{Reason: "request body was truncated"}
	}
	body, err := decodeJSON(entry.body)
	if err != nil {
		return false, nil
	}
	return subset(p.json, body), nil
}

func cloneValues[M ~map[string]S, S ~[]string](src M) M {
	if src == nil {
		return make(M)
	}
	out := make(M, len(src))
	for name, values := range src {
		out[name] = slices.Clone(values)
	}
	return out
}
