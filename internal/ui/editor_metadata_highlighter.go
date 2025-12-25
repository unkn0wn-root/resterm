package ui

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

type metadataValueMode int

const (
	metadataValueModeNone metadataValueMode = iota
	metadataValueModeToken
	metadataValueModeRest
)

var directiveValueModes = map[string]metadataValueMode{
	"name":                  metadataValueModeToken,
	"description":           metadataValueModeRest,
	"desc":                  metadataValueModeRest,
	"tag":                   metadataValueModeRest,
	"auth":                  metadataValueModeToken,
	"graphql":               metadataValueModeToken,
	"graphql-operation":     metadataValueModeToken,
	"operation":             metadataValueModeToken,
	"variables":             metadataValueModeRest,
	"graphql-variables":     metadataValueModeRest,
	"query":                 metadataValueModeRest,
	"graphql-query":         metadataValueModeRest,
	"grpc":                  metadataValueModeRest,
	"grpc-descriptor":       metadataValueModeRest,
	"grpc-reflection":       metadataValueModeToken,
	"grpc-plaintext":        metadataValueModeToken,
	"grpc-authority":        metadataValueModeRest,
	"grpc-metadata":         metadataValueModeRest,
	"script":                metadataValueModeToken,
	"use":                   metadataValueModeRest,
	"when":                  metadataValueModeRest,
	"skip-if":               metadataValueModeRest,
	"assert":                metadataValueModeRest,
	"for-each":              metadataValueModeRest,
	"switch":                metadataValueModeRest,
	"case":                  metadataValueModeRest,
	"default":               metadataValueModeRest,
	"if":                    metadataValueModeRest,
	"elif":                  metadataValueModeRest,
	"else":                  metadataValueModeRest,
	"no-log":                metadataValueModeNone,
	"log-sensitive-headers": metadataValueModeToken,
	"log-secret-headers":    metadataValueModeToken,
}

var httpRequestMethods = map[string]struct{}{
	"GET":     {},
	"POST":    {},
	"PUT":     {},
	"PATCH":   {},
	"DELETE":  {},
	"HEAD":    {},
	"OPTIONS": {},
	"TRACE":   {},
	"CONNECT": {},
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

	if s.commentEnabled {
		ensureStyles()
		for idx := markerStart; idx < markerStart+markerLen && idx < len(line); idx++ {
			styles[idx] = s.commentStyle
		}
	}

	directiveEnd := directiveStart + 1
	for directiveEnd < len(line) && isDirectiveRune(line[directiveEnd]) {
		directiveEnd++
	}
	directiveKey := strings.ToLower(string(line[directiveStart+1 : directiveEnd]))

	if dirStyle, ok := s.directiveStyle(directiveKey); ok {
		ensureStyles()
		for idx := directiveStart; idx < directiveEnd; idx++ {
			styles[idx] = dirStyle
		}
	}

	valueStart := skipSpace(line, directiveEnd)
	if valueStart >= len(line) {
		if styled {
			return styles
		}
		return nil
	}

	switch directiveKey {
	case "setting":
		s.applySettingStyles(line, &styles, &styled, valueStart)
		if styled {
			return styles
		}
		return nil
	case "timeout":
		s.applyTimeoutStyles(line, &styles, &styled, valueStart)
		if styled {
			return styles
		}
		return nil
	}

	mode := metadataValueModeToken
	if m, ok := directiveValueModes[directiveKey]; ok {
		mode = m
	}
	if mode == metadataValueModeNone || !s.valueEnabled {
		if styled {
			return styles
		}
		return nil
	}

	ensureStyles()
	switch mode {
	case metadataValueModeRest:
		for idx := valueStart; idx < len(line); idx++ {
			styles[idx] = s.valueStyle
		}
	case metadataValueModeToken:
		tokenEnd := readToken(line, valueStart)
		for idx := valueStart; idx < tokenEnd && idx < len(line); idx++ {
			styles[idx] = s.valueStyle
		}
	}

	return styles
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
	line []rune,
	styles *[]lipgloss.Style,
	styled *bool,
	start int,
) {
	if !s.settingKeyEnabled && !s.settingValueEnabled {
		return
	}

	ensure := func() {
		if !*styled {
			*styles = make([]lipgloss.Style, len(line))
			*styled = true
		}
	}

	keyEnd := readToken(line, start)
	if keyEnd > start && s.settingKeyEnabled {
		ensure()
		for idx := start; idx < keyEnd && idx < len(line); idx++ {
			(*styles)[idx] = s.settingKeyStyle
		}
	}

	valueStart := skipSpace(line, keyEnd)
	if valueStart >= len(line) || !s.settingValueEnabled {
		return
	}

	ensure()
	for idx := valueStart; idx < len(line); idx++ {
		(*styles)[idx] = s.settingValueStyle
	}
}

func (s *metadataRuneStyler) applyTimeoutStyles(
	line []rune,
	styles *[]lipgloss.Style,
	styled *bool,
	start int,
) {
	if !s.settingValueEnabled {
		return
	}
	if !*styled {
		*styles = make([]lipgloss.Style, len(line))
		*styled = true
	}
	for idx := start; idx < len(line); idx++ {
		(*styles)[idx] = s.settingValueStyle
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

	token := strings.ToUpper(string(line[start:end]))
	if token == "GRPC" {
		return true
	}

	_, ok := httpRequestMethods[token]
	return ok
}
