package mock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const maxMockRequestBody = 4 << 20

// matcher is one compiled @match condition, run per request against a probe.
type matcher func(*probe) (bool, *problem)

// newMatchers validates and compiles the whole @match block. Runtime order is
// load-bearing: JSON body errors surface only after query and header conditions
// pass.
func newMatchers(m restfile.MockMatch) ([]matcher, error) {
	query, err := compileQueryRules(m.Query)
	if err != nil {
		return nil, err
	}
	headers, err := compileMatchHeaders(m.Headers)
	if err != nil {
		return nil, err
	}

	body, err := compileJSONBody(m.JSON, m.JSONRules)
	if err != nil {
		return nil, err
	}

	var ms []matcher
	if len(query) > 0 {
		ms = append(ms, queryMatcher(query))
	}
	if len(headers) > 0 {
		ms = append(ms, headerMatcher(headers))
	}
	if body != nil {
		ms = append(ms, jsonMatcher(body))
	}
	return ms, nil
}

func compileMatchHeaders(src map[string]restfile.MockHeaderRule) (headerRules, error) {
	return compileRules("header", src, canonMatchHeaderName, compileHeaderRule)
}

// Selector headers choose the mock variant before match rules run. Accepting
// one here would create a condition that can never select another variant, so
// check the canonical name and reject it during compilation.
func canonMatchHeaderName(key string) (string, error) {
	name, err := canonHeaderName(key)
	if err != nil {
		return "", err
	}
	if isSelectorHeader(name) {
		return "", fmt.Errorf("mock selector header %q cannot be used as a matcher", name)
	}
	return name, nil
}

func queryMatcher(rules queryRules) matcher {
	return func(p *probe) (bool, *problem) {
		return rules.matches(queryLookup(p.query())), nil
	}
}

func headerMatcher(rules headerRules) matcher {
	return func(p *probe) (bool, *problem) {
		return rules.matches(headerLookup(p.r.Header, p.r.Host)), nil
	}
}

func jsonMatcher(want jsonPredicate) matcher {
	return func(p *probe) (bool, *problem) {
		body, ok, err := p.json()
		if err != nil {
			return false, err
		}
		return ok && want(body), nil
	}
}

func (v *variant) matches(p *probe) (bool, *problem) {
	for _, m := range v.matchers {
		ok, err := m(p)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// probe carries per-request state shared by every variant of a route. The body
// can only be read once, so json caches its result and error for later variants.
type probe struct {
	r      *http.Request
	q      url.Values
	loaded bool
	body   any
	ok     bool
	err    *problem
}

func (p *probe) query() url.Values {
	if p.q == nil {
		p.q = p.r.URL.Query()
	}
	return p.q
}

func isJSONMediaType(s string) bool {
	mt, _, err := mime.ParseMediaType(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

func (p *probe) json() (any, bool, *problem) {
	if p.loaded {
		return p.body, p.ok, p.err
	}
	p.loaded = true

	if !isJSONMediaType(p.r.Header.Get("Content-Type")) {
		return nil, false, nil
	}

	rd := io.Reader(http.NoBody)
	if p.r.Body != nil {
		rd = p.r.Body
	}
	data, err := io.ReadAll(io.LimitReader(rd, maxMockRequestBody+1))
	if err != nil {
		p.err = &problem{
			status: http.StatusBadRequest,
			detail: "read JSON request body: " + err.Error(),
		}
		return nil, false, p.err
	}
	if len(data) > maxMockRequestBody {
		p.err = &problem{
			status: http.StatusRequestEntityTooLarge,
			detail: "JSON request body exceeds 4 MiB limit",
		}
		return nil, false, p.err
	}
	p.body, err = decodeJSON(data)
	if err != nil {
		p.err = &problem{
			status: http.StatusBadRequest,
			detail: "invalid JSON request body: " + err.Error(),
		}
		return nil, false, p.err
	}
	p.ok = true
	return p.body, true, nil
}

func decodeJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var v any
	if err := decoder.Decode(&v); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return v, nil
}
