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
	if s.tail != "" {
		src = s.tail + src
		s.tail = ""
	}

	for len(src) > 0 {
		if !utf8.FullRuneInString(src) {
			s.tail = strings.Clone(src)
			return
		}
		r, size := utf8.DecodeRuneInString(src)
		if s.field.feed(r, true) == fieldEnd {
			s.field.reset()
		}
		src = src[size:]
	}
}

func (s *OptionsScanner) Closer() rune {
	if !s.field.written && !s.field.escaped && s.tail == "" {
		return 0
	}
	return s.field.closer()
}

func (s *OptionsScanner) ValueOpen() bool {
	return s.field.state != inKey && s.Closer() != 0
}
