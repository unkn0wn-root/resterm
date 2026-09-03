package directive

import (
	"strings"
	"unicode/utf8"
)

type OptionsScanner struct {
	field fieldScan
	tail  string
}

func (s *OptionsScanner) Feed(src string) {
	s.scan(s.resume(src))
}

// FeedOpen consumes src and returns how many leading bytes belong to a value
// that was already open. It returns zero if no value was open.
func (s *OptionsScanner) FeedOpen(src string) int {
	if !s.valueOpen() {
		s.Feed(src)
		return 0
	}

	rest := s.resume(src)
	for s.valueOpen() && rest != "" {
		n := s.step(rest)
		if n == 0 {
			rest = ""
			break
		}
		rest = rest[n:]
	}
	s.scan(rest)
	return len(src) - len(rest)
}

func (s *OptionsScanner) Closer() rune {
	if !s.field.written && !s.field.escaped && s.tail == "" {
		return 0
	}
	return s.field.closer()
}

// A quoted word with no key also has a closer. Only a key makes it an option value.
func (s *OptionsScanner) valueOpen() bool {
	return s.field.state != inKey && s.Closer() != 0
}

func (s *OptionsScanner) scan(src string) {
	for src != "" {
		n := s.step(src)
		if n == 0 {
			return
		}
		src = src[n:]
	}
}

// step returns zero when src ends with an incomplete UTF-8 encoding.
func (s *OptionsScanner) step(src string) int {
	if !utf8.FullRuneInString(src) {
		s.tail = strings.Clone(src)
		return 0
	}
	r, size := utf8.DecodeRuneInString(src)
	if s.field.feed(r, true) == fieldEnd {
		s.field.reset()
	}
	return size
}

// resume scans bytes saved by the previous call and returns the unscanned part
// of src.
func (s *OptionsScanner) resume(src string) string {
	if s.tail == "" {
		return src
	}

	text := s.tail + src
	s.tail = ""
	for len(text) > len(src) {
		n := s.step(text)
		if n == 0 {
			return ""
		}
		text = text[n:]
	}
	return text
}
