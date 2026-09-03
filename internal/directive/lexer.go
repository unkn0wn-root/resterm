package directive

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Fields keeps quoted values together, as well as the bracketed JSON and the
// call arguments that follow an equals sign.
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

	tok   strings.Builder
	begun bool
	start int
	field fieldScan
}

type fieldScan struct {
	written bool
	last    rune
	quote   rune
	escaped bool
	// closers holds the brackets still waiting to be matched, outermost first.
	closers  []rune
	inString bool
	strEsc   bool
	state    nameState
}

type fieldStep uint8

const (
	fieldSkip fieldStep = iota
	fieldKeep
	fieldEscaped
	fieldEnd
)

// nameState tracks how much of the token still reads as a name, which is what
// tells an argument list from a value that merely holds a parenthesis.
type nameState uint8

const (
	inKey  nameState = iota // before the equals sign
	inName                  // every rune of the value so far is a name rune
	inText                  // the value holds something a name cannot
)

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
		switch l.field.feed(r, l.escapes) {
		case fieldEscaped:
			l.tok.WriteRune('\\')
			l.tok.WriteRune(r)
		case fieldKeep:
			l.tok.WriteRune(r)
		case fieldEnd:
			l.pos = base + i + utf8.RuneLen(r)
			return token{val: l.tok.String(), start: l.start, end: base + i}, true
		}
	}

	l.pos = len(l.src)
	if l.field.escaped {
		l.tok.WriteRune('\\')
	}
	if l.tok.Len() == 0 {
		return token{}, false
	}
	return token{val: l.tok.String(), start: l.start, end: len(l.src)}, true
}

func (l *lexer) reset() {
	l.tok.Reset()
	l.begun = false
	l.field.reset()
}

func (f *fieldScan) feed(r rune, escapes bool) fieldStep {
	switch {
	case len(f.closers) > 0:
		f.group(r)
		return fieldKeep
	case f.escaped:
		f.escaped = false
		// Preserve backslashes that do not escape the quote or another backslash.
		if f.quote != 0 && r != f.quote && r != '\\' {
			f.write('\\')
			f.write(r)
			return fieldEscaped
		}
		f.write(r)
		return fieldKeep
	case escapes && r == '\\':
		f.escaped = true
		return fieldSkip
	case f.quote != 0:
		if r == f.quote {
			f.quote = 0
			return fieldSkip
		}
		f.write(r)
		return fieldKeep
	case f.opens(r):
		f.write(r)
		f.push(r)
		return fieldKeep
	case (r == '"' || r == '\'') && f.startsValue():
		f.quote = r
		return fieldSkip
	case unicode.IsSpace(r):
		if f.written {
			return fieldEnd
		}
		return fieldSkip
	default:
		f.name(r)
		f.write(r)
		return fieldKeep
	}
}

func (f *fieldScan) reset() {
	f.written = false
	f.last = 0
	f.quote = 0
	f.escaped = false
	f.closers = f.closers[:0]
	f.inString = false
	f.strEsc = false
	f.state = inKey
}

func (f *fieldScan) write(r rune) {
	f.written = true
	f.last = r
}

func (f *fieldScan) closer() rune {
	if n := len(f.closers); n > 0 {
		return f.closers[n-1]
	}
	return f.quote
}

// A value opens a group when a bracket follows the equals sign, or when an
// argument list follows a name: headers={"a":"b"} and latency=random(1s,2s).
func (f *fieldScan) opens(r rune) bool {
	switch r {
	case '[', '{':
		return f.last == '='
	case '(':
		return f.state == inName && f.last != '='
	}
	return false
}

func (f *fieldScan) name(r rune) {
	switch {
	case f.state == inKey:
		if r == '=' && f.written {
			f.state = inName
		}
	case f.state == inName && !IsKeyRune(r):
		f.state = inText
	}
}

// A quote groups a value when it opens the token or follows the equals sign.
func (f *fieldScan) startsValue() bool {
	return !f.written || f.last == '='
}

// Delimiters inside JSON strings are ordinary characters. Outside strings we
// track nesting, but leave mismatched ones for the value parser to reject.
func (f *fieldScan) group(r rune) {
	f.write(r)
	switch {
	case f.inString:
		switch {
		case f.strEsc:
			f.strEsc = false
		case r == '\\':
			f.strEsc = true
		case r == '"':
			f.inString = false
		}
	case r == '"':
		f.inString = true
	case isOpener(r):
		f.push(r)
	default:
		f.pop(r)
	}
}

func isOpener(r rune) bool {
	return r == '[' || r == '{' || r == '('
}

func (f *fieldScan) push(r rune) {
	switch r {
	case '[':
		f.closers = append(f.closers, ']')
	case '{':
		f.closers = append(f.closers, '}')
	case '(':
		f.closers = append(f.closers, ')')
	}
}

func (f *fieldScan) pop(r rune) {
	if n := len(f.closers); n > 0 && f.closers[n-1] == r {
		f.closers = f.closers[:n-1]
	}
}
