package urltpl

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	tokenPrefix      = "rtpl_"
	tokenRandomBytes = 16
	tokenMaxAttempts = 8
)

// RawQuery returns the literal query component of a request target. Closed
// {{...}} templates are opaque, so delimiters inside them are not URL syntax.
func RawQuery(raw string) (string, bool) {
	q := outsideTemplate(raw, '?')
	f := outsideTemplate(raw, '#')
	if q < 0 || f >= 0 && f < q {
		return "", false
	}

	raw = raw[q+1:]
	if f = outsideTemplate(raw, '#'); f >= 0 {
		raw = raw[:f]
	}
	return raw, true
}

// ParseTargetQuery parses a request target without requiring the non-query
// portion to be a valid URL. Closed {{...}} templates remain opaque data.
func ParseTargetQuery(raw string) (map[string][]string, error) {
	q, ok := RawQuery(raw)
	if !ok {
		return map[string][]string{}, nil
	}

	state := newTemplateState(q)
	vals, err := url.ParseQuery(state.replace(q))
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(vals))
	for key, items := range vals {
		key = state.restore(key)
		for _, item := range items {
			out[key] = append(out[key], state.restore(item))
		}
	}
	return out, nil
}

func outsideTemplate(s string, sep byte) int {
	for off := 0; off < len(s); {
		i := strings.IndexByte(s[off:], sep)
		if i < 0 {
			return -1
		}
		open := strings.Index(s[off:], "{{")
		if open < 0 || i < open {
			return off + i
		}

		open += off
		close := strings.Index(s[open+2:], "}}")
		if close < 0 {
			return off + i
		}
		off = open + close + 4
	}
	return -1
}

func PatchQuery(raw string, patch map[string]*string) (string, error) {
	if len(patch) == 0 {
		return raw, nil
	}

	state := newTemplateState(collectSources(raw, patch, nil)...)
	raw = state.replace(raw)
	patch = state.replacePatch(patch)

	updated, err := patchQueryURL(raw, patch)
	if err != nil {
		return "", err
	}
	return state.restore(updated), nil
}

func MergeQuery(raw string, patch map[string][]string) (string, error) {
	if len(patch) == 0 {
		return raw, nil
	}

	state := newTemplateState(collectSources(raw, nil, patch)...)
	raw = state.replace(raw)
	patch = state.replaceMergePatch(patch)

	updated, err := mergeQueryURL(raw, patch)
	if err != nil {
		return "", err
	}
	return state.restore(updated), nil
}

// queryOf reads the query a rewrite is about to re-encode. url.URL.Query drops
// pairs it cannot decode, which would delete them from the URL the caller gets
// back, so a malformed escape stops the rewrite instead.
func queryOf(u *url.URL) (url.Values, error) {
	return url.ParseQuery(u.RawQuery)
}

func patchQueryURL(raw string, patch map[string]*string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	vals, err := queryOf(parsed)
	if err != nil {
		return "", err
	}
	for key, val := range patch {
		if val == nil {
			vals.Del(key)
			continue
		}
		vals.Set(key, *val)
	}
	parsed.RawQuery = vals.Encode()
	return parsed.String(), nil
}

func mergeQueryURL(raw string, patch map[string][]string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}

	vals, err := queryOf(parsed)
	if err != nil {
		return "", err
	}
	for key, items := range patch {
		if len(items) == 0 {
			vals.Del(key)
			continue
		}
		vals.Del(key)
		for _, item := range items {
			vals.Add(key, item)
		}
	}
	parsed.RawQuery = vals.Encode()
	return parsed.String(), nil
}

type templateState struct {
	sources []string
	tokens  map[string]string
	byValue map[string]string
	nextID  int
}

func newTemplateState(sources ...string) *templateState {
	return &templateState{
		sources: sources,
		tokens:  make(map[string]string),
		byValue: make(map[string]string),
	}
}

func (s *templateState) replace(input string) string {
	if !strings.Contains(input, "{{") {
		return input
	}

	var b strings.Builder
	for {
		start := strings.Index(input, "{{")
		if start == -1 {
			b.WriteString(input)
			break
		}

		end := strings.Index(input[start+2:], "}}")
		if end == -1 {
			b.WriteString(input)
			break
		}

		end += start + 2
		b.WriteString(input[:start])
		tpl := input[start : end+2]
		token, ok := s.byValue[tpl]
		if !ok {
			token = s.nextToken()
			s.tokens[token] = tpl
			s.byValue[tpl] = token
		}
		b.WriteString(token)
		input = input[end+2:]
	}
	return b.String()
}

func (s *templateState) replacePatch(patch map[string]*string) map[string]*string {
	if len(patch) == 0 {
		return patch
	}

	out := make(map[string]*string, len(patch))
	for key, val := range patch {
		rkey := s.replace(key)
		if val == nil {
			out[rkey] = nil
			continue
		}
		rval := s.replace(*val)
		out[rkey] = &rval
	}
	return out
}

func (s *templateState) replaceMergePatch(patch map[string][]string) map[string][]string {
	if len(patch) == 0 {
		return patch
	}

	out := make(map[string][]string, len(patch))
	for key, vals := range patch {
		rkey := s.replace(key)
		if len(vals) == 0 {
			out[rkey] = nil
			continue
		}

		outVals := make([]string, 0, len(vals))
		for _, val := range vals {
			outVals = append(outVals, s.replace(val))
		}
		out[rkey] = outVals
	}
	return out
}

func (s *templateState) restore(input string) string {
	if len(s.tokens) == 0 {
		return input
	}

	keys := make([]string, 0, len(s.tokens))
	for key := range s.tokens {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	out := input
	for _, key := range keys {
		out = strings.ReplaceAll(out, key, s.tokens[key])
	}
	return out
}

func (s *templateState) nextToken() string {
	for range tokenMaxAttempts {
		token, err := randomToken()
		if err == nil && s.tokenAvailable(token) {
			return token
		}
	}
	for {
		token := tokenPrefix + strconv.FormatInt(int64(s.nextID), 36) + "x"
		s.nextID++
		if s.tokenAvailable(token) {
			return token
		}
	}
}

func (s *templateState) tokenAvailable(token string) bool {
	if _, exists := s.tokens[token]; exists {
		return false
	}
	for _, src := range s.sources {
		if strings.Contains(src, token) {
			return false
		}
	}
	return true
}

func collectSources(raw string, patch map[string]*string, merge map[string][]string) []string {
	var sources []string
	if raw != "" {
		sources = append(sources, raw)
	}
	if len(patch) > 0 {
		for key, val := range patch {
			sources = append(sources, key)
			if val != nil {
				sources = append(sources, *val)
			}
		}
	}
	if len(merge) > 0 {
		for key, vals := range merge {
			sources = append(sources, key)
			sources = append(sources, vals...)
		}
	}
	return sources
}

func randomToken() (string, error) {
	buf := make([]byte, tokenRandomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return tokenPrefix + hex.EncodeToString(buf), nil
}
