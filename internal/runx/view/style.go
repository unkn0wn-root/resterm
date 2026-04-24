package runview

import (
	"net/http"
	"strings"

	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"google.golang.org/grpc/codes"
)

type tone int

const (
	toneValue tone = iota
	toneHeader
	toneSuccess
	toneWarn
	toneCaution
	toneNeutral
	toneMsg
	toneDur
)

type styler struct {
	cfg termcolor.Config
	pal theme.CLIPalette
}

func newStyler(cfg termcolor.Config, def *theme.Definition) styler {
	return styler{cfg: cfg, pal: theme.CLIPaletteFor(def)}
}

func (s styler) section(text string) string {
	return s.paint(text, s.pal.Heading, true, false)
}

func (s styler) sectionWarn(text string) string {
	return s.paint(text, s.pal.Warn, true, false)
}

func (s styler) sectionCaution(text string) string {
	return s.paint(text, s.pal.Caution, true, false)
}

func (s styler) label(text string) string {
	return s.paint(text, s.pal.Label, false, false)
}

func (s styler) pair(label, value string, t tone) string {
	key := s.label(label + ":")
	if value == "" {
		return key
	}
	return key + " " + s.value(value, t)
}

func (s styler) badge(text string, ok bool) string {
	if ok {
		return s.value(text, toneSuccess)
	}
	return s.value(text, toneWarn)
}

func (s styler) value(text string, t tone) string {
	switch t {
	case toneHeader:
		return s.paint(text, s.pal.HeaderValue, false, false)
	case toneSuccess:
		return s.paint(text, s.pal.Success, true, false)
	case toneWarn:
		return s.paint(text, s.pal.Warn, true, false)
	case toneCaution:
		return s.paint(text, s.pal.Caution, true, false)
	case toneNeutral:
		return s.paint(text, s.pal.Neutral, false, false)
	case toneMsg:
		return s.paint(text, s.pal.Message, false, true)
	case toneDur:
		return s.paint(text, s.pal.Duration, true, false)
	default:
		return s.paint(text, s.pal.Value, true, false)
	}
}

func (s styler) paint(text, fg string, bold, faint bool) string {
	if !s.cfg.Enabled || text == "" {
		return text
	}
	p := s.profile()
	st := p.String(text)
	if fg != "" {
		st = st.Foreground(p.Color(fg))
	}
	if bold {
		st = st.Bold()
	}
	if faint {
		st = st.Faint()
	}
	return st.String()
}

func (s styler) profile() termenv.Profile {
	if s.cfg.Profile == termenv.Ascii {
		return termenv.ANSI
	}
	return s.cfg.Profile
}

func formatHeaders(headers http.Header, s styler) string {
	fields := bodyfmt.HeaderFields(headers)
	if len(fields) == 0 {
		return ""
	}
	var buf strings.Builder
	for _, field := range fields {
		buf.WriteString(s.pair(field.Name, field.Value, toneHeader))
		buf.WriteByte('\n')
	}
	return strings.TrimRight(buf.String(), "\n")
}

func statusTone(res runner.Result) tone {
	switch {
	case res.Response != nil:
		return httpTone(res.Response.StatusCode)
	case res.GRPC != nil:
		if res.GRPC.StatusCode == codes.OK {
			return toneSuccess
		}
		return toneWarn
	default:
		return toneValue
	}
}

func httpTone(code int) tone {
	switch {
	case code >= 400:
		return toneWarn
	case code >= 300:
		return toneNeutral
	case code > 0:
		return toneSuccess
	default:
		return toneValue
	}
}

func indent(text, prefix string) string {
	if text == "" || prefix == "" {
		return text
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
