package rts

import (
	"bytes"
	"strings"
)

// Mask hides strings, comments, and group contents with spaces.
// Byte offsets stay the same, so callers can find top-level separators.
func Mask(src string) string {
	out := bytes.Repeat([]byte{' '}, len(src))
	lx := NewLexer("", []byte(src))
	var depth int
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

// MaskText hides strings and comments with spaces, including line breaks.
// It keeps code inside groups and preserves byte offsets.
func MaskText(src string) string {
	out := bytes.Repeat([]byte{' '}, len(src))
	lx := NewLexer("", []byte(src))
	for {
		tok := lx.Next()
		if tok.K == EOF {
			return string(out)
		}
		if tok.K == STRING || tok.K == ILLEGAL || tok.K == AUTO_SEMI {
			continue
		}
		copy(out[lx.start:lx.i], src[lx.start:lx.i])
	}
}

// StringRanges returns byte ranges for string literals in src.
// It skips strings inside calls to callee, including nested arguments.
func StringRanges(src, callee string) [][2]int {
	var (
		out   [][2]int
		path  string
		depth int
		skip  int // call depth, or zero outside the call
	)
	lx := NewLexer("", []byte(src))
	for {
		tok := lx.Next()
		switch tok.K {
		case EOF:
			return out
		case IDENT:
			if !strings.HasSuffix(path, ".") {
				path = ""
			}
			path += tok.Lit
			continue
		case DOT:
			// A leading dot keeps f().vars.interpolate from matching vars.interpolate.
			path += "."
			continue
		case LPAREN, LBRACK, LBRACE:
			depth++
			if tok.K == LPAREN && skip == 0 && path == callee {
				skip = depth
			}
		case RPAREN, RBRACK, RBRACE:
			if depth == skip {
				skip = 0
			}
			depth = max(depth-1, 0)
		case STRING:
			if skip == 0 {
				out = append(out, [2]int{lx.start, lx.i})
			}
		}
		path = ""
	}
}
