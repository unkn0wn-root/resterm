package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type RequestPattern struct {
	Method    string                             `json:"method,omitempty"`
	Path      string                             `json:"path,omitempty"`
	Query     map[string]restfile.MockQueryRule  `json:"query,omitempty"`
	Headers   map[string]restfile.MockHeaderRule `json:"headers,omitempty"`
	JSON      json.RawMessage                    `json:"json,omitempty"`
	JSONRules json.RawMessage                    `json:"jsonRules,omitempty"`
}

func (p RequestPattern) Clone() RequestPattern {
	p.Query = restfile.CloneMockQuery(p.Query)
	p.Headers = restfile.CloneMockHeaders(p.Headers)
	p.JSON = slices.Clone(p.JSON)
	p.JSONRules = slices.Clone(p.JSONRules)
	return p
}

// compiledPattern holds the normalized pattern next to the predicates built from
// it. The pattern is what callers read back and what gets serialized, so the two
// have to travel together.
type compiledPattern struct {
	pattern RequestPattern
	path    *pathMatcher
	query   queryRules
	headers headerRules
	json    jsonPredicate
}

type pathMatcher struct {
	mux *http.ServeMux
}

func compileRequestPattern(p RequestPattern) (*compiledPattern, error) {
	cp := &compiledPattern{
		pattern: RequestPattern{
			Method:    strings.ToUpper(strings.TrimSpace(p.Method)),
			Path:      strings.TrimSpace(p.Path),
			JSON:      slices.Clone(p.JSON),
			JSONRules: slices.Clone(p.JSONRules),
		},
	}
	if cp.pattern.Method != "" && !httpguts.ValidHeaderFieldName(cp.pattern.Method) {
		return nil, fmt.Errorf("invalid request pattern method %q", p.Method)
	}

	var err error
	if cp.pattern.Path != "" {
		if cp.path, err = newPathMatcher(cp.pattern.Path); err != nil {
			return nil, err
		}
	}
	if cp.query, err = compileQueryRules(p.Query); err != nil {
		return nil, err
	}
	if cp.headers, err = compileHeaderRules(p.Headers); err != nil {
		return nil, err
	}
	if cp.json, err = compileJSONBody(cp.pattern.JSON, cp.pattern.JSONRules); err != nil {
		return nil, err
	}
	cp.pattern.Query = cp.query.declared()
	cp.pattern.Headers = cp.headers.declared()
	return cp, nil
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
	if !p.query.matches(queryLookup(entry.query)) {
		return false, nil
	}
	if !p.headers.matches(headerLookup(entry.headers, entry.host)) {
		return false, nil
	}
	if p.json == nil {
		return true, nil
	}
	if !isJSONMediaType(entry.headers.Get("Content-Type")) {
		return false, nil
	}
	if entry.bodyTruncated {
		return false, &IncompleteError{Reason: "request body was truncated"}
	}
	body, err := decodeJSON(entry.body)
	if err != nil {
		return false, nil
	}
	return p.json(body), nil
}
