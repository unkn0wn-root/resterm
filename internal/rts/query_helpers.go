package rts

import (
	"net/url"
	"strings"
)

func parseQuery(txt string) (url.Values, error) {
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

func parseURLQuery(raw string) url.Values {
	u := strings.TrimSpace(raw)
	if u == "" {
		return url.Values{}
	}
	if !hasURLMarkers(u) {
		return url.Values{}
	}

	vals, err := parseQuery(u)
	if err != nil {
		return url.Values{}
	}
	return vals
}

func hasURLMarkers(s string) bool {
	return strings.Contains(s, "?") || strings.Contains(s, "://")
}

func valuesDict(vals url.Values) map[string]Value {
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
