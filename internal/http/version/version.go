package version

import (
	"fmt"
	"regexp"
	"strings"
)

const Key = "http-version"

const tokenPrefix = "http/"

// HTTP/2 is accepted alongside the RFC's dotted version form. Other tails are
// URL text unless they match this numeric grammar.
var tokenRe = regexp.MustCompile(`(?i)^http/[0-9]+(\.[0-9]+)?$`)

type HTTP int

// HTTP/1.0 is omitted because net/http cannot send HTTP/1.0 requests.
const (
	Unknown HTTP = iota
	V11
	V2
)

func ParseToken(raw string) (HTTP, bool) {
	return parse(raw, false)
}

func ParseValue(raw string) (HTTP, bool) {
	return parse(raw, true)
}

// UnsupportedError reports an unsupported HTTP version token.
type UnsupportedError struct {
	Token string
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf(
		"unsupported HTTP version %q (use HTTP/1.1 or HTTP/2)",
		e.Token,
	)
}

// SplitToken removes a supported trailing HTTP version.
// Unsupported version tokens return an error.
func SplitToken(fields []string) ([]string, HTTP, error) {
	if len(fields) == 0 {
		return fields, Unknown, nil
	}
	last := fields[len(fields)-1]
	if !isTokenForm(last) {
		return fields, Unknown, nil
	}
	v, ok := ParseToken(last)
	if !ok {
		return fields, Unknown, &UnsupportedError{Token: last}
	}
	return fields[:len(fields)-1], v, nil
}

func isTokenForm(raw string) bool {
	return tokenRe.MatchString(raw)
}

func Format(v HTTP) string {
	switch v {
	case V11:
		return "1.1"
	case V2:
		return "2"
	default:
		return ""
	}
}

func SetIfMissing(m map[string]string, v HTTP) map[string]string {
	if v == Unknown {
		return m
	}
	if m == nil {
		m = make(map[string]string)
	}
	if hasKey(m, Key) {
		return m
	}
	if val := Format(v); val != "" {
		m[Key] = val
	}
	return m
}

func parse(raw string, allowBare bool) (HTTP, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Unknown, false
	}
	s = strings.ToLower(s)
	if after, ok := strings.CutPrefix(s, tokenPrefix); ok {
		s = after
	} else if !allowBare {
		return Unknown, false
	}
	switch s {
	case "1.1":
		return V11, true
	case "2", "2.0":
		return V2, true
	default:
		return Unknown, false
	}
}

func hasKey(m map[string]string, key string) bool {
	for k := range m {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}
