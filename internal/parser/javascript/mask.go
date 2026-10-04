package js

import (
	"bytes"
	"slices"
	"unicode"
	"unicode/utf8"
)

var regexKeywords = []string{
	"await", "case", "delete", "do", "else", "in", "instanceof",
	"new", "of", "return", "throw", "typeof", "void", "yield",
}

// MaskText replaces strings, comments, regular expressions, and template literal
// text with spaces. Code inside ${...} stays unchanged, as do byte offsets and
// line breaks.
//
// The slash check can mistake division for a regular expression and hide
// code on the rest of that line.
func MaskText(src string) string {
	m := masker{b: []byte(src)}
	m.code(false)
	return string(m.b)
}

type masker struct {
	b []byte
	i int
}

// In ${...}, stop at the matching closing brace.
func (m *masker) code(sub bool) {
	depth := 0
	regex := true
	for m.i < len(m.b) {
		r, size := utf8.DecodeRune(m.b[m.i:])
		switch {
		case r == '/' && m.peek() == '/':
			end := len(m.b)
			if n := bytes.IndexByte(m.b[m.i:], '\n'); n >= 0 {
				end = m.i + n
			}
			m.blankTo(end)
		case r == '/' && m.peek() == '*':
			end := len(m.b)
			if n := bytes.Index(m.b[m.i+2:], []byte("*/")); n >= 0 {
				end = m.i + 2 + n + 2
			}
			m.blankTo(end)
		case r == '/' && regex:
			m.regex()
			regex = false
		case r == '\'' || r == '"':
			m.quoted(byte(r))
			regex = false
		case r == '`':
			m.template()
			regex = false
		case isIdentifierPart(r):
			start := m.i
			for m.i < len(m.b) && isIdentifierPart(r) {
				m.i += size
				r, size = utf8.DecodeRune(m.b[m.i:])
			}
			regex = slices.Contains(regexKeywords, string(m.b[start:m.i]))
		case r == '}' && sub && depth == 0:
			return
		default:
			switch r {
			case '{':
				depth++
			case '}':
				depth = max(depth-1, 0)
			}
			if !unicode.IsSpace(r) {
				regex = r != ')' && r != ']'
			}
			m.i += size
		}
	}
}

func (m *masker) quoted(quote byte) {
	m.blankTo(m.i + 1)
	for m.i < len(m.b) {
		switch m.b[m.i] {
		case '\\':
			m.blankTo(m.i + 2)
		case quote:
			m.blankTo(m.i + 1)
			return
		case '\n':
			return
		default:
			m.blankTo(m.i + 1)
		}
	}
}

func (m *masker) template() {
	m.blankTo(m.i + 1)
	for m.i < len(m.b) {
		switch {
		case m.b[m.i] == '\\':
			m.blankTo(m.i + 2)
		case m.b[m.i] == '`':
			m.blankTo(m.i + 1)
			return
		case m.b[m.i] == '$' && m.peek() == '{':
			m.blankTo(m.i + 2)
			m.code(true)
			m.blankTo(m.i + 1)
		default:
			m.blankTo(m.i + 1)
		}
	}
}

func (m *masker) regex() {
	m.blankTo(m.i + 1)
	class := false
	for m.i < len(m.b) {
		switch c := m.b[m.i]; {
		case c == '\\':
			m.blankTo(m.i + 2)
		case c == '\n':
			return
		case c == '/' && !class:
			m.blankTo(m.i + 1)
			return
		case c == '[' || c == ']':
			class = c == '['
			m.blankTo(m.i + 1)
		default:
			m.blankTo(m.i + 1)
		}
	}
}

func (m *masker) peek() byte {
	if m.i+1 < len(m.b) {
		return m.b[m.i+1]
	}
	return 0
}

func (m *masker) blankTo(end int) {
	for ; m.i < min(end, len(m.b)); m.i++ {
		if m.b[m.i] != '\n' {
			m.b[m.i] = ' '
		}
	}
}
