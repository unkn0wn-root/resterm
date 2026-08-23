// Package eol splits text while preserving line endings.
package eol

import (
	"bytes"
	"iter"
	"strings"
)

const (
	LF   = "\n"
	CRLF = "\r\n"
)

// ScanLines is a bufio.SplitFunc that keeps line endings.
func ScanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// Cut separates a trailing LF or CRLF from s.
func Cut(s string) (text, term string) {
	switch {
	case strings.HasSuffix(s, CRLF):
		return s[:len(s)-len(CRLF)], CRLF
	case strings.HasSuffix(s, LF):
		return s[:len(s)-len(LF)], LF
	default:
		return s, ""
	}
}

// Lines yields each line and its ending. Joining each pair reproduces s.
func Lines(s string) iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for s != "" {
			i := strings.IndexByte(s, '\n')
			if i < 0 {
				yield(s, "")
				return
			}
			text, term := Cut(s[:i+1])
			if !yield(text, term) {
				return
			}
			s = s[i+1:]
		}
	}
}
