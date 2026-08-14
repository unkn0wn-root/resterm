package prompt

import (
	"strings"
	"unicode"
)

type Token struct {
	Value string
	Start int
	End   int
}

type Line struct {
	Tokens   []Token
	Unclosed rune

	runes []rune
}

func Lex(input string) Line {
	runes := []rune(input)
	line := Line{runes: runes}
	i := skipPrefix(runes)

	for i < len(runes) {
		for i < len(runes) && unicode.IsSpace(runes[i]) {
			i++
		}
		if i == len(runes) {
			break
		}

		start := i
		var value strings.Builder
		var quote rune
		for i < len(runes) {
			r := runes[i]
			if quote == 0 {
				if unicode.IsSpace(r) {
					break
				}
				if r == '\'' || r == '"' {
					quote = r
					i++
					continue
				}
				value.WriteRune(r)
				i++
				continue
			}

			switch {
			case r == quote:
				quote = 0
				i++
			case r == '\\' && i+1 < len(runes) && runes[i+1] == quote:
				value.WriteRune(quote)
				i += 2
			default:
				value.WriteRune(r)
				i++
			}
		}

		line.Tokens = append(line.Tokens, Token{Value: value.String(), Start: start, End: i})
		if quote != 0 {
			line.Unclosed = quote
		}
	}
	return line
}

func Body(input string) (body string, start, end int) {
	runes := []rune(input)
	start = skipPrefix(runes)
	return string(runes[start:]), start, len(runes)
}

func skipPrefix(runes []rune) int {
	i := 0
	for i < len(runes) && unicode.IsSpace(runes[i]) {
		i++
	}
	if i < len(runes) && runes[i] == ':' {
		i++
	}
	return i
}

func (l Line) Values() []string {
	values := make([]string, len(l.Tokens))
	for i, token := range l.Tokens {
		values[i] = token.Value
	}
	return values
}

func (l Line) TokenAt(cursor int) (Token, int, bool) {
	cursor = l.clamp(cursor)
	for i, token := range l.Tokens {
		if cursor < token.Start {
			break
		}
		if cursor <= token.End {
			return token, i, true
		}
	}
	return Token{}, -1, false
}

func (l Line) ValueAt(token Token, cursor int) string {
	if cursor >= token.End {
		return token.Value
	}
	prefix := Lex(l.Text(token.Start, l.clamp(max(cursor, token.Start))))
	if len(prefix.Tokens) == 0 {
		return ""
	}
	return prefix.Tokens[0].Value
}

func (l Line) Text(start, end int) string {
	start = l.clamp(start)
	end = l.clamp(max(end, start))
	return string(l.runes[start:end])
}

func (l Line) SpaceBetween(start, end int) bool {
	start = l.clamp(start)
	end = l.clamp(max(end, start))
	if start == end {
		return false
	}
	for _, r := range l.runes[start:end] {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func (l Line) clamp(cursor int) int {
	return min(max(cursor, 0), len(l.runes))
}

func Quote(value string) string {
	if value != "" && !strings.ContainsFunc(value, unicode.IsSpace) &&
		!strings.ContainsAny(value, `"'`) {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
