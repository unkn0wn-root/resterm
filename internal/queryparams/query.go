// Package queryparams defines parsing for raw query strings and URLs.
package queryparams

import (
	"net/url"
	"strings"
)

// Values is a query multimap. Every key maps to a list, including keys with
// zero or one value.
type Values map[string][]string

// Parse parses raw query text. One leading question mark is syntax and is
// removed; all other bytes, including whitespace, are data.
func Parse(raw string) (Values, error) {
	q := strings.TrimPrefix(raw, "?")
	vals, err := url.ParseQuery(q)
	if err != nil {
		return nil, err
	}
	return Clone(vals), nil
}

// FromURL parses the query component of raw. It does not trim or otherwise
// repair the URL.
func FromURL(raw string) (Values, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return Clone(u.Query()), nil
}

// Clone returns an independent copy of a query multimap.
func Clone(src map[string][]string) Values {
	out := make(Values, len(src))
	for name, vals := range src {
		out[name] = append([]string(nil), vals...)
	}
	return out
}

// Encode returns the standard URL encoding of vals.
func Encode(vals Values) string {
	return url.Values(vals).Encode()
}
