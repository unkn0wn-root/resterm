package ui

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/intellisense"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

// stylePainter fills styles lazily so a line with nothing painted returns nil.
type stylePainter struct {
	styles []lipgloss.Style
	n      int
}

func (p *stylePainter) paint(from, to int, style lipgloss.Style) {
	if from >= to {
		return
	}
	if p.styles == nil {
		p.styles = make([]lipgloss.Style, p.n)
	}
	for i := from; i < to; i++ {
		p.styles[i] = style
	}
}

type metadataRuneStyler struct {
	palette             theme.EditorMetadataPalette
	commentStyle        lipgloss.Style
	commentEnabled      bool
	directiveStyles     map[string]lipgloss.Style
	valueStyle          lipgloss.Style
	valueEnabled        bool
	settingKeyStyle     lipgloss.Style
	settingKeyEnabled   bool
	settingValueStyle   lipgloss.Style
	settingValueEnabled bool
	requestLineStyle    lipgloss.Style
	requestLineEnabled  bool
	requestSepStyle     lipgloss.Style
	requestSepEnabled   bool
	syntax              parser.SourceSyntax
	cache               map[int]lineCache
}

type lineCache struct {
	hash     uint64
	length   int
	computed bool
	styles   []lipgloss.Style
}

func newMetadataRuneStyler(p theme.EditorMetadataPalette) *metadataRuneStyler {
	s := &metadataRuneStyler{
		palette:         p,
		directiveStyles: make(map[string]lipgloss.Style),
		cache:           make(map[int]lineCache),
	}

	if c := p.CommentMarker; c != "" {
		s.commentStyle = lipgloss.NewStyle().Foreground(c)
		s.commentEnabled = true
	}

	if c := p.Value; c != "" {
		s.valueStyle = lipgloss.NewStyle().Foreground(c)
		s.valueEnabled = true
	}

	// option keys repeat on every directive line, so they stay unbold and let the
	// directive name and the values carry the emphasis
	if c := p.SettingKey; c != "" {
		s.settingKeyStyle = lipgloss.NewStyle().Foreground(c)
		s.settingKeyEnabled = true
	}

	if c := p.SettingValue; c != "" {
		s.settingValueStyle = lipgloss.NewStyle().Foreground(c)
		s.settingValueEnabled = true
	}

	reqColor := p.RequestLine
	if reqColor == "" {
		reqColor = p.DirectiveDefault
	}
	if reqColor != "" {
		s.requestLineStyle = lipgloss.NewStyle().Foreground(reqColor).Bold(true)
		s.requestLineEnabled = true
	}

	if c := p.RequestSeparator; c != "" {
		s.requestSepStyle = lipgloss.NewStyle().Foreground(c)
		s.requestSepEnabled = true
	}

	return s
}

func selectEditorRuneStyler(path string, palette theme.EditorMetadataPalette) textarea.RuneStyler {
	if strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".rts") {
		return newRTSRuneStyler(palette)
	}
	return newMetadataRuneStyler(palette)
}

func (s *metadataRuneStyler) SetSource(source string) {
	s.syntax.Classify(source)
	clear(s.cache)
}

func (s *metadataRuneStyler) StylesForLine(line []rune, idx int) []lipgloss.Style {
	if len(line) == 0 {
		delete(s.cache, idx)
		return nil
	}

	lineHash := hashRunes(line)
	if cached, ok := s.cache[idx]; ok && cached.computed && cached.hash == lineHash &&
		cached.length == len(line) {
		return cached.styles
	}

	styles := s.computeStyles(line, s.syntax.Line(idx))
	s.cache[idx] = lineCache{hash: lineHash, length: len(line), computed: true, styles: styles}
	return styles
}

func (s *metadataRuneStyler) computeStyles(line []rune, syntax parser.SourceLine) []lipgloss.Style {
	start := skipSpace(line, 0)
	if start >= len(line) {
		return nil
	}

	switch syntax.Kind {
	case parser.SourceLineCode:
		return s.requestLineStyles(line, start)
	case parser.SourceLineComment:
		return s.proseStyles(line, start)
	case parser.SourceLineDirective:
		return s.directiveLineStyles(line, start, syntax)
	case parser.SourceLineDirectiveValue:
		return s.directiveValueStyles(line, start, syntax)
	case parser.SourceLineRequestSeparator:
		return s.separatorStyles(line, start)
	default:
		return nil
	}
}

func (s *metadataRuneStyler) proseStyles(line []rune, start int) []lipgloss.Style {
	if !s.commentEnabled {
		return nil
	}
	return fillFrom(line, start, s.commentStyle)
}

func (s *metadataRuneStyler) directiveLineStyles(
	line []rune,
	start int,
	syntax parser.SourceLine,
) []lipgloss.Style {
	p := &stylePainter{n: len(line)}
	nameStart, end, ok := syntax.ContentRange(len(line))
	if s.commentEnabled {
		p.paint(start, nameStart, s.commentStyle)
	}
	if !ok {
		return p.styles
	}

	nameEnd := nameStart + 1
	for nameEnd < end && isDirectiveRune(line[nameEnd]) {
		nameEnd++
	}
	if style, ok := s.directiveStyle(strings.ToLower(string(line[nameStart+1 : nameEnd]))); ok {
		p.paint(nameStart, nameEnd, style)
	}

	s.paintArgument(p, line, syntax.Args, skipArgSep(line, nameEnd, end), end)
	return p.styles
}

func (s *metadataRuneStyler) directiveValueStyles(
	line []rune,
	start int,
	syntax parser.SourceLine,
) []lipgloss.Style {
	p := &stylePainter{n: len(line)}
	valueStart, end, ok := syntax.ContentRange(len(line))
	if s.commentEnabled {
		p.paint(start, valueStart, s.commentStyle)
	}
	if ok {
		optionValueEnd := min(syntax.OptionValueEnd, end)
		if syntax.Args == directive.ArgOptions && optionValueEnd > valueStart {
			if s.valueEnabled {
				p.paint(valueStart, end, s.valueStyle)
			}
			if s.settingValueEnabled {
				p.paint(valueStart, optionValueEnd, s.settingValueStyle)
			}
			s.applyOptionFieldStyles(p, line, optionValueEnd, end)
		} else {
			s.paintArgument(p, line, syntax.Args, valueStart, end)
		}
	}
	return p.styles
}

func (s *metadataRuneStyler) paintArgument(
	p *stylePainter,
	line []rune,
	args directive.ArgKind,
	start, end int,
) {
	if start >= end {
		return
	}

	switch args {
	case directive.ArgNone:
	case directive.ArgSetting:
		s.applySettingStyles(p, line, start, end)
	case directive.ArgOptions:
		s.applyOptionStyles(p, line, start, end)
	case directive.ArgText:
		if s.valueEnabled {
			p.paint(start, end, s.valueStyle)
		}
	case directive.ArgToken:
		if s.valueEnabled {
			p.paint(start, readToken(line, start, end), s.valueStyle)
		}
	}
}

func (s *metadataRuneStyler) directiveStyle(key string) (lipgloss.Style, bool) {
	if style, ok := s.directiveStyles[key]; ok {
		return style, true
	}

	var color lipgloss.Color
	if c, ok := s.palette.DirectiveColors[key]; ok && c != "" {
		color = c
	} else {
		color = s.palette.DirectiveDefault
	}
	if color == "" {
		return lipgloss.Style{}, false
	}
	style := lipgloss.NewStyle().Foreground(color).Bold(true)
	s.directiveStyles[key] = style
	return style, true
}

func (s *metadataRuneStyler) applySettingStyles(
	p *stylePainter,
	line []rune,
	start, end int,
) {
	if !s.settingKeyEnabled && !s.settingValueEnabled {
		return
	}

	// @setting also accepts the @settings key=value form.
	tokEnd := readToken(line, start, end)
	if slices.Contains(line[start:tokEnd], '=') {
		s.applyOptionStyles(p, line, start, end)
		return
	}

	keyEnd := tokEnd
	for keyEnd > start && line[keyEnd-1] == ':' {
		keyEnd--
	}
	if s.settingKeyEnabled {
		p.paint(start, keyEnd, s.settingKeyStyle)
	}
	if s.settingValueEnabled {
		p.paint(skipSpaceTo(line, tokEnd, end), end, s.settingValueStyle)
	}
}

func (s *metadataRuneStyler) applyOptionStyles(
	p *stylePainter,
	line []rune,
	start, end int,
) {
	if s.valueEnabled {
		p.paint(start, end, s.valueStyle)
	}
	s.applyOptionFieldStyles(p, line, start, end)
}

func (s *metadataRuneStyler) applyOptionFieldStyles(
	p *stylePainter,
	line []rune,
	start, end int,
) {
	if !s.settingKeyEnabled && !s.settingValueEnabled {
		return
	}

	// Spans are byte offsets in rest. The cursor turns them into rune indexes.
	rest := string(line[start:end])
	b, r := 0, start
	runeAt := func(off int) int {
		for b < off {
			_, size := utf8.DecodeRuneInString(rest[b:])
			b += size
			r++
		}
		return r
	}
	for _, f := range directive.FieldSpans(rest) {
		if f.Eq < 0 {
			continue
		}
		keyStart, keyEnd := runeAt(f.Start), runeAt(f.Eq)
		if s.settingKeyEnabled {
			p.paint(keyStart, keyEnd, s.settingKeyStyle)
		}
		if s.settingValueEnabled {
			p.paint(keyEnd+1, runeAt(f.End), s.settingValueStyle)
		}
	}
}

func hashRunes(runes []rune) uint64 {
	var h uint64 = 1469598103934665603
	const prime uint64 = 1099511628211
	for _, r := range runes {
		h ^= uint64(r)
		h *= prime
	}
	return h
}

func (s *metadataRuneStyler) requestLineStyles(line []rune, start int) []lipgloss.Style {
	if !s.requestLineEnabled || !isRequestLine(line, start) {
		return nil
	}
	return fillFrom(line, start, s.requestLineStyle)
}

func (s *metadataRuneStyler) separatorStyles(line []rune, start int) []lipgloss.Style {
	if !s.requestSepEnabled {
		return nil
	}
	return fillFrom(line, start, s.requestSepStyle)
}

func fillFrom(line []rune, start int, style lipgloss.Style) []lipgloss.Style {
	p := stylePainter{n: len(line)}
	p.paint(start, len(line), style)
	return p.styles
}

func skipSpace(line []rune, start int) int {
	return skipSpaceTo(line, start, len(line))
}

func skipSpaceTo(line []rune, start, end int) int {
	i := start
	for i < end && unicode.IsSpace(line[i]) {
		i++
	}
	return i
}

// The colon in "@name: value" separates, it is not part of the value, so the
// styling starts past it.
func skipArgSep(line []rune, start, end int) int {
	i := start
	for i < end && directive.IsArgSep(line[i]) {
		i++
	}
	return i
}

func isDirectiveRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.'
}

func readToken(line []rune, start, end int) int {
	i := start
	for i < end && !unicode.IsSpace(line[i]) {
		i++
	}
	return i
}

func isRequestLine(line []rune, start int) bool {
	if start >= len(line) {
		return false
	}

	end := readToken(line, start, len(line))
	if end <= start {
		return false
	}

	return intellisense.IsMethodKeyword(string(line[start:end]))
}
