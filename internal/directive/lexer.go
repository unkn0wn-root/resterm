package directive

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Fields keeps quoted values and bracketed JSON after an equals sign together.
// Quotes used to group a value are not included in the returned field.
func Fields(input string) []string {
	return (&lexer{src: input}).collect()
}

// Here a backslash makes the next character literal. Inside quotes it only
// escapes a quote or another backslash, which is what lets a quoted Windows
// path keep its separators. A trailing backslash stays as it is.
func fieldsEscaped(input string) []string {
	return (&lexer{src: input, escapes: true}).collect()
}

type lexer struct {
	src     string
	pos     int
	escapes bool

	tok     strings.Builder
	last    rune
	quote   rune
	escaped bool
	begun   bool
	start   int
	// closers holds the brackets still waiting to be matched, outermost first.
	closers  []rune
	inString bool
	strEsc   bool
}

// A token carries the span it came from because meaning depends on the source.
// "a=b" and a=b decode to the same text and only one of them is an option.
type token struct {
	val   string
	start int
	end   int
}

func (l *lexer) collect() []string {
	var fields []string
	for tok, ok := l.next(); ok; tok, ok = l.next() {
		fields = append(fields, tok.val)
	}
	return fields
}

func (l *lexer) next() (token, bool) {
	l.reset()
	base := l.pos

	for i, r := range l.src[base:] {
		if !l.begun && !unicode.IsSpace(r) {
			l.begun = true
			l.start = base + i
		}
		switch {
		case len(l.closers) > 0:
			l.json(r)
		case l.escaped:
			l.escaped = false
			// Anything else inside quotes keeps the backslash it was typed with,
			// so paths do not get mangled.
			if l.quote != 0 && r != l.quote && r != '\\' {
				l.write('\\')
			}
			l.write(r)
		case l.escapes && r == '\\':
			l.escaped = true
		case l.quote != 0:
			if r == l.quote {
				l.quote = 0
				break
			}
			l.write(r)
		case (r == '[' || r == '{') && l.last == '=':
			l.write(r)
			l.push(r)
		case (r == '"' || r == '\'') && l.startsValue():
			l.quote = r
		case unicode.IsSpace(r):
			if l.tok.Len() > 0 {
				l.pos = base + i + utf8.RuneLen(r)
				return token{val: l.tok.String(), start: l.start, end: base + i}, true
			}
		default:
			l.write(r)
		}
	}

	l.pos = len(l.src)
	if l.escaped {
		l.write('\\')
	}
	if l.tok.Len() == 0 {
		return token{}, false
	}
	return token{val: l.tok.String(), start: l.start, end: len(l.src)}, true
}

func (l *lexer) reset() {
	l.tok.Reset()
	l.last = 0
	l.quote = 0
	l.escaped = false
	l.begun = false
	l.closers = l.closers[:0]
	l.inString = false
	l.strEsc = false
}

func (l *lexer) write(r rune) {
	l.tok.WriteRune(r)
	l.last = r
}

// A quote groups a value when it opens the token or follows the equals sign.
func (l *lexer) startsValue() bool {
	return l.tok.Len() == 0 || l.last == '='
}

// Brackets inside JSON strings are ordinary characters. Outside strings we
// track nesting, but leave mismatched brackets for the value parser to reject.
func (l *lexer) json(r rune) {
	l.write(r)
	switch {
	case l.inString:
		switch {
		case l.strEsc:
			l.strEsc = false
		case r == '\\':
			l.strEsc = true
		case r == '"':
			l.inString = false
		}
	case r == '"':
		l.inString = true
	case r == '[' || r == '{':
		l.push(r)
	default:
		l.pop(r)
	}
}

func (l *lexer) push(r rune) {
	switch r {
	case '[':
		l.closers = append(l.closers, ']')
	case '{':
		l.closers = append(l.closers, '}')
	}
}

func (l *lexer) pop(r rune) {
	if n := len(l.closers); n > 0 && l.closers[n-1] == r {
		l.closers = l.closers[:n-1]
	}
}
