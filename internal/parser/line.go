package parser

import (
	"strings"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

// line stores the raw text, a trimmed form for matching, and its line ending.
type line struct {
	no     int // 1-based
	raw    string
	text   string
	eol    string
	indent int
}

func makeLine(no int, raw, term string) line {
	body := str.TrimLeft(raw)
	return line{
		no:     no,
		raw:    raw,
		text:   str.TrimRight(body),
		eol:    term,
		indent: len(raw) - len(body),
	}
}

type commentText struct {
	text  string
	start int
	end   int
}

func (c commentText) col() int {
	return c.start + 1
}

func (ln line) span(text string, off int) commentText {
	start := ln.indent + off
	return commentText{text: text, start: start, end: start + len(text)}
}

func (ln line) isSeparator() bool {
	return strings.HasPrefix(ln.text, "###")
}

func (ln line) hasScriptMarker() bool {
	return strings.HasPrefix(ln.text, ">")
}

func (ln line) isBlockCommentStart() bool {
	return strings.HasPrefix(ln.text, "/*")
}

func (ln line) isComment() bool {
	_, _, ok := stripComment(ln.text)
	return ok
}

func (ln line) comment() (commentText, bool) {
	text, off, ok := stripComment(ln.text)
	if !ok {
		return commentText{}, false
	}
	return ln.span(text, off), true
}

// stripComment strips a leading //, # or -- marker. It returns the comment
// text and its byte offset.
func stripComment(text string) (string, int, bool) {
	var n int
	switch {
	case strings.HasPrefix(text, "//"):
		n = 2
	case strings.HasPrefix(text, "#"):
		n = 1
	case strings.HasPrefix(text, "--"):
		n = 2
	default:
		return "", 0, false
	}
	body := str.TrimLeft(text[n:])
	return str.TrimRight(body), len(text) - len(body), true
}

func (ln line) blockComment(opening bool) (c commentText, closed bool) {
	body := ln.text
	off := 0
	if opening {
		rest := strings.TrimPrefix(body, "/*")
		off, body = len(body)-len(rest), rest
	}
	if end := strings.Index(body, "*/"); end >= 0 {
		body, closed = body[:end], true
	}

	text := str.TrimLeft(body)
	off += len(body) - len(text)
	for strings.HasPrefix(text, "*") {
		rest := str.TrimLeft(text[1:])
		off += len(text) - len(rest)
		text = rest
	}
	return ln.span(str.TrimRight(text), off), closed
}

func (ln line) isScriptBlockStart() bool {
	if !ln.hasScriptMarker() {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(ln.text, ">"))
	return rest == "{%"
}

func (ln line) isScriptBlockEnd() bool {
	text := ln.text
	if after, ok := strings.CutPrefix(text, ">"); ok {
		text = str.TrimLeft(after)
	}
	if !strings.HasPrefix(text, "%}") {
		return false
	}
	rest := strings.TrimPrefix(text, "%}")
	if rest == "" {
		return true
	}
	rest = str.TrimLeft(rest)
	if rest == "" {
		return true
	}
	_, _, ok := stripComment(rest)
	return ok
}

// cutScriptMarker strips a leading ">" script marker and returns the script
// body and its 1-based source column. ok is false when the line has no marker.
func (ln line) cutScriptMarker() (body string, col int, ok bool) {
	s := str.TrimLeft(ln.raw)
	after, ok := strings.CutPrefix(s, ">")
	if !ok {
		return "", 0, false
	}
	col = len(ln.raw) - len(s) + 2
	b := str.TrimLeadingOnce(after)
	col += len(after) - len(b)
	return str.TrimRight(b), col, true
}
