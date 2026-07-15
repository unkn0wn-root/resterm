package restfile

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var mockNameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

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

func (m MockMatch) HasConditions() bool {
	return len(m.Query) > 0 || len(m.Headers) > 0 || len(m.JSON) > 0
}

func ValidateMockPath(path string) error {
	_, err := CompileMockPathPattern(path)
	return err
}

func CompileMockPathPattern(path string) (string, error) {
	if err := validateMockPathForm(path); err != nil {
		return "", err
	}
	if path == "/" {
		return "/{$}", nil
	}

	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	trailing := parts[len(parts)-1] == ""
	if trailing {
		parts = parts[:len(parts)-1]
	}
	for i, part := range parts {
		if !strings.ContainsAny(part, "{}") {
			seg, err := canonLiteral(part)
			if err != nil {
				return "", err
			}
			parts[i] = seg
			continue
		}
		catchAll, err := parseWildcard(part)
		if err != nil {
			return "", err
		}
		if catchAll && i != len(parts)-1 {
			return "", fmt.Errorf("catch-all wildcard must be the final path segment")
		}
		if catchAll && trailing {
			return "", fmt.Errorf("catch-all wildcard cannot be followed by a trailing slash")
		}
		// positional rename: prevents duplicate-name mux panics and lets
		// differently-named wildcards merge onto one route pattern
		if catchAll {
			parts[i] = fmt.Sprintf("{p%d...}", i)
		} else {
			parts[i] = fmt.Sprintf("{p%d}", i)
		}
	}

	pattern := "/" + strings.Join(parts, "/")
	if trailing {
		pattern += "/{$}"
	}
	return pattern, nil
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

// parseWildcard reports whether a {name} / {name...} segment is a catch-all.
// The name is checked for diagnostics only - the caller renames wildcards positionally.
func parseWildcard(part string) (bool, error) {
	name, open := strings.CutPrefix(part, "{")
	name, closed := strings.CutSuffix(name, "}")
	if !open || !closed || strings.ContainsAny(name, "{}") {
		return false, fmt.Errorf("wildcards must occupy a complete path segment")
	}
	catchAll := strings.HasSuffix(name, "...")
	name = strings.TrimSuffix(name, "...")
	if name == "" || name == "$" || name != strings.TrimSpace(name) {
		return false, fmt.Errorf("mock path wildcard name is missing or invalid")
	}
	return catchAll, nil
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
