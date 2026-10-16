package rts

import (
	"bytes"
	"strings"
)

// Mask hides strings, comments, and group contents with spaces.
// Byte offsets stay the same, so callers can find top-level separators.
func Mask(src string) string {
	out := bytes.Repeat([]byte{' '}, len(src))
	lx := newLexerAt([]byte(src), Pos{})
	var depth int
	for {
		tok := lx.scan()
		if tok.K == tokEOF {
			return string(out)
		}
		var keep bool
		switch tok.K {
		case tokLparen, tokLbrack, tokLbrace:
			keep = depth == 0
			depth++
		case tokRparen, tokRbrack, tokRbrace:
			depth = max(depth-1, 0)
			keep = depth == 0
		case tokString, tokIllegal, tokAutoSemi:
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
	lx := newLexerAt([]byte(src), Pos{})
	for {
		tok := lx.scan()
		if tok.K == tokEOF {
			return string(out)
		}
		if tok.K == tokString || tok.K == tokIllegal || tok.K == tokAutoSemi {
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
	lx := newLexerAt([]byte(src), Pos{})
	for {
		tok := lx.scan()
		switch tok.K {
		case tokEOF:
			return out
		case tokIdent:
			if !strings.HasSuffix(path, ".") {
				path = ""
			}
			path += tok.Lit
			continue
		case tokDot:
			// A leading dot keeps f().vars.interpolate from matching vars.interpolate.
			path += "."
			continue
		case tokLparen, tokLbrack, tokLbrace:
			depth++
			if tok.K == tokLparen && skip == 0 && path == callee {
				skip = depth
			}
		case tokRparen, tokRbrack, tokRbrace:
			if depth == skip {
				skip = 0
			}
			depth = max(depth-1, 0)
		case tokString:
			if skip == 0 {
				out = append(out, [2]int{lx.start, lx.i})
			}
		}
		path = ""
	}
}
