// Package lexer tokenizes directive text into whitespace separated fields.
// It understands quoted values, optional backslash escapes and bracketed
// JSON-like values that have to stay inside a single field.
package lexer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func SplitFirst(text string) (string, string) {
	tr := strings.TrimSpace(text)
	if tr == "" {
		return "", ""
	}
	f := strings.Fields(tr)
	if len(f) == 0 {
		return "", ""
	}
	tok := f[0]
	rem := strings.TrimSpace(tr[len(tok):])
	return tok, rem
}

func SplitDirective(text string) (string, string) {
	f := strings.Fields(text)
	if len(f) == 0 {
		return "", ""
	}

	key := strings.ToLower(strings.TrimRight(f[0], ":"))
	var rest string
	if len(text) > len(f[0]) {
		rest = strings.TrimSpace(text[len(f[0]):])
	}
	return key, rest
}

// Fields splits input on spaces while keeping quoted values together.
// Quotes themselves get stripped, so `"hello resterm"` becomes `hello resterm`.
func Fields(input string) []string {
	return New(input).collect()
}

// FieldsEscaped is like Fields but also treats backslashes as escapes.
// A trailing backslash gets preserved if nothing follows it.
func FieldsEscaped(input string) []string {
	return NewEscaped(input).collect()
}

// A Lexer scans directive text one field at a time. Fields split on
// whitespace. A quoted value stays in one field and loses its quotes. A "["
// or "{" right after "=" opens a bracket balanced run that also stays in one
// field. In escape mode a backslash makes the next rune literal.
type Lexer struct {
	src     string
	pos     int
	escapes bool
}

func New(src string) *Lexer {
	return &Lexer{src: src}
}

// NewEscaped returns a Lexer that treats backslashes as escapes.
func NewEscaped(src string) *Lexer {
	return &Lexer{src: src, escapes: true}
}

// Next returns the next field. ok is false once the input is exhausted.
func (l *Lexer) Next() (field string, ok bool) {
	var tok strings.Builder
	var quote rune
	var last rune
	hasLast := false
	escaping := false
	var jsonClosers []rune
	inJSONString := false
	jsonEscaped := false

	write := func(r rune) {
		tok.WriteRune(r)
		last = r
		hasLast = true
	}

	startsQuotedValue := func() bool {
		return tok.Len() == 0 || (hasLast && last == '=')
	}

	startsJSONValue := func(r rune) bool {
		return (r == '[' || r == '{') && hasLast && last == '='
	}

	pushJSONCloser := func(r rune) {
		switch r {
		case '[':
			jsonClosers = append(jsonClosers, ']')
		case '{':
			jsonClosers = append(jsonClosers, '}')
		}
	}

	popJSONCloser := func(r rune) {
		if len(jsonClosers) == 0 {
			return
		}
		// The lexer groups JSON-like directive values but does not validate them.
		// Downstream parsers validate each field according to its own syntax.
		if jsonClosers[len(jsonClosers)-1] != r {
			return
		}
		jsonClosers = jsonClosers[:len(jsonClosers)-1]
	}

	for i, r := range l.src[l.pos:] {
		switch {
		case len(jsonClosers) > 0:
			write(r)
			switch {
			case inJSONString:
				if jsonEscaped {
					jsonEscaped = false
					continue
				}
				if r == '\\' {
					jsonEscaped = true
					continue
				}
				if r == '"' {
					inJSONString = false
				}
			case r == '"':
				inJSONString = true
			case r == '[' || r == '{':
				pushJSONCloser(r)
			default:
				popJSONCloser(r)
			}
		case escaping:
			write(r)
			escaping = false
		case l.escapes && r == '\\':
			escaping = true
		case quote != 0:
			if r == quote {
				quote = 0
				break
			}
			write(r)
		case startsJSONValue(r):
			write(r)
			pushJSONCloser(r)
		case (r == '"' || r == '\'') && startsQuotedValue():
			quote = r
		case unicode.IsSpace(r):
			if tok.Len() > 0 {
				l.pos += i + utf8.RuneLen(r)
				return tok.String(), true
			}
		default:
			write(r)
		}
	}

	l.pos = len(l.src)
	if escaping {
		write('\\')
	}
	if tok.Len() > 0 {
		return tok.String(), true
	}
	return "", false
}

func (l *Lexer) collect() []string {
	var fields []string
	for f, ok := l.Next(); ok; f, ok = l.Next() {
		fields = append(fields, f)
	}
	return fields
}

func TrimQuotes(value string) string {
	if len(value) >= 2 {
		f := value[0]
		l := value[len(value)-1]
		if (f == '"' && l == '"') || (f == '\'' && l == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func IsIdent(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if !isIdentStartRune(r) {
				return false
			}
			continue
		}
		if !IsIdentRune(r) {
			return false
		}
	}
	return true
}

func isIdentStartRune(r rune) bool {
	return r == '_' || isAlpha(r)
}

func IsIdentRune(r rune) bool {
	return isIdentStartRune(r) || isDigit(r)
}

func isAlpha(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
