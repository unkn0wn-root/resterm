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

// HasTemplate reports whether the string contains a template marker.
func HasTemplate(s string) bool {
	return strings.Contains(s, "{{")
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

func patchQueryURL(raw string, patch map[string]*string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}

	vals := parsed.Query()
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
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}

	vals := parsed.Query()
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
	nextID  int
}

func newTemplateState(sources ...string) *templateState {
	return &templateState{
		sources: sources,
		tokens:  make(map[string]string),
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
		token := s.nextToken()
		s.tokens[token] = tpl
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
