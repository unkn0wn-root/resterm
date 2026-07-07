// Package mdterm renders the markdown subset that release notes actually
// use as terminal text: ATX headings, nested lists, fenced code blocks,
// horizontal rules, inline-only blockquotes, emphasis, code spans, links
// and bare URLs. With color enabled output is ANSI-styled. Otherwise it
// degrades to structured plain text (heading underlines, markers stripped).
//
// Intentionally not a full markdown engine: setext headings, tables,
// HTML and block constructs nested in lists or quotes are out of scope
// and consecutive paragraph lines are not joined because release notes are
// line structured.
// Anything unrecognized or malformed passes through as literal text.
package mdterm

import (
	"context"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/wrap"
)

type Options struct {
	Width int // wrap width in cells; <= 0 disables wrapping
	Color termcolor.Config
}

func Render(src string, opts Options) string {
	r := renderer{st: newStyler(opts.Color), w: opts.Width}
	lines := strings.Split(sanitize(src), "\n")
	for i := 0; i < len(lines); i++ {
		if f, ok := fenceOpen(lines[i]); ok {
			i = r.codeBlock(lines, i+1, f)
			continue
		}
		r.line(lines[i])
	}
	for len(r.out) > 0 && r.out[len(r.out)-1] == "" {
		r.out = r.out[:len(r.out)-1]
	}
	return strings.Join(r.out, "\n")
}

func Rule(cfg termcolor.Config, width int) string {
	return rule(newStyler(cfg), width)
}

func sanitize(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, src)
}

type renderer struct {
	st    styler
	w     int
	out   []string
	stack []int // source indents of open list levels
	col   int   // content column of the last list item
}

// line classifies and renders one nonfence source line; order matters
// (an HR like "* * *" must win over a "* " list marker).
func (r *renderer) line(ln string) {
	t := strings.TrimSpace(ln)
	if t == "" {
		r.blank()
		return
	}
	if isHR(t) {
		r.reset()
		r.hr()
		return
	}
	if lvl, txt, ok := headingAt(t); ok {
		r.reset()
		r.heading(lvl, txt)
		return
	}
	if t[0] == '>' {
		r.reset()
		d, txt := quote(t)
		r.quote(d, txt)
		return
	}
	if m, ok := item(t); ok {
		r.item(leadWidth(ln), m)
		return
	}
	if len(r.stack) > 0 && leadWidth(ln) >= 2 {
		// indented text under an open list item continues that item
		r.emit(t, r.col)
		return
	}
	r.reset()
	r.emit(t, 0)
}

func (r *renderer) blank() {
	if n := len(r.out); n > 0 && r.out[n-1] != "" {
		r.out = append(r.out, "")
	}
}

func (r *renderer) reset() {
	r.stack = r.stack[:0]
	r.col = 0
}

const ruleWidth = 40 // rule width when wrapping is disabled

func (r *renderer) hr() {
	r.out = append(r.out, rule(r.st, r.w))
}

func rule(st styler, w int) string {
	if w <= 0 {
		w = ruleWidth
	}
	return st.span(strings.Repeat("─", w), aFaint)
}

func (r *renderer) heading(lvl int, txt string) {
	if txt == "" {
		r.blank()
		return
	}
	base := aBold
	switch lvl {
	case 1:
		base |= aUnder
	case 2:
		base |= aAccent
	}
	r.blank()
	s := renderInline(txt, base, r.st)
	r.wrapTo(s, 0, "", "")
	if !r.st.on && lvl <= 2 {
		u := "="
		if lvl == 2 {
			u = "-"
		}
		w := runewidth.StringWidth(s)
		if r.w > 0 && w > r.w {
			w = r.w
		}
		r.out = append(r.out, strings.Repeat(u, w))
	}
}

func (r *renderer) quote(d int, txt string) {
	bar := strings.Repeat("│ ", d)
	pre := r.st.span(bar, aFaint)
	r.wrapTo(renderInline(txt, 0, r.st), 2*d, pre, pre)
}

func (r *renderer) item(ind int, m marker) {
	lvl := r.level(ind)
	g := m.num + "."
	if m.num == "" {
		g = glyph(lvl)
	}
	pad := strings.Repeat(" ", 2*lvl)
	// hanging indent: continuations align after the marker, wide ("12. ")
	// or narrow ("• ") alike
	col := 2*lvl + runewidth.StringWidth(g) + 1
	r.col = col
	r.wrapTo(renderInline(m.text, 0, r.st), col, pad+g+" ", strings.Repeat(" ", col))
}

func (r *renderer) emit(t string, col int) {
	pad := strings.Repeat(" ", col)
	r.wrapTo(renderInline(t, 0, r.st), col, pad, pad)
}

// wrapTo wraps styled text s to the remaining width after col cells of
// prefix, emitting first before the initial segment and cont before the rest.
func (r *renderer) wrapTo(s string, col int, first, cont string) {
	w := 0
	if r.w > 0 {
		if w = r.w - col; w <= 0 {
			w = r.w
		}
	}
	segs, _ := wrap.Line(context.Background(), s, w, wrap.Plain)
	for i, sg := range segs {
		// wrap reopens the active SGR state on each continuation segment, so
		// close it at every break or the style bleeds under the next indent.
		if r.st.on && i < len(segs)-1 && strings.Contains(sg, "\x1b[") {
			sg += "\x1b[0m"
		}
		if i == 0 {
			r.out = append(r.out, first+sg)
		} else {
			r.out = append(r.out, cont+sg)
		}
	}
}

// level maps a source indent to a nesting depth: deeper indents push a level,
// shallower ones pop, so both 2- and 4-space nesting styles work.
func (r *renderer) level(ind int) int {
	for len(r.stack) > 0 && ind < r.stack[len(r.stack)-1] {
		r.stack = r.stack[:len(r.stack)-1]
	}
	if len(r.stack) == 0 || ind > r.stack[len(r.stack)-1] {
		r.stack = append(r.stack, ind)
	}
	return len(r.stack) - 1
}

// codeBlock emits lines[i:] (4-space indented, never wrapped or inline-parsed)
// until the closing fence, returning the index of the fence line
// or len(lines) when the fence is unclosed.
func (r *renderer) codeBlock(lines []string, i int, f fence) int {
	for ; i < len(lines); i++ {
		if fenceClose(lines[i], f) {
			return i
		}
		t := strings.TrimRight(lines[i], " \t")
		if t == "" {
			r.out = append(r.out, "")
			continue
		}
		r.out = append(r.out, "    "+t)
	}
	return i
}

type fence struct {
	ch byte
	n  int
}

// fenceOpen matches a code-fence opener: up to three leading spaces, then
// three or more backticks or tildes; the info string ("go") is dropped.
func fenceOpen(ln string) (fence, bool) {
	i := 0
	for i < len(ln) && i < 3 && ln[i] == ' ' {
		i++
	}
	t := ln[i:]
	if len(t) < 3 || (t[0] != '`' && t[0] != '~') {
		return fence{}, false
	}
	n := runLen(t, 0)
	if n < 3 {
		return fence{}, false
	}
	// CommonMark forbids backticks in a backtick fence's info string; this
	// keeps a paragraph starting with an inline ```code``` span out of fence
	// mode, which would otherwise swallow the rest of the document.
	if t[0] == '`' && strings.IndexByte(t[n:], '`') >= 0 {
		return fence{}, false
	}
	return fence{ch: t[0], n: n}, true
}

// fenceClose matches a closer: a run of the opening char at least as long
// as the opener, with nothing else on the line.
func fenceClose(ln string, f fence) bool {
	t := strings.TrimSpace(ln)
	return len(t) >= f.n && t[0] == f.ch && runLen(t, 0) == len(t)
}

type marker struct {
	num  string // ordered-list number; empty for bullets
	text string
}

// item parses a list marker: "- ", "* ", "+ ", or ordered "12. " / "12) "
// with at most nine digits (CommonMark's limit, and what keeps a long
// number followed by a period from becoming a list).
func item(t string) (marker, bool) {
	if len(t) >= 2 && (t[0] == '-' || t[0] == '*' || t[0] == '+') && t[1] == ' ' {
		return marker{text: strings.TrimSpace(t[2:])}, true
	}
	i := 0
	for i < len(t) && i < 9 && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(t) && (t[i] == '.' || t[i] == ')') && t[i+1] == ' ' {
		return marker{num: t[:i], text: strings.TrimSpace(t[i+2:])}, true
	}
	return marker{}, false
}

func glyph(lvl int) string {
	switch lvl {
	case 0:
		return "•"
	case 1:
		return "◦"
	default:
		return "·"
	}
}

// isHR matches a horizontal rule: three or more of the same -, * or _,
// optionally space-separated ("* * *"), and nothing else.
func isHR(t string) bool {
	c := t[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	n := 0
	for i := 0; i < len(t); i++ {
		switch t[i] {
		case c:
			n++
		case ' ':
		default:
			return false
		}
	}
	return n >= 3
}

// headingAt parses an ATX heading, stripping an optional closing # run when
// it is preceded by a space (so "issue #5" and "C#" survive).
func headingAt(t string) (int, string, bool) {
	if t[0] != '#' {
		return 0, "", false
	}
	n := runLen(t, 0)
	if n > 6 || n >= len(t) || t[n] != ' ' {
		return 0, "", false
	}
	txt := strings.TrimSpace(t[n:])
	i := len(txt)
	for i > 0 && txt[i-1] == '#' {
		i--
	}
	if i > 0 && i < len(txt) && txt[i-1] == ' ' {
		txt = strings.TrimRight(txt[:i], " ")
	}
	return n, txt, true
}

func quote(t string) (int, string) {
	d := 0
	for strings.HasPrefix(t, ">") {
		t = strings.TrimPrefix(strings.TrimPrefix(t, ">"), " ")
		d++
	}
	return d, t
}

// leadWidth measures leading whitespace in columns, counting a tab as 4.
func leadWidth(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}
