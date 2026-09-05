package intellisense

import (
	"slices"
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

type Kind int

const (
	KindNone         Kind = iota
	KindDirective         // @directive name, optionally before its comment prefix is inserted
	KindDirectiveArg      // a sub-token of a directive (auth/k8s/trace/...)
	KindMethod            // first token of a request line
	KindScheme            // URL scheme after a request method
	KindHeaderName        // start of a header line
	KindHeaderValue       // value after "Name:" on a header line
	KindVariable          // identifier inside an open {{ ... }}
	KindPath              // a filesystem path, in a directive or a body reference
)

// PathKind identifies which file types to suggest.
type PathKind uint8

const (
	PathNone PathKind = iota
	PathAny
	PathRTS
	PathGraphQL
	PathJSON
	PathScript
)

// PathContext describes the syntax allowed for a path completion.
type PathContext struct {
	Kind      PathKind
	Quote     bool   // quote paths that contain spaces
	List      bool   // complete one entry in a list of paths
	Home      bool   // expand a leading "~"
	Continues bool   // more arguments may follow the path
	Suffix    string // decoded value after the caret in a list
}

type Context struct {
	Kind  Kind
	Query string // the partial token being completed
	Start int    // first rune offset to replace on the current line
	End   int    // rune offset after the text to replace
	Path  PathContext

	name      directive.Name
	header    string    // lowercased header name for KindHeaderValue
	arg       *argument // argument whose value is being completed
	completed completed
	closing   string // text that completes an unfinished template
	call      bool   // the identifier is followed by an argument list
	bare      bool   // the directive was typed without its comment marker
}

// completed stores values already on the line under each argument's canonical name.
type completed map[string][]string

func (c completed) add(key, value string) {
	c[key] = append(c[key], value)
}

func (c completed) has(key string) bool {
	return len(c[key]) > 0
}

func (c completed) holds(key, value string) bool {
	return slices.ContainsFunc(c[key], func(v string) bool {
		return strings.EqualFold(v, value)
	})
}

func (c completed) last(key string) (string, bool) {
	values := c[key]
	if len(values) == 0 {
		return "", false
	}
	return values[len(values)-1], true
}

// Lines exposes newline-separated buffer lines to Analyze. Line indexes start at zero.
type Lines interface {
	LineCount() int
	LineRunes(i int) []rune
}

func Analyze(lines Lines, line, col int) (Context, bool) {
	if line < 0 || line >= lines.LineCount() {
		return Context{}, false
	}
	cur := lines.LineRunes(line)
	col = clamp(col, 0, len(cur))

	if ctx, ok := analyzeVariable(cur, col); ok {
		return ctx, true
	}
	if marker := commentPrefixLen(cur); marker >= 0 {
		return analyzeDirective(cur, marker, col, false)
	}
	if ctx, ok := analyzeDirective(cur, 0, col, true); ok {
		return ctx, true
	}
	if ctx, ok := analyzeBodyPath(cur, col); ok {
		return ctx, true
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
	for start > open && IsTokenRune(cur[start-1]) {
		start--
	}
	end := col
	for end < len(cur) && IsTokenRune(cur[end]) {
		end++
	}
	ctx := Context{Kind: KindVariable, Query: string(cur[start:col]), Start: start, End: end}

	next := skipSpace(cur, end)
	if next < len(cur) && cur[next] == '(' {
		ctx.call = true
		return ctx, true
	}
	// Missing braces go after the whitespace, so the replacement covers it.
	if closers := skipPartial(cur, next, "}}") - next; closers < 2 {
		ctx.End = next
		ctx.closing = string(cur[end:next]) + strings.Repeat("}", 2-closers)
	}
	return ctx, true
}

// analyzeBodyPath handles body files ("< file"), script includes ("> < file"),
// and inline body includes ("@ file").
func analyzeBodyPath(cur []rune, col int) (Context, bool) {
	start := leadingSpaceLen(cur)
	if start >= len(cur) || col < start {
		return Context{}, false
	}
	kind := PathAny
	switch cur[start] {
	case '<', '@':
		start++
	case '>':
		start = skipSpace(cur, start+1)
		if start >= len(cur) || cur[start] != '<' {
			return Context{}, false
		}
		start++
		kind = PathScript
	default:
		return Context{}, false
	}
	if start < len(cur) && !unicode.IsSpace(cur[start]) {
		return Context{}, false
	}
	start = skipSpace(cur, start)
	if col < start {
		return Context{}, false
	}
	return Context{
		Kind:  KindPath,
		Query: string(cur[start:col]),
		Start: start,
		End:   len(cur),
		Path:  PathContext{Kind: kind},
	}, true
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
	start := skipSpace(cur, 0)
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
			if !unicode.IsLetter(r) {
				return Context{}, false
			}
		}
		return Context{
			Kind:  KindMethod,
			Query: strings.ToLower(string(cur[start:col])),
			Start: start,
			End:   end,
		}, true
	}

	// Second token: the URL scheme, while still typing scheme letters (before "://").
	u := skipSpace(cur, end)
	uEnd := u
	for uEnd < len(cur) && unicode.IsLetter(cur[uEnd]) {
		uEnd++
	}
	if col <= u || col > uEnd {
		return Context{}, false
	}
	return Context{
		Kind:  KindScheme,
		Query: strings.ToLower(string(cur[u:col])),
		Start: u,
		End:   skipPartial(cur, uEnd, "://"),
	}, true
}

func headerContext(cur []rune, col int) (Context, bool) {
	colon := slices.Index(cur, ':')

	if colon < 0 || col <= colon {
		start := skipSpace(cur, 0)
		if col < start {
			return Context{}, false
		}
		end := len(cur)
		if colon >= 0 {
			end = colon + 1
		}
		return Context{
			Kind:  KindHeaderName,
			Query: strings.ToLower(strings.TrimSpace(string(cur[start:col]))),
			Start: start,
			End:   end,
		}, true
	}

	start := colon + 1
	for start < col && unicode.IsSpace(cur[start]) {
		start++
	}
	return Context{
		Kind:   KindHeaderValue,
		header: strings.ToLower(strings.TrimSpace(string(cur[:colon]))),
		Query:  strings.ToLower(string(cur[start:col])),
		Start:  start,
		End:    len(cur),
	}, true
}

func commentPrefixLen(cur []rune) int {
	i := leadingSpaceLen(cur)
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

func leadingSpaceLen(cur []rune) int {
	return skipSpace(cur, 0)
}

func skipSpace(cur []rune, i int) int {
	for i < len(cur) && unicode.IsSpace(cur[i]) {
		i++
	}
	return i
}

// skipPartial consumes the matching prefix of want at i.
func skipPartial(cur []rune, i int, want string) int {
	for _, r := range want {
		if i >= len(cur) || cur[i] != r {
			break
		}
		i++
	}
	return i
}

func clamp(v, lo, hi int) int {
	return min(max(v, lo), hi)
}

func looksLikeRequestLine(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	token := t
	if i := strings.IndexFunc(t, unicode.IsSpace); i >= 0 {
		token = t[:i]
	}
	if IsMethodKeyword(token) {
		return true
	}
	lower := strings.ToLower(t)
	return strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://")
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

func IsTokenRune(r rune) bool {
	return isQueryRune(r) || r == '$' || r == '.'
}
