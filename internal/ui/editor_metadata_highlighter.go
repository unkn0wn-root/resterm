package ui

import (
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
	"name":              metadataValueModeToken,
	"description":       metadataValueModeRest,
	"desc":              metadataValueModeRest,
	"tag":               metadataValueModeRest,
	"auth":              metadataValueModeToken,
	"graphql":           metadataValueModeToken,
	"graphql-operation": metadataValueModeToken,
	"operation":         metadataValueModeToken,
	"variables":         metadataValueModeRest,
	"graphql-variables": metadataValueModeRest,
	"query":             metadataValueModeRest,
	"graphql-query":     metadataValueModeRest,
	"grpc":              metadataValueModeRest,
	"grpc-descriptor":   metadataValueModeRest,
	"grpc-reflection":   metadataValueModeToken,
	"grpc-plaintext":    metadataValueModeToken,
	"grpc-authority":    metadataValueModeRest,
	"grpc-metadata":     metadataValueModeRest,
	"script":            metadataValueModeToken,
	"no-log":            metadataValueModeNone,
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
	palette theme.EditorMetadataPalette
}

func newMetadataRuneStyler(p theme.EditorMetadataPalette) textarea.RuneStyler {
	return &metadataRuneStyler{palette: p}
}

func (s *metadataRuneStyler) StylesForLine(line []rune, _ int) []lipgloss.Style {
	if len(line) == 0 {
		return nil
	}

	i := skipSpace(line, 0)
	if i >= len(line) {
		return nil
	}

	if styles := s.requestLineStyles(line, i); styles != nil {
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

	styles := make([]lipgloss.Style, len(line))

	if color := s.palette.CommentMarker; color != "" {
		markerStyle := lipgloss.NewStyle().Foreground(color)
		for idx := markerStart; idx < markerStart+markerLen && idx < len(line); idx++ {
			styles[idx] = markerStyle
		}
	}

	directiveEnd := directiveStart + 1
	for directiveEnd < len(line) && isDirectiveRune(line[directiveEnd]) {
		directiveEnd++
	}
	directiveKey := strings.ToLower(string(line[directiveStart+1 : directiveEnd]))

	directiveColor := s.palette.DirectiveDefault
	if c, ok := s.palette.DirectiveColors[directiveKey]; ok && c != "" {
		directiveColor = c
	}
	if directiveColor != "" {
		dirStyle := lipgloss.NewStyle().Foreground(directiveColor).Bold(true)
		for idx := directiveStart; idx < directiveEnd; idx++ {
			styles[idx] = dirStyle
		}
	}

	valueStart := skipSpace(line, directiveEnd)
	if valueStart >= len(line) {
		return styles
	}

	switch directiveKey {
	case "setting":
		return s.applySettingStyles(line, styles, valueStart)
	case "timeout":
		return s.applyTimeoutStyles(line, styles, valueStart)
	}

	mode := metadataValueModeToken
	if m, ok := directiveValueModes[directiveKey]; ok {
		mode = m
	}
	if mode == metadataValueModeNone || s.palette.Value == "" {
		return styles
	}

	valueStyle := lipgloss.NewStyle().Foreground(s.palette.Value)
	switch mode {
	case metadataValueModeRest:
		for idx := valueStart; idx < len(line); idx++ {
			styles[idx] = valueStyle
		}
	case metadataValueModeToken:
		tokenEnd := readToken(line, valueStart)
		for idx := valueStart; idx < tokenEnd && idx < len(line); idx++ {
			styles[idx] = valueStyle
		}
	}

	return styles
}

func (s *metadataRuneStyler) applySettingStyles(line []rune, styles []lipgloss.Style, start int) []lipgloss.Style {
	keyEnd := readToken(line, start)
	if keyEnd > start && s.palette.SettingKey != "" {
		keyStyle := lipgloss.NewStyle().Foreground(s.palette.SettingKey).Bold(true)
		for idx := start; idx < keyEnd && idx < len(line); idx++ {
			styles[idx] = keyStyle
		}
	}

	valueStart := skipSpace(line, keyEnd)
	if valueStart >= len(line) || s.palette.SettingValue == "" {
		return styles
	}

	valueStyle := lipgloss.NewStyle().Foreground(s.palette.SettingValue)
	for idx := valueStart; idx < len(line); idx++ {
		styles[idx] = valueStyle
	}
	return styles
}

func (s *metadataRuneStyler) applyTimeoutStyles(line []rune, styles []lipgloss.Style, start int) []lipgloss.Style {
	if s.palette.SettingValue == "" {
		return styles
	}

	valueStyle := lipgloss.NewStyle().Foreground(s.palette.SettingValue)
	for idx := start; idx < len(line); idx++ {
		styles[idx] = valueStyle
	}
	return styles
}

func (s *metadataRuneStyler) requestLineStyles(line []rune, start int) []lipgloss.Style {
	color := s.palette.RequestLine
	if color == "" {
		color = s.palette.DirectiveDefault
	}
	if color == "" {
		return nil
	}

	if !isRequestLine(line, start) {
		return nil
	}

	styles := make([]lipgloss.Style, len(line))
	lineStyle := lipgloss.NewStyle().Foreground(color).Bold(true)
	for idx := start; idx < len(line); idx++ {
		styles[idx] = lineStyle
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
