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

	var quote rune
	for i := 0; i < len(line); {
		ch := line[i]

		if quote != 0 {
			if s.stringEnabled {
				ensureStyles()
				styles[i] = s.stringStyle
			}
			if ch == '\\' && i+1 < len(line) {
				if s.stringEnabled {
					ensureStyles()
					styles[i+1] = s.stringStyle
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

		if ch == '"' || ch == '\'' {
			quote = ch
			if s.stringEnabled {
				ensureStyles()
				styles[i] = s.stringStyle
			}
			i++
			continue
		}

		if ch == '#' {
			if s.commentEnabled {
				ensureStyles()
				for j := i; j < len(line); j++ {
					styles[j] = s.commentStyle
				}
			}
			break
		}

		if isRTSIdentStart(ch) {
			start := i
			for i < len(line) && isRTSIdent(line[i]) {
				i++
			}
			class := rts.KeywordClassOf(string(line[start:i]))
			if class != rts.KeywordNone {
				if style, ok := s.keywordStyleForClass(class); ok {
					ensureStyles()
					for j := start; j < i; j++ {
						styles[j] = style
					}
				}
			}
			continue
		}

		if unicode.IsDigit(ch) {
			start := i
			for i < len(line) && (unicode.IsDigit(line[i]) || line[i] == '.') {
				i++
			}
			if s.numberEnabled {
				ensureStyles()
				for j := start; j < i; j++ {
					styles[j] = s.numberStyle
				}
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
