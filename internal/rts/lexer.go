package rts

import (
	"fmt"
)

type Lexer struct {
	src     []byte
	path    string
	i       int
	line    int
	col     int
	par     int
	brk     int
	last    Kind
	pend    *Tok
	eof     bool
	eofSemi bool
}

func NewLexer(path string, src []byte) *Lexer {
	return NewLexerAt(path, src, Pos{Line: 1, Col: 1})
}

func NewLexerAt(path string, src []byte, pos Pos) *Lexer {
	ln := pos.Line
	if ln <= 0 {
		ln = 1
	}

	cl := pos.Col
	if cl <= 0 {
		cl = 1
	}
	return &Lexer{src: src, path: path, line: ln, col: cl}
}

func (l *Lexer) Next() Tok {
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
				return Tok{K: AUTO_SEMI, P: l.pos()}
			}
			return Tok{K: EOF, P: l.pos()}
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
				l.last = AUTO_SEMI
				return Tok{K: AUTO_SEMI, P: p}
			}
			continue
		}

		if ch == '#' {
			l.skipComment()
			continue
		}

		p := l.pos()

		switch ch {
		case '(':
			l.read()
			l.par++
			return l.emit(LPAREN, "(", p)
		case ')':
			l.read()
			if l.par > 0 {
				l.par--
			}
			return l.emit(RPAREN, ")", p)
		case '[':
			l.read()
			l.brk++
			return l.emit(LBRACK, "[", p)
		case ']':
			l.read()
			if l.brk > 0 {
				l.brk--
			}
			return l.emit(RBRACK, "]", p)
		case '{':
			l.read()
			return l.emit(LBRACE, "{", p)
		case '}':
			l.read()
			return l.emit(RBRACE, "}", p)
		case ',':
			l.read()
			return l.emit(COMMA, ",", p)
		case '.':
			l.read()
			return l.emit(DOT, ".", p)
		case ';':
			l.read()
			return l.emit(SEMI, ";", p)
		case ':':
			l.read()
			return l.emit(COLON, ":", p)
		case '?':
			l.read()
			if l.peek() == '?' {
				l.read()
				return l.emit(COALESCE, "??", p)
			}
			return l.emit(QUESTION, "?", p)
		case '+':
			l.read()
			return l.emit(PLUS, "+", p)
		case '-':
			l.read()
			return l.emit(MINUS, "-", p)
		case '*':
			l.read()
			return l.emit(STAR, "*", p)
		case '/':
			l.read()
			return l.emit(SLASH, "/", p)
		case '%':
			l.read()
			return l.emit(PERCENT, "%", p)
		case '=':
			l.read()
			if l.peek() == '=' {
				l.read()
				return l.emit(EQ, "==", p)
			}
			return l.emit(ASSIGN, "=", p)
		case '!':
			l.read()
			if l.peek() == '=' {
				l.read()
				return l.emit(NE, "!=", p)
			}
			return l.illegal(p, "unexpected '!'")
		case '<':
			l.read()
			if l.peek() == '=' {
				l.read()
				return l.emit(LE, "<=", p)
			}
			return l.emit(LT, "<", p)
		case '>':
			l.read()
			if l.peek() == '=' {
				l.read()
				return l.emit(GE, ">=", p)
			}
			return l.emit(GT, ">", p)
		case '"', '\'':
			str, ok := l.scanString()
			if !ok {
				return l.illegal(p, "unterminated string")
			}
			return l.emit(STRING, str, p)
		}

		if isIdentStart(ch) {
			name := l.scanIdent()
			if k, ok := kw[name]; ok {
				return l.emit(k, name, p)
			}
			return l.emit(IDENT, name, p)
		}

		if isDigit(ch) {
			num := l.scanNumber()
			return l.emit(NUMBER, num, p)
		}

		l.read()
		return l.illegal(p, fmt.Sprintf("unexpected %q", ch))
	}
}

func (l *Lexer) pos() Pos {
	return Pos{Path: l.path, Line: l.line, Col: l.col}
}

func (l *Lexer) emit(k Kind, lit string, p Pos) Tok {
	l.last = k
	return Tok{K: k, Lit: lit, P: p}
}

func (l *Lexer) illegal(p Pos, msg string) Tok {
	l.last = ILLEGAL
	return Tok{K: ILLEGAL, Lit: msg, P: p}
}

func (l *Lexer) peek() byte {
	if l.i >= len(l.src) {
		return 0
	}
	return l.src[l.i]
}

func (l *Lexer) read() byte {
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

func (l *Lexer) skipComment() {
	for {
		ch := l.peek()
		if ch == 0 || ch == '\n' || ch == '\r' {
			return
		}
		l.read()
	}
}

func (l *Lexer) scanIdent() string {
	start := l.i
	l.read()
	for isIdent(l.peek()) {
		l.read()
	}
	return string(l.src[start:l.i])
}

func (l *Lexer) scanNumber() string {
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

func (l *Lexer) scanString() (string, bool) {
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

func semiOk(k Kind) bool {
	switch k {
	case IDENT,
		NUMBER,
		STRING,
		KW_TRUE,
		KW_FALSE,
		KW_NULL,
		RPAREN,
		RBRACK,
		RBRACE,
		KW_RETURN,
		KW_BREAK,
		KW_CONTINUE:
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
