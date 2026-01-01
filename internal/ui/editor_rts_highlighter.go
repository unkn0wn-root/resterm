package ui

import (
	"unicode"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

type rtsRuneStyler struct {
	commentStyle          lipgloss.Style
	commentEnabled        bool
	stringStyle           lipgloss.Style
	stringEnabled         bool
	keywordDefaultStyle   lipgloss.Style
	keywordDefaultEnabled bool
	keywordDeclStyle      lipgloss.Style
	keywordDeclEnabled    bool
	keywordControlStyle   lipgloss.Style
	keywordControlEnabled bool
	keywordLiteralStyle   lipgloss.Style
	keywordLiteralEnabled bool
	keywordLogicalStyle   lipgloss.Style
	keywordLogicalEnabled bool
	fnStyle               lipgloss.Style
	fnEnabled             bool
	methStyle             lipgloss.Style
	methEnabled           bool
	numberStyle           lipgloss.Style
	numberEnabled         bool
	cache                 map[int]lineCache
}

func newRTSRuneStyler(p theme.EditorMetadataPalette) textarea.RuneStyler {
	s := &rtsRuneStyler{
		cache: make(map[int]lineCache),
	}

	if c := p.CommentMarker; c != "" {
		s.commentStyle = lipgloss.NewStyle().Foreground(c)
		s.commentEnabled = true
	}
	if c := p.Value; c != "" {
		s.stringStyle = lipgloss.NewStyle().Foreground(c)
		s.stringEnabled = true
	}
	baseKeyword := p.RTSKeywordDefault
	if baseKeyword == "" {
		baseKeyword = p.DirectiveDefault
	}
	if baseKeyword == "" {
		baseKeyword = p.RequestLine
	}
	if baseKeyword != "" {
		s.keywordDefaultStyle = lipgloss.NewStyle().Foreground(baseKeyword).Bold(true)
		s.keywordDefaultEnabled = true
	}
	makeKeywordStyle := func(color lipgloss.Color) (lipgloss.Style, bool) {
		if color == "" {
			return lipgloss.Style{}, false
		}
		return lipgloss.NewStyle().Foreground(color).Bold(true), true
	}
	s.keywordDeclStyle, s.keywordDeclEnabled = makeKeywordStyle(p.RTSKeywordDecl)
	s.keywordControlStyle, s.keywordControlEnabled = makeKeywordStyle(p.RTSKeywordControl)
	s.keywordLiteralStyle, s.keywordLiteralEnabled = makeKeywordStyle(p.RTSKeywordLiteral)
	s.keywordLogicalStyle, s.keywordLogicalEnabled = makeKeywordStyle(p.RTSKeywordLogical)
	if c := p.SettingValue; c != "" {
		s.numberStyle = lipgloss.NewStyle().Foreground(c)
		s.numberEnabled = true
	}

	if c := pickColor(
		p.SettingKey,
		p.RTSKeywordControl,
		p.DirectiveDefault,
		p.RequestLine,
	); c != "" {
		s.fnStyle = lipgloss.NewStyle().Foreground(c).Bold(true)
		s.fnEnabled = true
	}
	if c := pickColor(p.SettingValue, p.Value); c != "" {
		s.methStyle = lipgloss.NewStyle().Foreground(c)
		s.methEnabled = true
	}
	if !s.methEnabled && s.fnEnabled {
		s.methStyle = s.fnStyle
		s.methEnabled = true
	}

	return s
}

func (s *rtsRuneStyler) StylesForLine(line []rune, idx int) []lipgloss.Style {
	if len(line) == 0 {
		delete(s.cache, idx)
		return nil
	}

	lineHash := hashRunes(line)
	if cached, ok := s.cache[idx]; ok && cached.computed && cached.hash == lineHash &&
		cached.length == len(line) {
		return cached.styles
	}

	styles := s.computeStyles(line)
	s.cache[idx] = lineCache{hash: lineHash, length: len(line), computed: true, styles: styles}
	return styles
}

func (s *rtsRuneStyler) computeStyles(line []rune) []lipgloss.Style {
	var (
		styles []lipgloss.Style
		styled bool
	)

	ensureStyles := func() {
		if !styled {
			styles = make([]lipgloss.Style, len(line))
			styled = true
		}
	}
	paint := func(start, end int, style lipgloss.Style) {
		if start < 0 {
			start = 0
		}
		if end > len(line) {
			end = len(line)
		}
		if start >= end {
			return
		}
		ensureStyles()
		for j := start; j < end; j++ {
			styles[j] = style
		}
	}

	var quote rune
	wantFn := false
	for i := 0; i < len(line); {
		ch := line[i]

		if quote != 0 {
			if s.stringEnabled {
				paint(i, i+1, s.stringStyle)
			}
			if ch == '\\' && i+1 < len(line) {
				if s.stringEnabled {
					paint(i+1, i+2, s.stringStyle)
				}
				i += 2
				continue
			}
			if ch == quote {
				quote = 0
			}
			i++
			continue
		}

		if wantFn && !unicode.IsSpace(ch) && !isRTSIdentStart(ch) {
			wantFn = false
		}

		if ch == '"' || ch == '\'' {
			quote = ch
			if s.stringEnabled {
				paint(i, i+1, s.stringStyle)
			}
			i++
			continue
		}

		if ch == '#' {
			if s.commentEnabled {
				paint(i, len(line), s.commentStyle)
			}
			break
		}

		if isRTSIdentStart(ch) {
			start := i
			for i < len(line) && isRTSIdent(line[i]) {
				i++
			}
			token := string(line[start:i])
			class := rts.KeywordClassOf(token)
			if class != rts.KeywordNone {
				if style, ok := s.keywordStyleForClass(class); ok {
					paint(start, i, style)
				}
				if token == "fn" {
					wantFn = true
				} else {
					wantFn = false
				}
				continue
			}
			call := isRTSCall(line, i)
			if wantFn || call {
				style, ok := s.nameStyle(line, start, wantFn)
				if ok {
					paint(start, i, style)
				}
			}
			wantFn = false
			continue
		}

		if unicode.IsDigit(ch) {
			start := i
			for i < len(line) && (unicode.IsDigit(line[i]) || line[i] == '.') {
				i++
			}
			if s.numberEnabled {
				paint(start, i, s.numberStyle)
			}
			continue
		}

		i++
	}

	if !styled {
		return nil
	}
	return styles
}

func (s *rtsRuneStyler) nameStyle(
	line []rune,
	start int,
	isFn bool,
) (lipgloss.Style, bool) {
	if isFn {
		if s.fnEnabled {
			return s.fnStyle, true
		}
		if s.methEnabled {
			return s.methStyle, true
		}
		return lipgloss.Style{}, false
	}
	if isRTSMethod(line, start) {
		if s.methEnabled {
			return s.methStyle, true
		}
		if s.fnEnabled {
			return s.fnStyle, true
		}
		return lipgloss.Style{}, false
	}
	if s.fnEnabled {
		return s.fnStyle, true
	}
	if s.methEnabled {
		return s.methStyle, true
	}
	return lipgloss.Style{}, false
}

func (s *rtsRuneStyler) keywordStyleForClass(class rts.KeywordClass) (lipgloss.Style, bool) {
	switch class {
	case rts.KeywordDecl:
		if s.keywordDeclEnabled {
			return s.keywordDeclStyle, true
		}
	case rts.KeywordControl:
		if s.keywordControlEnabled {
			return s.keywordControlStyle, true
		}
	case rts.KeywordLiteral:
		if s.keywordLiteralEnabled {
			return s.keywordLiteralStyle, true
		}
	case rts.KeywordLogical:
		if s.keywordLogicalEnabled {
			return s.keywordLogicalStyle, true
		}
	case rts.KeywordDefault:
		if s.keywordDefaultEnabled {
			return s.keywordDefaultStyle, true
		}
	}
	if s.keywordDefaultEnabled {
		return s.keywordDefaultStyle, true
	}
	return lipgloss.Style{}, false
}

func isRTSIdentStart(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}

func isRTSIdent(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

func isRTSCall(line []rune, end int) bool {
	i := skipSpace(line, end)
	return i < len(line) && line[i] == '('
}

func isRTSMethod(line []rune, start int) bool {
	i := prevNonSpace(line, start-1)
	return i >= 0 && line[i] == '.'
}

func prevNonSpace(line []rune, idx int) int {
	for i := idx; i >= 0; i-- {
		if !unicode.IsSpace(line[i]) {
			return i
		}
	}
	return -1
}

func pickColor(colors ...lipgloss.Color) lipgloss.Color {
	for _, c := range colors {
		if c != "" {
			return c
		}
	}
	return ""
}
