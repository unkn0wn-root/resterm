package bodyref

import (
	"strings"
	"unicode"
	"unicode/utf8"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

type Location int

const (
	// Line parses a standalone body reference line, e.g. "< ./payload.json".
	Line Location = iota
	// Inline parses a body reference embedded in an at-prefixed line, e.g. "@query < ./query.graphql".
	Inline
)

type Options struct {
	Location Location
	// ForceInline disables body file references for ambiguous literal bodies.
	ForceInline bool
}

// Parse returns the body file path from Resterm's "< path" body reference syntax.
// Options controls where to look for "<" and whether body file references are
// disabled for the current body.
func Parse(line string, opt Options) (string, bool) {
	if opt.ForceInline {
		return "", false
	}
	s := str.Trim(line)
	t, ok := tail(s, opt.Location)
	if !ok {
		return "", false
	}
	return parsePath(t)
}

func tail(s string, loc Location) (string, bool) {
	switch loc {
	case Line:
		if !strings.HasPrefix(s, "<") {
			return "", false
		}
		return s[1:], true
	case Inline:
		if !strings.HasPrefix(s, "@") {
			return "", false
		}
		_, after, ok := strings.Cut(s, "<")
		if !ok {
			return "", false
		}
		return after, true
	default:
		return "", false
	}
}

func parsePath(s string) (string, bool) {
	if !hasLeadingSpace(s) {
		return "", false
	}
	p := str.Trim(s)
	if p == "" {
		return "", false
	}
	return p, true
}

func hasLeadingSpace(s string) bool {
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsSpace(r)
}
