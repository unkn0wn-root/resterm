package rts

import (
	"fmt"
)

// lexer splits RTS code into tokens, one per call to scan.
type lexer struct {
	src     []byte
	file    *File
	i       int
	start   int // byte offset of the current token
	line    int
	col     int
	par     int
	brk     int
	last    tokKind
	pend    *token
	eof     bool
	eofSemi bool
}

// newLexer returns a lexer for the whole text of f.
func newLexer(f *File) *lexer {
	return newLexerAt(f.src, f.Start())
}

// newLexerAt returns a lexer for src, a snippet that starts at the line and
// column of at. Its positions are in at's file.
func newLexerAt(src []byte, at Pos) *lexer {
	ln := at.Line
	if ln <= 0 {
		ln = 1
	}

	cl := at.Col
	if cl <= 0 {
		cl = 1
	}
	return &lexer{src: src, file: at.file, line: ln, col: cl}
}

// scan returns the next token. At the end of the text it inserts a semicolon
// after a token that can end a statement, then returns tokEOF on every call.
func (l *lexer) scan() token {
	if l.pend != nil {
		t := *l.pend
		l.pend = nil
		l.last = t.K
		return t
	}

	for {
		if l.eof {
			if !l.eofSemi && semiOk(l.last) {
				l.eofSemi = true
				return token{K: tokAutoSemi, P: l.pos()}
			}
			return token{K: tokEOF, P: l.pos()}
		}

		ch := l.peek()
		if ch == 0 {
			l.eof = true
			continue
		}

		if isSpace(ch) {
			l.read()
			continue
		}

		if ch == '\n' || ch == '\r' {
			p := l.pos()
			l.read()
			if l.par > 0 || l.brk > 0 {
				continue
			}
			if semiOk(l.last) {
				l.last = tokAutoSemi
				return token{K: tokAutoSemi, P: p}
			}
			continue
		}

		if ch == '#' {
			l.skipComment()
			continue
		}

		p := l.pos()
		l.start = l.i

		switch ch {
		case '(':
			l.read()
			l.par++
			return l.emit(tokLparen, "(", p)
		case ')':
			l.read()
			if l.par > 0 {
				l.par--
			}
			return l.emit(tokRparen, ")", p)
		case '[':
			l.read()
			l.brk++
			return l.emit(tokLbrack, "[", p)
		case ']':
			l.read()
			if l.brk > 0 {
				l.brk--
			}
			return l.emit(tokRbrack, "]", p)
		case '{':
			l.read()
			return l.emit(tokLbrace, "{", p)
		case '}':
			l.read()
			return l.emit(tokRbrace, "}", p)
		case ',':
			l.read()
			return l.emit(tokComma, ",", p)
		case '.':
			l.read()
			return l.emit(tokDot, ".", p)
		case ';':
			l.read()
			return l.emit(tokSemi, ";", p)
		case ':':
			l.read()
			return l.emit(tokColon, ":", p)
		case '?':
			l.read()
			if l.peek() == '?' {
				l.read()
				return l.emit(tokCoalesce, "??", p)
			}
			return l.emit(tokQuestion, "?", p)
		case '+':
			l.read()
			return l.emit(tokPlus, "+", p)
		case '-':
			l.read()
			return l.emit(tokMinus, "-", p)
		case '*':
			l.read()
			return l.emit(tokStar, "*", p)
		case '/':
			l.read()
			return l.emit(tokSlash, "/", p)
		case '%':
			l.read()
			return l.emit(tokPercent, "%", p)
		case '=':
			l.read()
			if l.peek() == '=' {
				l.read()
				return l.emit(tokEq, "==", p)
			}
			return l.emit(tokAssign, "=", p)
		case '!':
			l.read()
			if l.peek() == '=' {
				l.read()
				return l.emit(tokNe, "!=", p)
			}
			return l.emit(tokBang, "!", p)
		// there are no bitwise operators, so a lone & or | is a mistyped && or ||
		case '&':
			l.read()
			if l.peek() == '&' {
				l.read()
				return l.emit(tokAndAnd, "&&", p)
			}
			return l.illegal(p, "unexpected '&'")
		case '|':
			l.read()
			if l.peek() == '|' {
				l.read()
				return l.emit(tokOrOr, "||", p)
			}
			return l.illegal(p, "unexpected '|'")
		case '<':
			l.read()
			if l.peek() == '=' {
				l.read()
				return l.emit(tokLe, "<=", p)
			}
			return l.emit(tokLt, "<", p)
		case '>':
			l.read()
			if l.peek() == '=' {
				l.read()
				return l.emit(tokGe, ">=", p)
			}
			return l.emit(tokGt, ">", p)
		case '"', '\'':
			str, ok := l.scanString()
			if !ok {
				return l.illegal(p, "unterminated string")
			}
			return l.emit(tokString, str, p)
		}

		if isIdentStart(ch) {
			name := l.scanIdent()
			if k, ok := kw[name]; ok {
				return l.emit(k, name, p)
			}
			return l.emit(tokIdent, name, p)
		}

		if isDigit(ch) {
			num := l.scanNumber()
			return l.emit(tokNumber, num, p)
		}

		l.read()
		return l.illegal(p, fmt.Sprintf("unexpected %q", ch))
	}
}

func (l *lexer) pos() Pos {
	return Pos{file: l.file, Line: l.line, Col: l.col}
}

func (l *lexer) emit(k tokKind, lit string, p Pos) token {
	l.last = k
	return token{K: k, Lit: lit, P: p}
}

func (l *lexer) illegal(p Pos, msg string) token {
	l.last = tokIllegal
	return token{K: tokIllegal, Lit: msg, P: p}
}

func (l *lexer) peek() byte {
	if l.i >= len(l.src) {
		return 0
	}
	return l.src[l.i]
}

func (l *lexer) read() byte {
	if l.i >= len(l.src) {
		return 0
	}
	b := l.src[l.i]
	l.i++
	if b == '\r' {
		if l.i < len(l.src) && l.src[l.i] == '\n' {
			l.i++
		}
		l.line++
		l.col = 1
		return '\n'
	}
	if b == '\n' {
		l.line++
		l.col = 1
		return '\n'
	}
	l.col++
	return b
}

func (l *lexer) skipComment() {
	for {
		ch := l.peek()
		if ch == 0 || ch == '\n' || ch == '\r' {
			return
		}
		l.read()
	}
}

func (l *lexer) scanIdent() string {
	start := l.i
	l.read()
	for isIdent(l.peek()) {
		l.read()
	}
	return string(l.src[start:l.i])
}

func (l *lexer) scanNumber() string {
	start := l.i
	l.read()
	for isDigit(l.peek()) {
		l.read()
	}
	if l.peek() == '.' {
		next := byte(0)
		if l.i+1 < len(l.src) {
			next = l.src[l.i+1]
		}
		if isDigit(next) {
			l.read()
			for isDigit(l.peek()) {
				l.read()
			}
		}
	}
	return string(l.src[start:l.i])
}

func (l *lexer) scanString() (string, bool) {
	q := l.read()
	start := l.i
	var out []byte
	for {
		ch := l.peek()
		if ch == 0 || ch == '\n' || ch == '\r' {
			return "", false
		}
		if ch == q {
			l.read()
			break
		}
		if ch == '\\' {
			out = append(out, l.src[start:l.i]...)
			l.read()
			esc := l.read()
			switch esc {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case '\\':
				out = append(out, '\\')
			case '\'':
				out = append(out, '\'')
			case '"':
				out = append(out, '"')
			default:
				out = append(out, esc)
			}
			start = l.i
			continue
		}
		l.read()
	}
	if len(out) == 0 {
		return string(l.src[start : l.i-1]), true
	}
	out = append(out, l.src[start:l.i-1]...)
	return string(out), true
}

func semiOk(k tokKind) bool {
	switch k {
	case tokIdent,
		tokNumber,
		tokString,
		tokTrue,
		tokFalse,
		tokNull,
		tokRparen,
		tokRbrack,
		tokRbrace,
		tokReturn,
		tokBreak,
		tokContinue:
		return true
	default:
		return false
	}
}

func isSpace(ch byte) bool {
	return ch == ' ' || ch == '\t'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdent(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}
