package intellisense

import (
	"strings"
	"unicode"
)

type Kind int

const (
	KindNone         Kind = iota
	KindDirective         // @directive name on a comment line
	KindDirectiveArg      // a sub-token of a directive (auth/k8s/trace/...)
	KindMethod            // first token of a request line
	KindScheme            // URL scheme after a request method
	KindHeaderName        // start of a header line
	KindHeaderValue       // value after "Name:" on a header line
	KindVariable          // identifier inside an open {{ ... }}
)

type Context struct {
	Kind      Kind
	Directive string // base key (KindDirectiveArg) or header name (KindHeaderValue)
	ArgKey    string // option key when completing a value, e.g. "use"
	Query     string // partial token being completed, lowercased
	Start     int    // rune offset within the caret line where Query begins
}

// Lines exposes the buffer's logical lines to Analyze - LineRunes(i) is the
// 0-based line i (the same sequence as strings.Split(value, "\n")).
type Lines interface {
	LineCount() int
	LineRunes(i int) []rune
}

func Analyze(lines Lines, line, col int) (Context, bool) {
	if line < 0 || line >= lines.LineCount() {
		return Context{}, false
	}
	cur := lines.LineRunes(line)
	if col < 0 {
		col = 0
	}
	if col > len(cur) {
		col = len(cur)
	}

	if ctx, ok := analyzeVariable(cur, col); ok {
		return ctx, true
	}
	if marker := commentPrefixLen(cur); marker >= 0 {
		return analyzeDirective(cur, marker, col)
	}
	return analyzeRequest(lines, line, cur, col)
}

func analyzeVariable(cur []rune, col int) (Context, bool) {
	open := -1
	for i := col - 1; i > 0; i-- {
		if cur[i] == '{' && cur[i-1] == '{' {
			open = i + 1
			break
		}
		if cur[i] == '}' && cur[i-1] == '}' {
			return Context{}, false
		}
	}
	if open < 0 {
		return Context{}, false
	}
	start := col
	for start > open && isVarRune(cur[start-1]) {
		start--
	}
	return Context{
		Kind:  KindVariable,
		Query: strings.ToLower(string(cur[start:col])),
		Start: start,
	}, true
}

func analyzeDirective(cur []rune, marker, col int) (Context, bool) {
	at := -1
	for i := col - 1; i >= marker; i-- {
		if cur[i] == '@' {
			at = i
			break
		}
	}
	if at < 0 {
		return Context{}, false
	}
	// everything between the marker and '@' must be blank for this to be a
	// directive anchor (e.g. "# @", "/* @"), not an '@' inside other text.
	if strings.TrimSpace(string(cur[marker:at])) != "" {
		return Context{}, false
	}

	ctx, ok := analyzeDirectiveArea(cur[at+1 : col])
	if !ok {
		return Context{}, false
	}
	// area offsets are relative to the rune after '@'.
	if ctx.Kind == KindDirective {
		ctx.Start = at
	} else {
		ctx.Start += at + 1
	}
	return ctx, true
}

func analyzeDirectiveArea(area []rune) (Context, bool) {
	if len(area) == 0 {
		return Context{Kind: KindDirective}, true
	}

	firstSpace := -1
	for i, r := range area {
		if unicode.IsSpace(r) {
			firstSpace = i
			break
		}
		if !isQueryRune(r) {
			return Context{}, false
		}
	}
	if firstSpace == -1 {
		return Context{Kind: KindDirective, Query: strings.ToLower(string(area))}, true
	}
	if firstSpace == 0 {
		return Context{}, false
	}

	base := normalizeKey(string(area[:firstSpace]))
	if base == "" {
		return Context{}, false
	}

	start, token, ok := splitToken(area, skipSpaces(area, firstSpace))
	if !ok {
		return Context{}, false
	}

	ctx := Context{Kind: KindDirectiveArg, Directive: base, Start: start}
	if key, val, found := splitValueToken(token); found {
		ctx.ArgKey = key
		ctx.Query = strings.ToLower(val)
		ctx.Start = start + (len(token) - len([]rune(val)))
	} else {
		ctx.Query = strings.ToLower(string(token))
	}
	return ctx, true
}

func analyzeRequest(lines Lines, line int, cur []rune, col int) (Context, bool) {
	methodLine := -1
	for i := line - 1; i >= 0; i-- {
		s := string(lines.LineRunes(i))
		if strings.HasPrefix(strings.TrimSpace(s), "###") {
			break
		}
		if looksLikeRequestLine(s) {
			methodLine = i
			break
		}
	}

	if methodLine < 0 {
		return requestLineContext(cur, col)
	}

	for i := methodLine + 1; i < line; i++ {
		if strings.TrimSpace(string(lines.LineRunes(i))) == "" {
			return Context{}, false // blank line ends the header section -> body
		}
	}
	if strings.TrimSpace(string(cur)) == "" {
		return Context{}, false
	}
	return headerContext(cur, col)
}

func requestLineContext(cur []rune, col int) (Context, bool) {
	start := 0
	for start < len(cur) && unicode.IsSpace(cur[start]) {
		start++
	}
	end := start
	for end < len(cur) && !unicode.IsSpace(cur[end]) {
		end++
	}

	// First token: the request method.
	if col <= end {
		if col < start {
			return Context{}, false
		}
		for _, r := range cur[start:col] {
			if !isMethodRune(r) {
				return Context{}, false
			}
		}
		return Context{Kind: KindMethod, Query: strings.ToLower(string(cur[start:col])), Start: start}, true
	}

	// Second token: the URL scheme, while still typing scheme letters (before "://").
	u := end
	for u < len(cur) && unicode.IsSpace(cur[u]) {
		u++
	}
	uEnd := u
	for uEnd < len(cur) && !unicode.IsSpace(cur[uEnd]) {
		uEnd++
	}
	if col <= u || col > uEnd {
		return Context{}, false
	}
	for _, r := range cur[u:col] {
		if !unicode.IsLetter(r) {
			return Context{}, false
		}
	}
	return Context{Kind: KindScheme, Query: strings.ToLower(string(cur[u:col])), Start: u}, true
}

func headerContext(cur []rune, col int) (Context, bool) {
	colon := -1
	for i, r := range cur {
		if r == ':' {
			colon = i
			break
		}
	}

	if colon < 0 || col <= colon {
		start := 0
		for start < len(cur) && unicode.IsSpace(cur[start]) {
			start++
		}
		end := col
		if colon >= 0 && colon < end {
			end = colon
		}
		if end < start {
			return Context{}, false
		}
		return Context{
			Kind:  KindHeaderName,
			Query: strings.ToLower(strings.TrimSpace(string(cur[start:end]))),
			Start: start,
		}, true
	}

	name := strings.ToLower(strings.TrimSpace(string(cur[:colon])))
	start := colon + 1
	for start < col && unicode.IsSpace(cur[start]) {
		start++
	}
	return Context{
		Kind:      KindHeaderValue,
		Directive: name,
		Query:     strings.ToLower(string(cur[start:col])),
		Start:     start,
	}, true
}

func commentPrefixLen(cur []rune) int {
	i := 0
	for i < len(cur) && unicode.IsSpace(cur[i]) {
		i++
	}
	rest := cur[i:]
	switch {
	case hasRunePrefix(rest, "//"), hasRunePrefix(rest, "/*"), hasRunePrefix(rest, "--"):
		return i + 2
	case hasRunePrefix(rest, "#"), hasRunePrefix(rest, "*"):
		return i + 1
	default:
		return -1
	}
}

func looksLikeRequestLine(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	fields := strings.Fields(t)
	if IsMethodKeyword(fields[0]) {
		return true
	}
	lower := strings.ToLower(t)
	return strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://")
}

func splitValueToken(token []rune) (key, value string, ok bool) {
	for i, r := range token {
		if r == '=' {
			// only use= gets value completion. Other key=value tokens stay whole
			// so their label still prefix-matches (e.g. enabled=true).
			if k := strings.ToLower(string(token[:i])); k == "use" {
				return k, string(token[i+1:]), true
			}
			return "", "", false
		}
		if !isQueryRune(r) {
			return "", "", false
		}
	}
	return "", "", false
}

func skipSpaces(area []rune, start int) int {
	for start < len(area) {
		if !unicode.IsSpace(area[start]) {
			return start
		}
		start++
	}
	return len(area)
}

func splitToken(area []rune, start int) (int, []rune, bool) {
	tokenStart := start
	pos := start
	for pos < len(area) {
		r := area[pos]
		if unicode.IsSpace(r) {
			pos++
			for pos < len(area) && unicode.IsSpace(area[pos]) {
				pos++
			}
			tokenStart = pos
			continue
		}
		if !isSubcommandRune(r) {
			return 0, nil, false
		}
		pos++
	}
	return tokenStart, area[tokenStart:], true
}

func hasRunePrefix(s []rune, prefix string) bool {
	p := []rune(prefix)
	if len(s) < len(p) {
		return false
	}
	for i := range p {
		if s[i] != p[i] {
			return false
		}
	}
	return true
}

func isQueryRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	return r == '-' || r == '_'
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

func isVarRune(r rune) bool {
	return isQueryRune(r) || r == '$' || r == '.'
}

func isMethodRune(r rune) bool {
	return unicode.IsLetter(r)
}

func IsTokenRune(r rune) bool {
	return isVarRune(r)
}
