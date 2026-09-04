package ui

import (
	"context"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func prettifyBody(body []byte, contentType string) string {
	return bodyfmt.Prettify(
		context.Background(),
		body,
		contentType,
		bodyfmt.PrettyOptions{Color: termcolor.TrueColor()},
	)
}

func oneLine(value string) string {
	return displayText(strings.Join(strings.Fields(value), " "))
}

// displayText escapes characters that can affect terminal output.
func displayText(value string) string {
	if utf8.ValidString(value) && !strings.ContainsFunc(value, unprintableRune) {
		return value
	}

	var b strings.Builder
	b.Grow(len(value))
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		part := value[:size]
		// Quote one rune at a time to preserve literal quotes and backslashes.
		if (r == utf8.RuneError && size == 1) || unprintableRune(r) {
			quoted := strconv.QuoteToGraphic(part)
			b.WriteString(quoted[1 : len(quoted)-1])
		} else {
			b.WriteString(part)
		}
		value = value[size:]
	}
	return b.String()
}

// displayLines escapes terminal controls but preserves line breaks.
func displayLines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if !strings.Contains(value, "\n") {
		return displayText(value)
	}

	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = displayText(line)
	}
	return strings.Join(lines, "\n")
}

// unprintableRune reports whether r can control terminal output or row layout.
// It allows other valid Unicode, including joiners and private-use glyphs.
func unprintableRune(r rune) bool {
	switch {
	case unicode.IsControl(r):
		return true
	case r == '\u2028', r == '\u2029':
		return true
	// Bidi controls can reorder text in the row.
	case unicode.Is(unicode.Bidi_Control, r):
		return true
	}
	return false
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if limit < 4 || len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}
