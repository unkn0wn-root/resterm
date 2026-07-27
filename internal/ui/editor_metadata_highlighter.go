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
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

// An unknown name still paints a token value. The zero Spec.Args is ArgNone,
// which would hide the value, so the fallback has to be spelled out.
func directiveArgKind(name directive.Name) directive.ArgKind {
	spec, ok := directive.Lookup(name)
	if !ok {
		return directive.ArgToken
	}
	return spec.Args
}

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
	cache               map[int]lineCache
}

type lineCache struct {
	hash     uint64
	length   int
	computed bool
	styles   []lipgloss.Style
}

func newMetadataRuneStyler(p theme.EditorMetadataPalette) textarea.RuneStyler {
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

	if c := p.SettingKey; c != "" {
		s.settingKeyStyle = lipgloss.NewStyle().Foreground(c).Bold(true)
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

	styles := s.computeStyles(line)
	s.cache[idx] = lineCache{hash: lineHash, length: len(line), computed: true, styles: styles}
	return styles
}

func (s *metadataRuneStyler) computeStyles(line []rune) []lipgloss.Style {
	i := skipSpace(line, 0)
	if i >= len(line) {
		return nil
	}

	if styles := s.requestLineStyles(line, i); styles != nil {
		return styles
	}

	if styles := s.requestSeparatorStyles(line, i); styles != nil {
		return styles
	}

	markerStart := i
	markerLen := commentMarkerLength(line, i)
	if markerLen == 0 {
		return nil
	}

	directiveStart := skipSpace(line, markerStart+markerLen)
	if directiveStart >= len(line) || line[directiveStart] != '@' {
		return nil
	}

	p := &stylePainter{n: len(line)}
	if s.commentEnabled {
		p.paint(markerStart, markerStart+markerLen, s.commentStyle)
	}

	directiveEnd := directiveStart + 1
	for directiveEnd < len(line) && isDirectiveRune(line[directiveEnd]) {
		directiveEnd++
	}
	directiveKey := strings.ToLower(string(line[directiveStart+1 : directiveEnd]))

	if dirStyle, ok := s.directiveStyle(directiveKey); ok {
		p.paint(directiveStart, directiveEnd, dirStyle)
	}

	valueStart := skipArgSep(line, directiveEnd)
	if valueStart >= len(line) {
		return p.styles
	}

	switch directiveArgKind(directive.Name(directiveKey)) {
	case directive.ArgNone:
	case directive.ArgSetting:
		s.applySettingStyles(p, line, valueStart)
	case directive.ArgOptions:
		s.applyOptionStyles(p, line, valueStart)
	case directive.ArgText:
		if s.valueEnabled {
			p.paint(valueStart, len(line), s.valueStyle)
		}
	case directive.ArgToken:
		if s.valueEnabled {
			p.paint(valueStart, readToken(line, valueStart), s.valueStyle)
		}
	}
	return p.styles
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

func (s *metadataRuneStyler) applySettingStyles(p *stylePainter, line []rune, start int) {
	if !s.settingKeyEnabled && !s.settingValueEnabled {
		return
	}

	// The @settings spelling. putSetting reads it with ParseOptions, so it gets
	// painted the way @settings is.
	tokEnd := readToken(line, start)
	if slices.Contains(line[start:tokEnd], '=') {
		s.applyOptionStyles(p, line, start)
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
		p.paint(skipSpace(line, tokEnd), len(line), s.settingValueStyle)
	}
}

func (s *metadataRuneStyler) applyOptionStyles(p *stylePainter, line []rune, start int) {
	if s.valueEnabled {
		p.paint(start, len(line), s.valueStyle)
	}
	if !s.settingKeyEnabled && !s.settingValueEnabled {
		return
	}

	// Spans are byte offsets in rest. The cursor turns them into rune indexes.
	rest := string(line[start:])
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
	if !s.requestLineEnabled {
		return nil
	}

	if !isRequestLine(line, start) {
		return nil
	}

	styles := make([]lipgloss.Style, len(line))
	for idx := start; idx < len(line); idx++ {
		styles[idx] = s.requestLineStyle
	}
	return styles
}

func (s *metadataRuneStyler) requestSeparatorStyles(line []rune, start int) []lipgloss.Style {
	if !s.requestSepEnabled {
		return nil
	}

	if !hasRequestSeparatorPrefix(line, start) {
		return nil
	}

	styles := make([]lipgloss.Style, len(line))
	for idx := start; idx < len(line); idx++ {
		styles[idx] = s.requestSepStyle
	}
	return styles
}

func skipSpace(line []rune, start int) int {
	i := start
	for i < len(line) && unicode.IsSpace(line[i]) {
		i++
	}
	return i
}

// The colon in "@name: value" separates, it is not part of the value, so the
// styling starts past it.
func skipArgSep(line []rune, start int) int {
	i := start
	for i < len(line) && directive.IsArgSep(line[i]) {
		i++
	}
	return i
}

func hasRequestSeparatorPrefix(line []rune, start int) bool {
	if len(line)-start < 3 {
		return false
	}
	if string(line[start:start+3]) != "###" {
		return false
	}
	if len(line) == start+3 {
		return true
	}
	return unicode.IsSpace(line[start+3])
}

func commentMarkerLength(line []rune, idx int) int {
	switch {
	case line[idx] == '#':
		return 1
	case line[idx] == '/' && idx+1 < len(line) && line[idx+1] == '/':
		return 2
	default:
		return 0
	}
}

func isDirectiveRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.'
}

func readToken(line []rune, start int) int {
	i := start
	for i < len(line) && !unicode.IsSpace(line[i]) {
		i++
	}
	return i
}

func isRequestLine(line []rune, start int) bool {
	if start >= len(line) {
		return false
	}

	end := readToken(line, start)
	if end <= start {
		return false
	}

	return intellisense.IsMethodKeyword(string(line[start:end]))
}
