package restfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/net/http/httpguts"
)

var mockNameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// MockSequenceDelimiter separates the responses of a mock sequence.
const MockSequenceDelimiter = "---"

func IsMockSequenceDelimiter(line string) bool {
	return strings.TrimSpace(line) == MockSequenceDelimiter
}

// CheckShape validates the structural invariants shared by the compiler and the
// writer: a scenario is either named or a sequence, and its response count fits
// its kind.
func (m *Mock) CheckShape() error {
	switch {
	case m.Name != "" && m.Sequence != "":
		return errors.New("mock name and sequence cannot be combined")
	case !m.SequenceKey.IsZero() && m.SequenceKey.String() == "":
		return errors.New("mock sequence key is invalid")
	case !m.SequenceKey.IsZero() && m.Sequence == "":
		return errors.New("mock sequence key requires a sequence")
	case m.Sequence == "" && len(m.Responses) != 1:
		return errors.New("mock must define exactly one response")
	case m.Sequence != "" && len(m.Responses) < 2:
		return errors.New("mock sequence must define at least two responses")
	}
	return nil
}

func ValidMockName(name string) bool {
	return mockNameRE.MatchString(name)
}

func ValidMockStatus(status int) bool {
	return status >= 200 && status <= 599
}

func (k MockSequenceKey) String() string {
	var source string
	switch k.Source {
	case MockSequenceKeySourcePath:
		source = "path"
	case MockSequenceKeySourceQuery:
		source = "query"
	case MockSequenceKeySourceHeader:
		source = "header"
	case MockSequenceKeySourceCookie:
		source = "cookie"
	default:
		return ""
	}
	if k.Name == "" || k.Name != strings.TrimSpace(k.Name) {
		return ""
	}
	return source + "." + k.Name
}

// ParseMockSequenceKey parses the source.name form used by @mock sequence-key.
func ParseMockSequenceKey(raw string) (MockSequenceKey, error) {
	source, name, ok := strings.Cut(strings.TrimSpace(raw), ".")
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		return MockSequenceKey{}, errors.New(
			"must use path.<name>, query.<name>, header.<name>, or cookie.<name>",
		)
	}
	key := MockSequenceKey{Name: name}
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "path":
		key.Source = MockSequenceKeySourcePath
	case "query":
		key.Source = MockSequenceKeySourceQuery
	case "header":
		key.Source = MockSequenceKeySourceHeader
	case "cookie":
		key.Source = MockSequenceKeySourceCookie
	default:
		return MockSequenceKey{}, fmt.Errorf("source %q is not supported", source)
	}
	return key, nil
}

// Check validates the key and returns it with header names canonicalized.
func (k MockSequenceKey) Check(params map[string]string) (MockSequenceKey, error) {
	if k.IsZero() {
		return k, nil
	}
	if k.Name == "" || k.Name != strings.TrimSpace(k.Name) {
		return MockSequenceKey{}, errors.New("name cannot be empty")
	}
	switch k.Source {
	case MockSequenceKeySourcePath:
		if _, ok := params[k.Name]; !ok {
			return MockSequenceKey{}, fmt.Errorf("path wildcard %q is not declared", k.Name)
		}
	case MockSequenceKeySourceQuery:
	case MockSequenceKeySourceHeader:
		if !httpguts.ValidHeaderFieldName(k.Name) {
			return MockSequenceKey{}, fmt.Errorf("header name %q is invalid", k.Name)
		}
		k.Name = http.CanonicalHeaderKey(k.Name)
	case MockSequenceKeySourceCookie:
		if !httpguts.ValidHeaderFieldName(k.Name) {
			return MockSequenceKey{}, fmt.Errorf("cookie name %q is invalid", k.Name)
		}
	default:
		return MockSequenceKey{}, errors.New("source is invalid")
	}
	return k, nil
}

func (l *StringList) UnmarshalJSON(data []byte) error {
	var scalar string
	if err := json.Unmarshal(data, &scalar); err == nil && string(data) != "null" {
		*l = StringList{scalar}
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil || values == nil {
		return errors.New("expected a string or string array")
	}
	*l = values
	return nil
}

func (r MockHeaderRule) MarshalJSON() ([]byte, error) {
	switch r.Op {
	case MockHeaderOpExact:
		if len(r.Values) == 0 {
			return nil, errors.New("mock header exact matcher requires at least one value")
		}
		if len(r.Values) == 1 {
			return json.Marshal(r.Values[0])
		}
		return json.Marshal(r.Values)
	case MockHeaderOpPrefix:
		if len(r.Values) != 1 || r.Values[0] == "" {
			return nil, errors.New("mock header prefix requires one non-empty value")
		}
		return json.Marshal(map[string]string{"prefix": r.Values[0]})
	case MockHeaderOpPresent:
		return []byte(`{"present":true}`), nil
	case MockHeaderOpAbsent:
		return []byte(`{"absent":true}`), nil
	default:
		return nil, errors.New("mock header matcher has an invalid operation")
	}
}

func (r *MockHeaderRule) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return errors.New("mock header matcher cannot be null")
	}
	var exact StringList
	if exact.UnmarshalJSON(data) == nil {
		if len(exact) == 0 {
			return errors.New("mock header exact matcher requires at least one value")
		}
		*r = MockHeaderRule{Op: MockHeaderOpExact, Values: exact}
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("mock header matcher must be a string, string array, or object")
	}
	if len(fields) != 1 {
		return errors.New("mock header matcher must contain exactly one operator")
	}
	var op string
	var raw json.RawMessage
	for name, value := range fields {
		op, raw = name, value
	}
	switch op {
	case "exact":
		var values StringList
		if err := values.UnmarshalJSON(raw); err != nil || len(values) == 0 {
			return errors.New("mock header exact matcher requires a string or non-empty string array")
		}
		*r = MockHeaderRule{Op: MockHeaderOpExact, Values: values}
		return nil
	case "prefix":
		var prefix string
		if err := json.Unmarshal(raw, &prefix); err != nil || prefix == "" {
			return errors.New("mock header prefix matcher must be a non-empty string")
		}
		*r = MockHeaderRule{Op: MockHeaderOpPrefix, Values: []string{prefix}}
		return nil
	case "present", "absent":
		var enabled bool
		if err := json.Unmarshal(raw, &enabled); err != nil || !enabled {
			return fmt.Errorf("mock header %s matcher must be true", op)
		}
		ruleOp := MockHeaderOpPresent
		if op == "absent" {
			ruleOp = MockHeaderOpAbsent
		}
		*r = MockHeaderRule{Op: ruleOp}
		return nil
	default:
		return fmt.Errorf("unknown mock header matcher operator %q", op)
	}
}

func ResponseAllowsBody(status int) bool {
	switch status {
	case http.StatusNoContent, http.StatusResetContent, http.StatusNotModified:
		return false
	default:
		return true
	}
}

func IsManagedMockResponseHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "connection", "content-length", "keep-alive", "proxy-connection",
		"te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}

// HasTemplate reports whether a generated response needs literal template preservation.
func (r MockResponse) HasTemplate() bool {
	if strings.Contains(r.Body.Text, "{{") {
		return true
	}
	for _, values := range r.Headers {
		for _, value := range values {
			if strings.Contains(value, "{{") {
				return true
			}
		}
	}
	return false
}

func (m MockMatch) HasConditions() bool {
	return len(m.Query) > 0 || len(m.Headers) > 0 || len(m.JSON) > 0
}

func ValidateMockPath(path string) error {
	_, _, err := CompileMockPath(path)
	return err
}

// CompileMockPath converts a mock route into a ServeMux pattern and maps each
// source wildcard name to the positional wildcard used by that pattern.
func CompileMockPath(path string) (string, map[string]string, error) {
	if err := validateMockPathForm(path); err != nil {
		return "", nil, err
	}
	if path == "/" {
		return "/{$}", map[string]string{}, nil
	}

	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	params := make(map[string]string)
	trailing := parts[len(parts)-1] == ""
	if trailing {
		parts = parts[:len(parts)-1]
	}
	for i, part := range parts {
		if !strings.ContainsAny(part, "{}") {
			seg, err := canonLiteral(part)
			if err != nil {
				return "", nil, err
			}
			parts[i] = seg
			continue
		}
		name, catchAll, err := parseWildcard(part)
		if err != nil {
			return "", nil, err
		}
		if _, exists := params[name]; exists {
			return "", nil, fmt.Errorf("mock path wildcard name %q is repeated", name)
		}
		if catchAll && i != len(parts)-1 {
			return "", nil, fmt.Errorf("catch-all wildcard must be the final path segment")
		}
		if catchAll && trailing {
			return "", nil, fmt.Errorf("catch-all wildcard cannot be followed by a trailing slash")
		}
		positional := fmt.Sprintf("p%d", i)
		params[name] = positional
		if catchAll {
			parts[i] = "{" + positional + "...}"
		} else {
			parts[i] = "{" + positional + "}"
		}
	}

	pattern := "/" + strings.Join(parts, "/")
	if trailing {
		pattern += "/{$}"
	}
	return pattern, params, nil
}

func validateMockPathForm(path string) error {
	if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#") {
		return fmt.Errorf("mock path must be an origin-form path without query or fragment")
	}
	if strings.Contains(path, "//") {
		return fmt.Errorf("mock path cannot contain repeated slashes")
	}
	if strings.IndexFunc(path, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return fmt.Errorf("mock path must escape whitespace and control characters")
	}
	return nil
}

func parseWildcard(part string) (string, bool, error) {
	name, open := strings.CutPrefix(part, "{")
	name, closed := strings.CutSuffix(name, "}")
	if !open || !closed || strings.ContainsAny(name, "{}") {
		return "", false, fmt.Errorf("wildcards must occupy a complete path segment")
	}
	catchAll := strings.HasSuffix(name, "...")
	name = strings.TrimSuffix(name, "...")
	if name == "" || name == "$" || name != strings.TrimSpace(name) {
		return "", false, fmt.Errorf("mock path wildcard name is missing or invalid")
	}
	return name, catchAll, nil
}

func canonLiteral(part string) (string, error) {
	seg, err := url.PathUnescape(part)
	if err != nil {
		return "", fmt.Errorf("invalid path escape in segment %q", part)
	}
	if seg == "." || seg == ".." {
		return "", fmt.Errorf("mock path cannot contain dot segments")
	}
	return url.PathEscape(seg), nil
}
