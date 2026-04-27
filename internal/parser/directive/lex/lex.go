package lex

import (
	"strings"
	"unicode"
)

type fieldMode uint8

const (
	fieldPlain fieldMode = iota
	fieldEscaped
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

// TokenizeFields tokenizes fields on spaces while keeping quoted values together.
// Quotes themselves get stripped, so `"hello resterm"` becomes `hello resterm`.
func TokenizeFields(input string) []string {
	return tokenizeFieldsWithMode(input, fieldPlain)
}

// TokenizeFieldsEscaped is like TokenizeFields but also treats backslashes as escapes.
// A trailing backslash gets preserved if nothing follows it.
func TokenizeFieldsEscaped(input string) []string {
	return tokenizeFieldsWithMode(input, fieldEscaped)
}

func tokenizeFieldsWithMode(input string, mode fieldMode) []string {
	var tokens []string
	var current strings.Builder
	var quote rune
	var last rune
	hasLast := false
	escaping := false
	var jsonClosers []rune
	inJSONString := false
	jsonEscaped := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
		last = 0
		hasLast = false
	}

	write := func(r rune) {
		current.WriteRune(r)
		last = r
		hasLast = true
	}

	startsQuotedValue := func() bool {
		return current.Len() == 0 || (hasLast && last == '=')
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
		// This tokenizer groups JSON-like directive values but does not validate them.
		// Downstream parsers validate each field according to its own syntax.
		if jsonClosers[len(jsonClosers)-1] != r {
			return
		}
		jsonClosers = jsonClosers[:len(jsonClosers)-1]
	}

	for _, r := range input {
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
		case mode == fieldEscaped && r == '\\':
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
			flush()
		default:
			write(r)
		}
	}
	if escaping {
		write('\\')
	}
	flush()
	return tokens
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
			if !IsIdentStartRune(r) {
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

func IsIdentStartRune(r rune) bool {
	return r == '_' || isAlpha(r)
}

func IsIdentRune(r rune) bool {
	return IsIdentStartRune(r) || isDigit(r)
}

func isAlpha(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
