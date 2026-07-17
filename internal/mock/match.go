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

func queryMatcher(want map[string][]string) matcher {
	return func(p *probe) (bool, *problem) {
		q := p.query()
		for k, vals := range want {
			if got, ok := q[k]; !ok || !slices.Equal(got, vals) {
				return false, nil
			}
		}
		return true, nil
	}
}

func headerMatcher(want map[string][]string) matcher {
	return func(p *probe) (bool, *problem) {
		for k, vals := range want {
			var got []string
			if strings.EqualFold(k, "Host") {
				if p.r.Host != "" {
					got = []string{p.r.Host}
				}
			} else {
				got = p.r.Header.Values(k)
			}
			if got == nil || !slices.Equal(got, vals) {
				return false, nil
			}
		}
		return true, nil
	}
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

func (p *probe) json() (any, bool, *problem) {
	if p.loaded {
		return p.body, p.ok, p.err
	}
	p.loaded = true

	mt, _, err := mime.ParseMediaType(strings.TrimSpace(p.r.Header.Get("Content-Type")))
	if err != nil || mt != "application/json" && !strings.HasSuffix(mt, "+json") {
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

// equalJSONNumbers compares two JSON numbers by value so 100, 1e2 and 100.0 all
// match. The int64 path keeps integers exact past float64's 2^53; the float64
// fallback handles the rest and, unlike big.Rat, cannot be made to allocate
// 10^exp by a hostile request - ParseFloat returns +Inf for a runaway exponent.
func equalJSONNumbers(want, got json.Number) bool {
	if want == got {
		return true
	}
	if a, err := want.Int64(); err == nil {
		if b, err := got.Int64(); err == nil {
			return a == b
		}
	}
	a, aerr := want.Float64()
	b, berr := got.Float64()
	return aerr == nil && berr == nil && a == b
}
