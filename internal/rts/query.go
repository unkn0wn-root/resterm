package rts

import (
	"net/url"
	"strings"
)

// ParseQuery parses either a raw query string or a URL containing a query.
func ParseQuery(txt string) (url.Values, error) {
	if hasURLMarkers(txt) {
		u, err := url.Parse(txt)
		if err != nil {
			return nil, err
		}
		return u.Query(), nil
	}
	tr := strings.TrimPrefix(txt, "?")
	return url.ParseQuery(tr)
}

// ParseURLQuery returns the parsed query for URL-like strings.
func ParseURLQuery(raw string) url.Values {
	u := strings.TrimSpace(raw)
	if u == "" {
		return url.Values{}
	}
	if !hasURLMarkers(u) {
		return url.Values{}
	}

	vals, err := ParseQuery(u)
	if err != nil {
		return url.Values{}
	}
	return vals
}

func hasURLMarkers(s string) bool {
	return strings.Contains(s, "?") || strings.Contains(s, "://")
}

// ValuesDict converts URL query values into an RTS dictionary.
func ValuesDict(vals url.Values) map[string]Value {
	if len(vals) == 0 {
		return map[string]Value{}
	}
	out := make(map[string]Value, len(vals))
	for k, v := range vals {
		if len(v) == 0 {
			out[k] = Str("")
			continue
		}
		if len(v) == 1 {
			out[k] = Str(v[0])
			continue
		}
		items := make([]Value, len(v))
		for i, it := range v {
			items[i] = Str(it)
		}
		out[k] = List(items)
	}
	return out
}
