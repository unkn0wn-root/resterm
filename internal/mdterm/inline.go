package mdterm

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type span struct {
	text string
	at   attr
}

func renderInline(s string, base attr, st styler) string {
	spans := make([]span, 0, 8)
	parseInline(s, base, &spans)
	var b strings.Builder
	for _, sp := range spans {
		b.WriteString(st.span(sp.text, sp.at))
	}
	return b.String()
}

// scanner holds one parseInline pass over s: literal bytes collect in lit
// until a construct method flushes them as a span and every emitted span
// inherits the attributes in at.
type scanner struct {
	s   string
	at  attr
	out *[]span
	lit strings.Builder
}

// parseInline splits s into styled spans in one left-to-right scan.
// Each construct method consumes its syntax and returns the index the scan
// resumes at. Parsing is identical in color and plain modes. Anything
// malformed - an unclosed delimiter, a broken link - stays literal text and
// the scan resumes after the failed opener so it is never rescanned.
func parseInline(s string, at attr, out *[]span) {
	sc := scanner{s: s, at: at, out: out}
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '\\' && escapes(s, i):
			sc.lit.WriteByte(s[i+1]) // \* renders a literal *
			i += 2
		case c == '`':
			i = sc.code(i)
		case c == '*' || c == '_':
			i = sc.emphasis(i)
		case c == '[', c == '!' && strings.HasPrefix(s[i+1:], "["):
			i = sc.link(i)
		case c == 'h' && urlStart(s, i):
			i = sc.url(i)
		default:
			sc.lit.WriteByte(c)
			i++
		}
	}
	sc.flush()
}

func (sc *scanner) flush() {
	if sc.lit.Len() > 0 {
		*sc.out = append(*sc.out, span{sc.lit.String(), sc.at})
		sc.lit.Reset()
	}
}

// emit flushes pending literal text and appends one styled span.
func (sc *scanner) emit(text string, a attr) {
	sc.flush()
	*sc.out = append(*sc.out, span{text, sc.at | a})
}

// code consumes a `code` span: content is verbatim between backtick runs of
// equal length; an unclosed run stays literal.
func (sc *scanner) code(i int) int {
	n := runLen(sc.s, i)
	j := closingRun(sc.s, i+n, n)
	if j < 0 {
		sc.lit.WriteString(sc.s[i : i+n])
		return i + n
	}
	sc.emit(sc.s[i+n:j], aCode)
	return j + n
}

// emphasis consumes *italic*, **bold** or ***both***, recursing so the
// enclosed text keeps its own constructs.
// A failed opener stays literal and the scan skips its whole run.
func (sc *scanner) emphasis(i int) int {
	txt, a, end, ok := emphAt(sc.s, i)
	if !ok {
		n := runLen(sc.s, i)
		sc.lit.WriteString(sc.s[i : i+n])
		return i + n
	}
	sc.flush()
	parseInline(txt, sc.at|a, sc.out)
	return end
}

// link consumes [text](url) or ![alt](url); images render as links.
func (sc *scanner) link(i int) int {
	br := i
	if sc.s[i] == '!' {
		br++
	}
	txt, url, end, ok := linkAt(sc.s, br)
	if !ok {
		sc.lit.WriteByte(sc.s[i])
		return i + 1
	}
	sc.emitLink(txt, url)
	return end
}

// emitLink emits "text (url)"
// when the text is empty or just repeats the url, the url alone stands in.
func (sc *scanner) emitLink(txt, url string) {
	if txt == "" || txt == url {
		sc.emit(url, aFaint)
		return
	}
	sc.flush()
	parseInline(txt, sc.at|aLink, sc.out)
	*sc.out = append(*sc.out, span{" (", sc.at}, span{url, sc.at | aFaint}, span{")", sc.at})
}

// url consumes a bare http(s):// URL: the token runs to whitespace, with
// trailing punctuation and a stray closing paren trimmed.
func (sc *scanner) url(i int) int {
	j := i
	for j < len(sc.s) && sc.s[j] != ' ' && sc.s[j] != '\t' {
		j++
	}
	u := strings.TrimRight(sc.s[i:j], ".,;:!?")
	if strings.HasSuffix(u, ")") && !strings.Contains(u, "(") {
		u = u[:len(u)-1]
	}
	sc.emit(u, aFaint)
	return i + len(u)
}

// escapes reports whether the backslash at s[i] escapes a markdown byte.
func escapes(s string, i int) bool {
	return i+1 < len(s) && strings.IndexByte("\\`*_[]", s[i+1]) >= 0
}

// emphAt parses an emphasis run opening at s[i] and returns the enclosed
// text, its attributes and the index just past the closer. It tries the
// longest delimiter first so ***x*** nests bold+italic. Openers need a
// following non-space and closers a preceding non-space ("2 * 3" stays
// literal); _ additionally needs word boundaries so snake_case survives.
func emphAt(s string, i int) (txt string, a attr, end int, ok bool) {
	c := s[i]
	if c == '_' && wordBefore(s, i) {
		return "", 0, 0, false
	}
	n := runLen(s, i)
	for k := min(n, 3); k >= 1; k-- {
		d := s[i : i+k]
		start := i + k
		if start >= len(s) || s[start] == ' ' {
			continue
		}
		j := findDelim(s, start, d, c == '_')
		if j < 0 {
			continue
		}
		switch k {
		case 3:
			a = aBold | aItalic
		case 2:
			a = aBold
		default:
			a = aItalic
		}
		return s[start:j], a, j + k, true
	}
	return "", 0, 0, false
}

// findDelim scans for a valid closer of the emphasis delimiter d, skipping
// escapes and code spans so `**` inside backticks never closes a bold run.
// under adds the _ closing rule: no word character may follow the closer.
func findDelim(s string, from int, d string, under bool) int {
	for i := from; i < len(s); {
		switch s[i] {
		case '\\':
			i += 2
		case '`':
			n := runLen(s, i)
			if j := closingRun(s, i+n, n); j >= 0 {
				i = j + n
			} else {
				i += n
			}
		default:
			if strings.HasPrefix(s[i:], d) && i > from && s[i-1] != ' ' &&
				(!under || !wordAfter(s, i+len(d))) {
				return i
			}
			i++
		}
	}
	return -1
}

// linkAt parses [text](url) with the opening bracket at s[i], returning the
// two parts and the index just past the final paren. Brackets may nest
// inside the text. The url must have balanced parens and no whitespace.
// Backslash escaped brackets and parens don't count as delimiters.
func linkAt(s string, i int) (txt, url string, end int, ok bool) {
	cl := -1 // index of the ] closing the text part
	depth := 0
scan:
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				cl = j
				break scan
			}
		}
	}
	if cl < 0 || cl+1 >= len(s) || s[cl+1] != '(' {
		return "", "", 0, false
	}
	pd := 0 // paren depth inside the url part
	for j := cl + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '(':
			pd++
		case ')':
			pd--
			if pd == 0 {
				url = strings.NewReplacer(`\(`, "(", `\)`, ")").Replace(s[cl+2 : j])
				return s[i+1 : cl], url, j + 1, true
			}
		case ' ', '\t':
			return "", "", 0, false
		}
	}
	return "", "", 0, false
}

// urlStart reports whether a bare URL starts at s[i]
// (not mid-word, so "xhttp://" stays literal).
func urlStart(s string, i int) bool {
	if wordBefore(s, i) {
		return false
	}
	return strings.HasPrefix(s[i:], "http://") || strings.HasPrefix(s[i:], "https://")
}

// runLen counts the run of bytes equal to s[i] starting at i.
func runLen(s string, i int) int {
	c := s[i]
	n := 1
	for i+n < len(s) && s[i+n] == c {
		n++
	}
	return n
}

// closingRun finds the next backtick run of exactly n bytes - a code span's
// closer must match its opener's length so a double-backtick span can
// contain single backticks.
func closingRun(s string, from, n int) int {
	for i := from; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		l := runLen(s, i)
		if l == n {
			return i
		}
		i += l
	}
	return -1
}

// Word boundaries are checked as runes so multibyte letters (café, кофе)
// count as word characters and intraword _ stays literal.
func wordBefore(s string, i int) bool {
	if i == 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func wordAfter(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
