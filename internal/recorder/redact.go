package recorder

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/http/header"
)

type direction uint8

const (
	directionRequest direction = iota
	directionResponse
)

// Redaction uses header and field names. Secrets in free text or under
// unrecognized names are left unchanged.
type redactor struct {
	policy policy
	count  int
}

func (p policy) sanitize(e *Entry, limit int64) {
	request, response := decodeBody(e.Request, limit), decodeBody(e.Response, limit)

	r := &redactor{policy: p}
	clean, ok := r.url(e.URL)
	if !ok {
		e.Request.Issue, e.Response.Issue = issueURL, issueURL
		request.issue, response.issue = issueURL, issueURL
	}
	e.URL = clean
	urlRedactions := r.count

	r.apply(&e.Request, request, directionRequest, limit)
	e.Request.Redactions += urlRedactions
	r.apply(&e.Response, response, directionResponse, limit)
}

func (r *redactor) apply(m *Message, b body, dir direction, limit int64) {
	before := r.count
	m.Headers = r.headers(m.Headers)
	m.Issue, m.Body, m.MatchJSON = b.issue, nil, nil
	if b.ok() {
		m.Body, m.MatchJSON = r.body(b, dir)
	}
	m.Redactions = r.count - before

	if int64(len(m.Body)) > limit {
		m.Body, m.MatchJSON = nil, nil
		m.Issue = issueRedactedLimit
	}
	if b.decoded {
		m.Headers.Del("Content-Encoding")
	}
	if b.decoded || !bytes.Equal(m.Body, b.data) || !m.Issue.ok() {
		for _, name := range digestHeaders {
			m.Headers.Del(name)
		}
	}
}

// Keep the original bytes when nothing was redacted, preserving formatting.
func (r *redactor) body(b body, dir direction) (data, match []byte) {
	before := r.count
	switch b.kind {
	case bodyJSON:
		clean, subset, _ := r.json(b.value)
		if r.count == before {
			data = b.data
		} else {
			data, _ = json.Marshal(clean)
		}
		if dir == directionRequest && keepSubset(b.value, subset) {
			match, _ = json.Marshal(subset)
		}
	case bodyForm:
		values := b.value.(url.Values)
		r.form(values)
		if r.count == before {
			data = b.data
		} else {
			data = []byte(values.Encode())
		}
	default:
		data = b.data
	}
	return data, match
}

// Exclude redacted fields from matchers. Exclude changed arrays entirely
// because the mock engine cannot match part of an array.
func (r *redactor) json(v any) (clean, match any, changed bool) {
	switch v := v.(type) {
	case map[string]any:
		out, subset := make(map[string]any, len(v)), make(map[string]any, len(v))
		for name, value := range v {
			if r.policy.secretField(name) {
				out[name] = redacted
				r.count++
				changed = true
				continue
			}
			field, pattern, altered := r.json(value)
			out[name] = field
			if keepSubset(value, pattern) {
				subset[name] = pattern
			}
			changed = changed || altered
		}
		return out, subset, changed
	case []any:
		out := make([]any, len(v))
		for i, value := range v {
			element, _, altered := r.json(value)
			out[i] = element
			changed = changed || altered
		}
		if changed {
			return out, nil, true
		}
		return out, out, false
	default:
		return v, v, false
	}
}

// A JSON null is a matcher the mock engine honours. A dropped subset is not.
func keepSubset(value, subset any) bool { return subset != nil || value == nil }

func (r *redactor) form(values url.Values) {
	for name, list := range values {
		if !r.policy.secretField(name) {
			continue
		}
		for i := range list {
			list[i] = redacted
			r.count++
		}
	}
}

func (r *redactor) url(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.User != nil {
		u.User = nil
		r.count++
	}

	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", false
	}
	// Only rebuild changed queries, since encoding can change their escapes.
	changed := false
	for name, values := range q {
		if !r.policy.secretField(name) {
			continue
		}
		for i := range values {
			values[i] = redacted
			r.count++
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String(), true
}

func (r *redactor) headers(src http.Header) http.Header {
	listed := connectionTokens(src)
	dst := make(http.Header, len(src))
	for name, values := range src {
		if hopHeaders.Has(name) || listed.Has(name) {
			continue
		}
		if cookieHeaders.Has(name) {
			r.count += len(values)
			continue
		}
		for _, value := range values {
			if v, ok := r.headerValue(name, value); ok {
				dst.Add(name, v)
			}
		}
	}
	return dst
}

func (r *redactor) headerValue(name, value string) (string, bool) {
	switch {
	case strings.ContainsAny(value, "\r\n"):
		return "", false
	case r.policy.secretHeader(name):
		r.count++
		return redacted, true
	case locationHeaders.Has(name):
		out, ok := r.url(value)
		if !ok {
			r.count++
			return "", false
		}
		return out, true
	default:
		return value, true
	}
}

func connectionTokens(src http.Header) header.Set {
	values := src.Values("Connection")
	if len(values) == 0 {
		return nil
	}
	tokens := make(header.Set)
	for _, value := range values {
		for name := range strings.SplitSeq(value, ",") {
			tokens.Add(name)
		}
	}
	return tokens
}
