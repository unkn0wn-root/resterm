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
	switch opt.Location {
	case Line:
		return parseLinePath(s)
	case Inline:
		return parseInlinePath(s)
	default:
		return "", false
	}
}

// ParseBodyFile returns a body file path from a protocol body line. It accepts
// standalone "< path" references and directive-style "@name < path" references.
func ParseBodyFile(line string, forceInline bool) (string, bool) {
	opt := Options{
		Location:    Line,
		ForceInline: forceInline,
	}
	if file, ok := Parse(line, opt); ok {
		return file, true
	}
	opt.Location = Inline
	return Parse(line, opt)
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

func parseLinePath(s string) (string, bool) {
	if !strings.HasPrefix(s, "<") {
		return "", false
	}
	return parsePath(s[1:])
}

func parseInlinePath(s string) (string, bool) {
	if !strings.HasPrefix(s, "@") {
		return "", false
	}

	headEnd := strings.IndexFunc(s, unicode.IsSpace)
	if headEnd <= 1 {
		return "", false
	}
	if !isDirectiveHead(s[:headEnd]) {
		return "", false
	}

	rest := str.TrimLeft(s[headEnd:])
	if !strings.HasPrefix(rest, "<") {
		return "", false
	}
	return parsePath(rest[1:])
}

func isDirectiveHead(s string) bool {
	if !strings.HasPrefix(s, "@") || len(s) == 1 {
		return false
	}
	for i, r := range s[1:] {
		if i == 0 {
			if !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if !isDirectiveRune(r) {
			return false
		}
	}
	return true
}

func hasLeadingSpace(s string) bool {
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsSpace(r)
}

func isDirectiveRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_.", r)
}
