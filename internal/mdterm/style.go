package mdterm

import (
	"github.com/muesli/termenv"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

// attr is a bitmask of inline styles; spans carry the full set they need so
// each paints as a self-contained SGR sequence.
type attr uint8

const (
	aBold attr = 1 << iota
	aItalic
	aCode
	aLink
	aFaint
	aUnder
	aAccent
)

// ANSI-16 palette indexes: the terminal theme picks the actual shades, so
// output stays readable on light and dark backgrounds alike.
const (
	fgCode   = "5" // magenta
	fgLink   = "4" // blue
	fgAccent = "6" // cyan
)

// styler paints spans when color is on and passes text through untouched
// when it isn't.
type styler struct {
	on bool
	p  termenv.Profile
}

func newStyler(cfg termcolor.Config) styler {
	return styler{on: cfg.Enabled, p: cfg.Termenv()}
}

func (st styler) span(s string, at attr) string {
	if !st.on || at == 0 || s == "" {
		return s
	}
	t := st.p.String(s)
	if at&aCode != 0 {
		t = t.Foreground(st.p.Color(fgCode))
	}
	if at&aAccent != 0 {
		t = t.Foreground(st.p.Color(fgAccent))
	}
	if at&aLink != 0 {
		t = t.Underline().Foreground(st.p.Color(fgLink))
	}
	if at&aFaint != 0 {
		t = t.Faint()
	}
	if at&aUnder != 0 {
		t = t.Underline()
	}
	if at&aBold != 0 {
		t = t.Bold()
	}
	if at&aItalic != 0 {
		t = t.Italic()
	}
	return t.String()
}
