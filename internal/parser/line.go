package parser

import (
	"strings"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

// line is one source line. Handlers read text, which is raw with the
// surrounding whitespace already trimmed off.
type line struct {
	no   int // 1-based
	raw  string
	text string
}

func makeLine(no int, raw string) line {
	return line{no: no, raw: raw, text: strings.TrimSpace(raw)}
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

// stripComment strips a leading //, # or -- marker. It returns the comment
// text, the 1-based column of that text inside the input and whether the
// input was a comment at all.
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
	body := text[n:]
	lead := len(body) - len(strings.TrimLeft(body, " \t"))
	return strings.TrimSpace(body), n + lead + 1, true
}

// cutBlockCommentStart strips the opening "/*" and parses the remainder as a
// block comment line.
func cutBlockCommentStart(text string) (string, bool) {
	return parseBlockCommentLine(strings.TrimPrefix(text, "/*"))
}

func parseBlockCommentLine(text string) (string, bool) {
	working := text
	closed := false
	if idx := strings.Index(working, "*/"); idx >= 0 {
		closed = true
		working = working[:idx]
	}

	working = strings.TrimSpace(working)
	for strings.HasPrefix(working, "*") {
		working = strings.TrimSpace(strings.TrimPrefix(working, "*"))
	}
	return working, closed
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
