package hint

import (
	"strings"
	"unicode"
)

type Mode int

const (
	ModeDirective Mode = iota
	ModeSubcommand
)

type Context struct {
	Mode       Mode
	Directive  string
	BaseKey    string
	Query      string
	TokenStart int
}

func AnalyzeContext(query []rune) (Context, bool) {
	ctx := Context{Mode: ModeDirective}
	if len(query) == 0 {
		return ctx, true
	}

	firstSpace := -1
	for i, r := range query {
		if r == '\n' || r == '\r' {
			return Context{}, false
		}
		if unicode.IsSpace(r) {
			firstSpace = i
			break
		}
		if !isQueryRune(r) {
			return Context{}, false
		}
	}

	if firstSpace == -1 {
		ctx.Query = strings.ToLower(string(query))
		return ctx, true
	}
	if firstSpace == 0 {
		return Context{}, false
	}

	dir := string(query[:firstSpace])
	base := NormalizeKey(dir)
	if base == "" {
		return Context{}, false
	}

	subStart := skipSpaces(query, firstSpace)
	if subStart == -1 {
		return Context{}, false
	}
	tokenStart, token, ok := splitToken(query, subStart)
	if !ok {
		return Context{}, false
	}

	ctx.Mode = ModeSubcommand
	ctx.Directive = dir
	ctx.BaseKey = base
	ctx.Query = strings.ToLower(string(token))
	ctx.TokenStart = tokenStart
	return ctx, true
}

func skipSpaces(query []rune, start int) int {
	for start < len(query) {
		if query[start] == '\n' || query[start] == '\r' {
			return -1
		}
		if !unicode.IsSpace(query[start]) {
			return start
		}
		start++
	}
	return len(query)
}

func splitToken(query []rune, start int) (int, []rune, bool) {
	tokenStart := start
	pos := start
	for pos < len(query) {
		r := query[pos]
		if r == '\n' || r == '\r' {
			return 0, nil, false
		}
		if unicode.IsSpace(r) {
			pos++
			for pos < len(query) && unicode.IsSpace(query[pos]) {
				if query[pos] == '\n' || query[pos] == '\r' {
					return 0, nil, false
				}
				pos++
			}
			if pos > len(query) {
				break
			}
			if pos <= len(query) {
				tokenStart = pos
			}
			continue
		}
		if !isSubcommandRune(r) {
			return 0, nil, false
		}
		pos++
	}
	if tokenStart > len(query) {
		tokenStart = len(query)
	}
	return tokenStart, query[tokenStart:], true
}

func isQueryRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '-', '_':
		return true
	default:
		return false
	}
}

func isSubcommandRune(r rune) bool {
	if isQueryRune(r) {
		return true
	}
	switch r {
	case '=', '<', '>', ',':
		return true
	default:
		return false
	}
}

func IsQueryRune(r rune) bool {
	return isQueryRune(r)
}
