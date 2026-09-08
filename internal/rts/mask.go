package rts

import "bytes"

// Mask hides strings, comments, and group contents with spaces.
// Byte offsets stay the same, so callers can find top-level separators.
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
