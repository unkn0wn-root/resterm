package bodyfmt

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// DisplayRow escapes invalid UTF-8 and characters that can disrupt a terminal
// row, so measured cell widths match the rendered text.
func DisplayRow(s string) string {
	if utf8.ValidString(s) && !strings.ContainsFunc(s, unprintable) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	state := -1
	for len(s) > 0 {
		var (
			cluster string
			cells   int
		)
		cluster, s, cells, state = uniseg.FirstGraphemeClusterInString(s, state)
		r, _ := utf8.DecodeRuneInString(cluster)
		// Preserve format characters attached to a visible glyph. Escaping them
		// separately would break emoji sequences and text shaping.
		if (cells == 0 && unprintable(r)) || !utf8.ValidString(cluster) {
			quoted := strconv.QuoteToGraphic(cluster)
			b.WriteString(quoted[1 : len(quoted)-1])
			continue
		}
		b.WriteString(cluster)
	}
	return b.String()
}

// Expand tabs before measuring rows so renderer tab settings cannot change
// their width.
const bodyTabWidth = 4

// DisplayBody escapes body text while preserving line breaks. It normalizes
// CRLF to LF and expands tabs to spaces before escaping each row.
func DisplayBody(s string) string {
	rows := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, row := range rows {
		rows[i] = DisplayRow(strings.ReplaceAll(row, "\t", strings.Repeat(" ", bodyTabWidth)))
	}
	return strings.Join(rows, "\n")
}

// unprintable reports whether r can affect terminal layout beyond its cell width.
func unprintable(r rune) bool {
	switch {
	case r < utf8.RuneSelf:
		return r < 0x20 || r == 0x7f
	// Preserve tag characters used in emoji flag sequences.
	case r >= 0xe0020 && r <= 0xe007f:
		return false
	}
	return unicode.IsControl(r) ||
		unicode.Is(unicode.Cf, r) ||
		unicode.Is(unicode.Zl, r) ||
		unicode.Is(unicode.Zp, r)
}
