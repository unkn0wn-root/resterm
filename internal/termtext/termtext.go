package termtext

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// Expand tabs before measuring rows so renderer tab settings cannot change
// their width.
const tabWidth = 4

// Row escapes invalid UTF-8 and characters that can disrupt a terminal row, so
// measured cell widths match the rendered text.
func Row(s string) string {
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
		if needsEscape(cluster, cells) {
			escape(&b, cluster)
			continue
		}
		b.WriteString(cluster)
	}
	return b.String()
}

// Block escapes multi-row text while preserving line breaks. It normalizes CRLF
// to LF and expands tabs to spaces before escaping each row.
func Block(s string) string {
	rows := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, row := range rows {
		rows[i] = Row(strings.ReplaceAll(row, "\t", strings.Repeat(" ", tabWidth)))
	}
	return strings.Join(rows, "\n")
}

// EscapeCluster returns the display form of one grapheme cluster and reports
// whether it changed. cells is the width the segmenter measured for it.
func EscapeCluster(cluster string, cells int) (string, bool) {
	if !needsEscape(cluster, cells) {
		return cluster, false
	}
	var b strings.Builder
	escape(&b, cluster)
	return b.String(), true
}

// needsEscape skips clusters wider than zero cells. A visible glyph carries
// them, so escaping their format characters would break emoji and shaping.
func needsEscape(cluster string, cells int) bool {
	r, _ := utf8.DecodeRuneInString(cluster)
	return (cells == 0 && unprintable(r)) || !utf8.ValidString(cluster)
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

const hexDigits = "0123456789abcdef"

// escape uses JSON escapes to keep valid JSON bodies parseable.
// Invalid UTF-8 bytes use \xNN to preserve their values.
func escape(b *strings.Builder, cluster string) {
	for i := 0; i < len(cluster); {
		r, size := utf8.DecodeRuneInString(cluster[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString(`\x`)
			b.WriteByte(hexDigits[cluster[i]>>4])
			b.WriteByte(hexDigits[cluster[i]&0xf])
			i++
			continue
		}
		escapeRune(b, r)
		i += size
	}
}

func escapeRune(b *strings.Builder, r rune) {
	if short := shortEscape(r); short != "" {
		b.WriteString(short)
		return
	}
	// JSON escapes code points above U+FFFF as UTF-16 surrogate pairs.
	if r > 0xffff {
		hi, lo := utf16.EncodeRune(r)
		escapeUnit(b, hi)
		escapeUnit(b, lo)
		return
	}
	escapeUnit(b, r)
}

func escapeUnit(b *strings.Builder, r rune) {
	b.WriteString(`\u`)
	for shift := 12; shift >= 0; shift -= 4 {
		b.WriteByte(hexDigits[(r>>shift)&0xf])
	}
}

func shortEscape(r rune) string {
	switch r {
	case '\b':
		return `\b`
	case '\f':
		return `\f`
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	}
	return ""
}
