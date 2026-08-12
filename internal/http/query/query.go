package query

import (
	"maps"
	"net/url"
	"slices"
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
	return Values(vals), nil
}

// FromURL parses the query component of raw. It does not trim or otherwise
// repair the URL.
func FromURL(raw string) (Values, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	// Not u.Query(): that discards pairs it cannot decode, so a malformed
	// escape would return a shorter query instead of saying what is wrong.
	vals, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, err
	}
	return Values(vals), nil
}

// Clone returns an independent copy of a query multimap.
func Clone(src map[string][]string) Values {
	out := Values(maps.Clone(src))
	for name, vals := range src {
		out[name] = slices.Clone(vals)
	}
	return out
}

// Encode returns the standard URL encoding of vals.
func Encode(vals Values) string {
	return url.Values(vals).Encode()
}
