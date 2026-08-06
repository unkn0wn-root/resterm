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
	Method  string                             `json:"method,omitempty"`
	Path    string                             `json:"path,omitempty"`
	Query   map[string]restfile.StringList     `json:"query,omitempty"`
	Headers map[string]restfile.MockHeaderRule `json:"headers,omitempty"`
	JSON    json.RawMessage                    `json:"json,omitempty"`
}

type compiledPattern struct {
	pattern RequestPattern
	path    *pathMatcher
	headers headerRules
	json    any
	hasJSON bool
}

type pathMatcher struct {
	mux *http.ServeMux
}

func compileRequestPattern(p RequestPattern) (*compiledPattern, error) {
	p, headers, err := normalizeRequestPattern(p)
	if err != nil {
		return nil, err
	}
	cp := &compiledPattern{pattern: p, headers: headers}
	if p.Path != "" {
		cp.path, err = newPathMatcher(p.Path)
		if err != nil {
			return nil, err
		}
	}
	if len(p.JSON) > 0 {
		cp.hasJSON = true
		cp.json, err = decodeJSON(p.JSON)
		if err != nil {
			return nil, fmt.Errorf("invalid request pattern JSON: %w", err)
		}
	}
	return cp, nil
}

func normalizeRequestPattern(p RequestPattern) (RequestPattern, headerRules, error) {
	q := make(map[string]restfile.StringList, len(p.Query))
	for k, vs := range p.Query {
		q[k] = slices.Clone(vs)
	}
	out := RequestPattern{
		Method: strings.ToUpper(strings.TrimSpace(p.Method)),
		Path:   strings.TrimSpace(p.Path),
		Query:  q,
		JSON:   slices.Clone(p.JSON),
	}
	if out.Method != "" && !httpguts.ValidHeaderFieldName(out.Method) {
		return RequestPattern{}, nil, fmt.Errorf("invalid request pattern method %q", p.Method)
	}
	if out.Path != "" {
		if err := restfile.ValidateMockPath(out.Path); err != nil {
			return RequestPattern{}, nil, err
		}
	}
	if err := checkQueryRules(out.Query); err != nil {
		return RequestPattern{}, nil, err
	}
	headers, err := compileHeaderRules(p.Headers)
	if err != nil {
		return RequestPattern{}, nil, err
	}
	out.Headers = headers.declared()
	return out, headers, nil
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
	if !matchQuery(entry.query, p.pattern.Query) {
		return false, nil
	}
	if !p.headers.matches(entry.headers, entry.host) {
		return false, nil
	}
	if !p.hasJSON {
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
	return subset(p.json, body), nil
}
