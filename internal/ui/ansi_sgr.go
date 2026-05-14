package ui

import (
	"strconv"
	"strings"
)

const (
	sgrExtForeground = 38
	sgrExtBackground = 48
	sgrExtUnderline  = 58
	sgrExtPalette    = 5
	sgrExtRGB        = 2
)

type sgrState struct {
	bold          bool
	faint         bool
	italic        bool
	underline     bool
	blink         bool
	reverse       bool
	strikethrough bool
	fg            []int
	bg            []int
	ul            []int
}

func isSGR(seq string) bool {
	_, ok := parseSGRParams(seq)
	return ok
}

func parseSGRParams(seq string) ([]int, bool) {
	if len(seq) < 3 || !strings.HasPrefix(seq, "\x1b[") || seq[len(seq)-1] != 'm' {
		return nil, false
	}
	params := seq[2 : len(seq)-1]
	if params == "" {
		return []int{0}, true
	}

	parts := strings.Split(params, ";")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			out = append(out, 0)
			continue
		}
		if !asciiDigits(part) {
			return nil, false
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

func asciiDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func (s *sgrState) apply(seq string) bool {
	params, ok := parseSGRParams(seq)
	if !ok {
		return false
	}
	s.applyParams(params)
	return true
}

func (s *sgrState) applyParams(params []int) {
	for i := 0; i < len(params); {
		p := params[i]
		switch {
		case p == 0:
			*s = sgrState{}
		case p == 1:
			s.bold = true
		case p == 2:
			s.faint = true
		case p == 3:
			s.italic = true
		case p == 4:
			s.underline = true
		case p == 5 || p == 6:
			s.blink = true
		case p == 7:
			s.reverse = true
		case p == 9:
			s.strikethrough = true
		case p == 22:
			s.bold = false
			s.faint = false
		case p == 23:
			s.italic = false
		case p == 24:
			s.underline = false
		case p == 25:
			s.blink = false
		case p == 27:
			s.reverse = false
		case p == 29:
			s.strikethrough = false
		case p >= 30 && p <= 37 || p >= 90 && p <= 97:
			s.fg = []int{p}
		case p == 39:
			s.fg = nil
		case p >= 40 && p <= 47 || p >= 100 && p <= 107:
			s.bg = []int{p}
		case p == 49:
			s.bg = nil
		case p == 59:
			s.ul = nil
		case p == sgrExtForeground || p == sgrExtBackground || p == sgrExtUnderline:
			i += s.applyExtColor(params[i:])
			continue
		}
		i++
	}
}

func (s *sgrState) applyExtColor(params []int) int {
	if len(params) < 2 {
		return 1
	}

	target := &s.fg
	switch params[0] {
	case sgrExtBackground:
		target = &s.bg
	case sgrExtUnderline:
		target = &s.ul
	}

	switch params[1] {
	case sgrExtPalette:
		if len(params) >= 3 {
			*target = append((*target)[:0], params[:3]...)
			return 3
		}
		return len(params)
	case sgrExtRGB:
		if len(params) >= 5 {
			*target = append((*target)[:0], params[:5]...)
			return 5
		}
		return len(params)
	default:
		return 2
	}
}

func (s sgrState) String() string {
	params := make([]int, 0, 16)
	if s.bold {
		params = append(params, 1)
	}
	if s.faint {
		params = append(params, 2)
	}
	if s.italic {
		params = append(params, 3)
	}
	if s.underline {
		params = append(params, 4)
	}
	if s.blink {
		params = append(params, 5)
	}
	if s.reverse {
		params = append(params, 7)
	}
	if s.strikethrough {
		params = append(params, 9)
	}
	params = append(params, s.fg...)
	params = append(params, s.bg...)
	params = append(params, s.ul...)
	if len(params) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\x1b[")
	for i, p := range params {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(strconv.Itoa(p))
	}
	b.WriteByte('m')
	return b.String()
}
