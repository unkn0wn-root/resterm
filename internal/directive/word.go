package directive

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// A word on a directive line is either bare or quoted. A bare word ends at the
// first space and means exactly what it says, backslashes included. A quoted
// word holds anything, with \" and \\ standing for a quote or a backslash.
//
// Quote and CutName are inverses. Rendering a document and reading it back has
// to return what went in, and that only holds while one side is written from
// the other.

// Quote renders a word for a directive line, bare when it can be read back
// as one.
func Quote(value string) string {
	if value == "" || strings.IndexFunc(value, needsQuote) < 0 {
		return value
	}

	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for _, r := range value {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// Space ends a bare word, a quote would group it, a backslash escapes inside
// one, and an equals sign turns it into an option.
func needsQuote(r rune) bool {
	return unicode.IsSpace(r) || r == '"' || r == '\'' || r == '\\' || r == '='
}

// CutName splits a leading name off input and returns the rest as written.
// Anything the option grammar reads as key=value stays where it is, so a
// directive that opens with an option has no name.
func CutName(input string) (string, string) {
	rest := strings.TrimLeftFunc(input, unicode.IsSpace)
	if rest == "" || isOption(rest) {
		return "", input
	}
	if q := rest[0]; q == '"' || q == '\'' {
		if name, tail, ok := cutQuoted(rest, rune(q)); ok {
			return name, tail
		}
	}
	if i := strings.IndexFunc(rest, unicode.IsSpace); i >= 0 {
		return rest[:i], rest[i:]
	}
	return rest, ""
}

// ok is false for an unterminated quote, which reads as a bare word instead so
// a stray quote cannot swallow the options behind it.
func cutQuoted(input string, quote rune) (string, string, bool) {
	var b strings.Builder
	esc := false
	for i, r := range input[1:] {
		switch {
		case esc:
			esc = false
			// A backslash in front of anything else was meant literally.
			if r != quote && r != '\\' {
				b.WriteRune('\\')
			}
			b.WriteRune(r)
		case r == '\\':
			esc = true
		case r == quote:
			return b.String(), input[1+i+utf8.RuneLen(r):], true
		default:
			b.WriteRune(r)
		}
	}
	return "", "", false
}
