package rts

import "bytes"

// Mask replaces strings, comments, and nested groups with spaces while keeping
// byte offsets intact. Top-level tokens remain so callers can find separators.
func Mask(src string) string {
	out := bytes.Repeat([]byte{' '}, len(src))
	lx := NewLexer("", []byte(src))
	depth := 0
	for {
		tok := lx.Next()
		if tok.K == EOF {
			return string(out)
		}
		var keep bool
		switch tok.K {
		case LPAREN, LBRACK, LBRACE:
			keep = depth == 0
			depth++
		case RPAREN, RBRACK, RBRACE:
			depth = max(depth-1, 0)
			keep = depth == 0
		case STRING, ILLEGAL, AUTO_SEMI:
			continue
		default:
			keep = depth == 0
		}
		if keep {
			copy(out[lx.start:lx.i], src[lx.start:lx.i])
		}
	}
}

// OpenGroup returns the delimiter for the innermost unclosed group in src.
// Groups inside strings and comments are ignored.
// A mismatched closer does not close a group.
func OpenGroup(src string) (closer rune) {
	lx := NewLexer("", []byte(src))
	var open []rune
	for {
		switch lx.Next().K {
		case EOF:
			if len(open) == 0 {
				return 0
			}
			return open[len(open)-1]
		case LPAREN:
			open = append(open, ')')
		case LBRACK:
			open = append(open, ']')
		case LBRACE:
			open = append(open, '}')
		case RPAREN:
			open = popGroup(open, ')')
		case RBRACK:
			open = popGroup(open, ']')
		case RBRACE:
			open = popGroup(open, '}')
		}
	}
}

func popGroup(open []rune, closer rune) []rune {
	if n := len(open); n > 0 && open[n-1] == closer {
		return open[:n-1]
	}
	return open
}

type groupScanMode uint8

const (
	groupScanCode groupScanMode = iota
	groupScanString
	groupScanComment
)

type GroupScanner struct {
	// open follows OpenGroup, while depth follows Mask. Mismatched closers affect only depth.
	open    []rune
	depth   int
	mode    groupScanMode
	quote   byte
	escaped bool
	skipLF  bool
	stopped bool
}

func (g *GroupScanner) Closer() rune {
	if len(g.open) == 0 {
		return 0
	}
	return g.open[len(g.open)-1]
}

func (g *GroupScanner) Feed(src string) {
	for i := range len(src) {
		g.ScanByte(src[i])
	}
}

// ScanByte returns bytes visible at the top level, using spaces for hidden text.
// Nested bytes return ok=false. NUL ends the scan and CRLF counts as one newline.
func (g *GroupScanner) ScanByte(ch byte) (top byte, ok bool) {
	if g.stopped {
		return 0, false
	}
	if ch == 0 {
		if g.mode == groupScanString && g.escaped {
			g.escaped = false
			return 0, false
		}
		g.stopped = true
		return 0, false
	}
	if g.skipLF {
		g.skipLF = false
		if ch == '\n' {
			return 0, false
		}
	}

	switch g.mode {
	case groupScanString:
		return g.stringByte(ch)
	case groupScanComment:
		return g.commentByte(ch)
	default:
		return g.codeByte(ch)
	}
}

func (g *GroupScanner) codeByte(ch byte) (byte, bool) {
	switch ch {
	case '\n', '\r':
		return g.endLine(ch)
	case '#':
		g.mode = groupScanComment
		return g.hiddenTopLevel()
	case '"', '\'':
		g.mode = groupScanString
		g.quote = ch
		return g.hiddenTopLevel()
	case '(':
		return g.openGroup(ch, ')')
	case '[':
		return g.openGroup(ch, ']')
	case '{':
		return g.openGroup(ch, '}')
	case ')', ']', '}':
		return g.closeGroup(ch)
	}
	if g.depth != 0 {
		return 0, false
	}
	if isTopLevelTokenByte(ch) {
		return ch, true
	}
	return ' ', true
}

func (g *GroupScanner) commentByte(ch byte) (byte, bool) {
	if ch != '\n' && ch != '\r' {
		return 0, false
	}
	return g.endLine(ch)
}

func (g *GroupScanner) stringByte(ch byte) (byte, bool) {
	if g.escaped {
		g.escaped = false
		if ch == '\r' {
			g.skipLF = true
		}
		return 0, false
	}
	switch ch {
	case '\n', '\r':
		return g.endLine(ch)
	case '\\':
		g.escaped = true
	case g.quote:
		g.mode = groupScanCode
		g.quote = 0
	}
	return 0, false
}

// An unterminated string ends with its line, the way the lexer reads it.
func (g *GroupScanner) endLine(ch byte) (byte, bool) {
	g.mode = groupScanCode
	g.quote = 0
	if ch == '\r' {
		g.skipLF = true
	}
	return g.hiddenTopLevel()
}

func (g *GroupScanner) hiddenTopLevel() (byte, bool) {
	if g.depth == 0 {
		return ' ', true
	}
	return 0, false
}

func (g *GroupScanner) openGroup(ch byte, closer rune) (byte, bool) {
	top := g.depth == 0
	g.depth++
	g.open = append(g.open, closer)
	return ch, top
}

func (g *GroupScanner) closeGroup(ch byte) (byte, bool) {
	if g.depth > 0 {
		g.depth--
	}
	g.open = popGroup(g.open, rune(ch))
	return ch, g.depth == 0
}

func isTopLevelTokenByte(ch byte) bool {
	switch ch {
	case ',', '.', ';', ':', '?', '+', '-', '*', '/', '%', '=', '!', '&', '|', '<', '>':
		return true
	}
	return isIdent(ch) || isSpace(ch)
}
