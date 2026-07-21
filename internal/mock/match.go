package mock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const maxMockRequestBody = 4 << 20

// matcher is one compiled @match condition, run per request against a probe.
type matcher func(*probe) (bool, *problem)

// order is load-bearing: JSON body errors surface only after query and header conditions pass
func newMatchers(m restfile.MockMatch) ([]matcher, error) {
	var ms []matcher
	if len(m.Query) > 0 {
		ms = append(ms, queryMatcher(m.Query))
	}
	if len(m.Headers) > 0 {
		ms = append(ms, headerMatcher(m.Headers))
	}
	if len(m.JSON) > 0 {
		want, err := decodeJSON(m.JSON)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON matcher: %w", err)
		}
		ms = append(ms, jsonMatcher(want))
	}
	return ms, nil
}

func queryMatcher(want map[string]restfile.StringList) matcher {
	return func(p *probe) (bool, *problem) {
		return matchQuery(p.query(), want), nil
	}
}

func matchQuery(got url.Values, want map[string]restfile.StringList) bool {
	for k, vals := range want {
		if g, ok := got[k]; !ok || !slices.Equal(g, []string(vals)) {
			return false
		}
	}
	return true
}

func headerMatcher(want map[string]restfile.MockHeaderRule) matcher {
	return func(p *probe) (bool, *problem) {
		for k, rule := range want {
			got := headerValues(p.r, k)
			if !matchHeaderRule(got, rule) {
				return false, nil
			}
		}
		return true, nil
	}
}

func matchHeaderRule(got []string, rule restfile.MockHeaderRule) bool {
	switch rule.Op {
	case restfile.MockHeaderOpExact:
		return got != nil && slices.Equal(got, rule.Values)
	case restfile.MockHeaderOpPrefix:
		if len(rule.Values) != 1 {
			return false
		}
		for _, value := range got {
			if strings.HasPrefix(value, rule.Values[0]) {
				return true
			}
		}
		return false
	case restfile.MockHeaderOpPresent:
		return len(got) > 0
	case restfile.MockHeaderOpAbsent:
		return len(got) == 0
	default:
		return false
	}
}

// headerValues reads a request header for mock config. net/http strips Host
// out of the header map, so every header lookup needs the same special case.
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

func jsonMatcher(want any) matcher {
	return func(p *probe) (bool, *problem) {
		body, ok, err := p.json()
		if err != nil {
			return false, err
		}
		return ok && subset(want, body), nil
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

func subset(want, got any) bool {
	switch want := want.(type) {
	case map[string]any:
		got, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range want {
			if g, ok := got[k]; !ok || !subset(v, g) {
				return false
			}
		}
		return true
	case []any:
		got, ok := got.([]any)
		if !ok || len(want) != len(got) {
			return false
		}
		for i := range want {
			if !subset(want[i], got[i]) {
				return false
			}
		}
		return true
	case json.Number:
		got, ok := got.(json.Number)
		return ok && equalJSONNumbers(want, got)
	default:
		return want == got
	}
}

// equalJSONNumbers compares two JSON numbers by value so 100, 1e2 and 100.0
// all match. 256 bits keep mixed int/decimal forms exact far past float64's
// 2^53. Unlike big.Rat, a hostile request cannot make this allocate 10^exp
// bytes, because a runaway exponent just saturates to Inf and Inf never
// matches anything.
func equalJSONNumbers(want, got json.Number) bool {
	if want == got {
		return true
	}
	a, _, aerr := big.ParseFloat(string(want), 10, 256, big.ToNearestEven)
	b, _, berr := big.ParseFloat(string(got), 10, 256, big.ToNearestEven)
	return aerr == nil && berr == nil && !a.IsInf() && !b.IsInf() && a.Cmp(b) == 0
}
