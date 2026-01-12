package curl

import (
	"fmt"
	"strings"
)

type lexState struct {
	token TokenState
	buf   strings.Builder
	out   []string
}

func (st *lexState) add(r rune) {
	st.buf.WriteRune(r)
}

func (st *lexState) flush() {
	if st.buf.Len() == 0 {
		return
	}
	st.out = append(st.out, st.buf.String())
	st.buf.Reset()
}

// Shell-style tokenization with single quotes (literal), double quotes (escape-aware),
// ANSI-C $'...' quoting, and backslash escaping. Single quotes disable escaping.
// Double quotes respect backslashes so you can have \"inside\" strings.
func splitTokens(input string) ([]string, error) {
	st := &lexState{}
	rs := []rune(input)
	opts := tokenOptions{decodeANSI: true, allowLineContinuation: true}

	for i := 0; i < len(rs); i++ {
		r := rs[i]
		step, err := st.token.advance(rs, &i, opts)
		if err != nil {
			return nil, err
		}
		if step.handled {
			if step.emit {
				st.add(step.r)
			}
			continue
		}

		if isWhitespace(r) {
			if st.token.InQuote() {
				st.add(r)
			} else {
				st.flush()
			}
			continue
		}

		st.add(r)
	}

	if st.token.Escaping() {
		return nil, fmt.Errorf("unterminated escape sequence")
	}

	if st.token.Open() {
		return nil, fmt.Errorf("unterminated quoted string")
	}

	st.flush()
	return st.out, nil
}

func ansiEsc(rs []rune, i *int) (rune, error) {
	if *i >= len(rs) {
		return 0, fmt.Errorf("unterminated escape sequence")
	}
	r := rs[*i]
	switch r {
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	case '\\':
		return '\\', nil
	case '\'':
		return '\'', nil
	case '"':
		return '"', nil
	case 'x':
		return readHex(rs, i, 2)
	case 'u':
		return readHex(rs, i, 4)
	default:
		return r, nil
	}
}

func readHex(rs []rune, i *int, n int) (rune, error) {
	if *i+n >= len(rs) {
		return 0, fmt.Errorf("invalid hex escape")
	}
	val := 0
	for j := 0; j < n; j++ {
		r := rs[*i+1+j]
		d, ok := hexVal(r)
		if !ok {
			return 0, fmt.Errorf("invalid hex escape")
		}
		val = val*16 + d
	}
	*i += n
	return rune(val), nil
}

func hexVal(r rune) (int, bool) {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0'), true
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10, true
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10, true
	default:
		return 0, false
	}
}

func isLineBreak(r rune) bool {
	return r == '\n' || r == '\r'
}

func isWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}
