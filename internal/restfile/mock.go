package restfile

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
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
