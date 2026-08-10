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
		switch tok := lx.Next(); tok.K {
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
			open = closeGroup(open, ')')
		case RBRACK:
			open = closeGroup(open, ']')
		case RBRACE:
			open = closeGroup(open, '}')
		}
	}
}

func closeGroup(open []rune, closer rune) []rune {
	if n := len(open); n > 0 && open[n-1] == closer {
		return open[:n-1]
	}
	return open
}
