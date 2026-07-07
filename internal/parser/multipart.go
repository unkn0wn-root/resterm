package parser

import "strings"

// multipartSpan tracks the region between the first multipart delimiter and
// the close delimiter. Lines inside it are body content and must bypass the
// comment, script, and variable handlers, which would otherwise consume them
// (the "--" comment marker eats boundary lines, "#" eats part content, ...).
type multipartSpan struct {
	delimiter string // "--" + boundary; empty when Content-Type has no boundary param
	open      bool
	closed    bool
}

func newMultipartSpan(ct string) *multipartSpan {
	return &multipartSpan{delimiter: "--" + boundaryParam(ct)}
}

// bodyLine reports whether trimmed is multipart body content: a delimiter
// line, or any line between the first delimiter and the close delimiter.
// Without a known boundary only "--" lines count, so comment-like part
// content is not protected but boundary lines still survive.
func (s *multipartSpan) bodyLine(trimmed string) bool {
	if s.delimiter == "--" {
		return strings.HasPrefix(trimmed, "--")
	}
	switch {
	case s.closed:
		return false
	case trimmed == s.delimiter+"--":
		s.closed = true
		return true
	case trimmed == s.delimiter:
		s.open = true
		return true
	default:
		return s.open
	}
}

func boundaryParam(ct string) string {
	params := strings.Split(ct, ";")
	for _, p := range params[1:] {
		k, v, ok := strings.Cut(p, "=")
		if ok && strings.EqualFold(strings.TrimSpace(k), "boundary") {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return ""
}
