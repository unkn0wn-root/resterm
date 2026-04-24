package cli

import (
	"io"

	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/runx/report"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

// WriteTextStyled writes a runfmt text report with CLI-owned ANSI styling.
func WriteTextStyled(
	w io.Writer,
	rep *runfmt.Report,
	color termcolor.Config,
	def *theme.Definition,
) error {
	return runfmt.WriteTextStyled(w, rep, textPainter{
		cfg: color,
		pal: theme.CLIPaletteFor(def),
	})
}

type textPainter struct {
	cfg termcolor.Config
	pal theme.CLIPalette
}

func (p textPainter) PaintText(text, fg string, bold bool) string {
	if !p.cfg.Enabled || text == "" {
		return text
	}
	profile := p.profile()
	st := profile.String(text)
	if fg != "" {
		st = st.Foreground(profile.Color(fg))
	}
	if bold {
		st = st.Bold()
	}
	return st.String()
}

func (p textPainter) profile() termenv.Profile {
	if p.cfg.Profile == termenv.Ascii {
		return termenv.ANSI
	}
	return p.cfg.Profile
}

func (p textPainter) TextPalette() runfmt.TextPalette {
	return runfmt.TextPalette{
		Heading: p.pal.Heading,
		Success: p.pal.Success,
		Warn:    p.pal.Warn,
		Caution: p.pal.Caution,
		Value:   p.pal.Value,
	}
}
