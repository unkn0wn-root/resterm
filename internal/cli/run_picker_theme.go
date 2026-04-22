package cli

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	runPickerBorderDark = "#4B4670"
	runPickerBorderLite = "#CBD5E1"
	runPickerTextDark   = "#F8F7FF"
	runPickerTextLite   = "#0F172A"
	runPickerMutedDark  = "#8E88A7"
	runPickerMutedLite  = "#64748B"
	runPickerSelBgDark  = "#261F42"
	runPickerSelBgLite  = "#E2E8F0"
	runPickerSelFgDark  = "#F8F7FF"
	runPickerSelFgLite  = "#0F172A"
	runPickerAccDark    = "#FFD46A"
	runPickerAccLite    = "#1E40AF"
	runPickerCurDark    = "#6F688D"
	runPickerCurLite    = "#6B7280"
	runPickerTgtDark    = "#8FD3FF"
	runPickerTgtLite    = "#0369A1"
	runPickerErrDark    = "#F87171"
	runPickerErrLite    = "#B91C1C"
	runPickerBgLite     = "#F8FAFC"
)

func newRunRequestPickerStyle(cfg termcolor.Config, def *theme.Definition) runRequestPickerStyle {
	r := lipgloss.NewRenderer(io.Discard)
	if cfg.Enabled {
		r.SetColorProfile(cfg.Profile)
	} else {
		r.SetColorProfile(termenv.Ascii)
	}

	df := pickerTheme(def)
	th := df.Theme
	ap := df.Appearance()

	boxBd := pickerColor(
		pickerBorderColor(
			th.AppFrame,
			th.EditorHintBox,
			th.BrowserBorder,
			th.EditorBorder,
			th.ResponseBorder,
		),
		pickerBorderFallback(ap),
	)
	boxBg := pickerColor(
		th.ModalInputBackground,
		pickerStyleBg(th.CommandBar),
		pickerBoxBgFallback(ap),
	)
	title := pickerColor(
		pickerStyleFg(th.HeaderTitle),
		pickerStyleFg(th.CommandBarHint),
		pickerStyleFg(th.ExplainSectionTitle),
		pickerAccentFallback(ap),
	)
	path := pickerColor(
		pickerStyleFg(th.HeaderValue),
		pickerStyleFg(theme.ActiveTextStyle(th)),
		pickerTextFallback(ap),
	)
	mute := pickerColor(
		pickerStyleFg(th.ExplainMuted),
		pickerStyleFg(th.ListItemDescription),
		pickerStyleFg(th.NavigatorTag),
		pickerMutedFallback(ap),
	)
	text := pickerColor(
		pickerStyleFg(th.ListItemTitle),
		pickerStyleFg(theme.ActiveTextStyle(th)),
		pickerStyleFg(th.HeaderValue),
		pickerTextFallback(ap),
	)
	selBg := pickerColor(
		pickerStyleBg(th.ListItemSelectedTitle),
		pickerStyleBg(th.ListItemSelectedDescription),
		pickerStyleBg(th.ResponseSelection),
		pickerSelBgFallback(ap),
	)
	selFg := pickerColor(
		pickerStyleFg(th.ListItemSelectedTitle),
		pickerStyleFg(th.ListItemSelectedDescription),
		pickerStyleFg(theme.ActiveTextStyle(th)),
		pickerSelFgFallback(ap),
	)
	cur := pickerColor(
		pickerStyleFg(th.ResponseCursor),
		pickerStyleFg(th.NavigatorTag),
		pickerStyleFg(th.ExplainMuted),
		pickerCursorFallback(ap),
	)
	acc := pickerColor(
		pickerStyleFg(th.CommandBarHint),
		pickerStyleFg(th.HeaderTitle),
		pickerAccentFallback(ap),
	)
	line := pickerColor(
		pickerStyleFg(th.ListItemDescription),
		pickerStyleFg(th.ExplainMuted),
		mute,
	)
	lineSel := pickerColor(
		pickerStyleFg(th.ListItemSelectedDescription),
		pickerStyleFg(th.ListItemSelectedTitle),
		selFg,
	)
	tgt := pickerColor(
		pickerStyleFg(th.ExplainLabel),
		pickerStyleFg(th.HeaderTitle),
		pickerTargetFallback(ap),
	)
	note := pickerColor(
		pickerStyleFg(th.Error),
		pickerErrorFallback(ap),
	)

	st := runRequestPickerStyle{
		box: textOnlyStyle(r, th.CommandBar, boxBd).
			Border(lipgloss.NormalBorder()).
			BorderForeground(boxBd).
			Background(boxBg).
			Padding(0, 1),
		title: textOnlyStyle(r, th.HeaderTitle, title).Bold(true),
		path:  textOnlyStyle(r, th.HeaderValue, path),
		meta:  textOnlyStyle(r, th.ExplainMuted, mute),
		row:   r.NewStyle(),
		rowSel: r.NewStyle().
			Background(selBg).
			Foreground(selFg).
			Bold(true),
		cursor:    textOnlyStyle(r, th.ResponseCursor, cur),
		cursorSel: textOnlyStyle(r, th.HeaderTitle, acc).Bold(true),
		number:    textOnlyStyle(r, th.ExplainMuted, mute),
		numberSel: textOnlyStyle(r, th.HeaderTitle, acc).Bold(true),
		name:      textOnlyStyle(r, th.ListItemTitle, text),
		nameSel:   textOnlyStyle(r, th.ListItemSelectedTitle, selFg).Bold(true),
		line:      textOnlyStyle(r, th.ListItemDescription, line),
		lineSel:   textOnlyStyle(r, th.ListItemSelectedDescription, lineSel),
		target:    textOnlyStyle(r, th.ExplainLabel, tgt),
		help:      textOnlyStyle(r, th.ExplainMuted, mute),
		note:      textOnlyStyle(r, th.Error, note).Bold(true),
		methods:   make(map[string]lipgloss.Style),
	}
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "GRPC", "WS", ""} {
		st.methods[m] = r.NewStyle().
			Foreground(pickerMethodColor(th, m)).
			Bold(true)
	}
	return st
}

func pickerTheme(def *theme.Definition) theme.Definition {
	if def != nil && def.Key != "" {
		return *def
	}
	return theme.DefaultDefinition()
}

func textOnlyStyle(
	r *lipgloss.Renderer,
	base lipgloss.Style,
	fb lipgloss.TerminalColor,
) lipgloss.Style {
	st := r.NewStyle()
	if fg := base.GetForeground(); theme.ColorDefined(fg) {
		st = st.Foreground(fg)
	} else if theme.ColorDefined(fb) {
		st = st.Foreground(fb)
	}
	if base.GetBold() {
		st = st.Bold(true)
	}
	if base.GetItalic() {
		st = st.Italic(true)
	}
	if base.GetUnderline() {
		st = st.Underline(true)
	}
	if base.GetFaint() {
		st = st.Faint(true)
	}
	return st
}

func pickerStyleFg(st lipgloss.Style) lipgloss.TerminalColor {
	return st.GetForeground()
}

func pickerStyleBg(st lipgloss.Style) lipgloss.TerminalColor {
	return st.GetBackground()
}

func pickerBorderColor(sts ...lipgloss.Style) lipgloss.TerminalColor {
	for _, st := range sts {
		for _, c := range []lipgloss.TerminalColor{
			st.GetBorderLeftForeground(),
			st.GetBorderTopForeground(),
			st.GetBorderRightForeground(),
			st.GetBorderBottomForeground(),
		} {
			if theme.ColorDefined(c) {
				return c
			}
		}
	}
	return nil
}

func pickerColor(cs ...lipgloss.TerminalColor) lipgloss.TerminalColor {
	for _, c := range cs {
		if theme.ColorDefined(c) {
			return c
		}
	}
	return nil
}

func pickerMethodColor(th theme.Theme, m string) lipgloss.TerminalColor {
	switch m {
	case "GET":
		return pickerColor(th.MethodColors.GET, lipgloss.Color("#34d399"))
	case "POST":
		return pickerColor(th.MethodColors.POST, lipgloss.Color("#60a5fa"))
	case "PUT":
		return pickerColor(th.MethodColors.PUT, lipgloss.Color("#f59e0b"))
	case "PATCH":
		return pickerColor(th.MethodColors.PATCH, lipgloss.Color("#14b8a6"))
	case "DELETE":
		return pickerColor(th.MethodColors.DELETE, lipgloss.Color("#f87171"))
	case "HEAD":
		return pickerColor(th.MethodColors.HEAD, lipgloss.Color("#a1a1aa"))
	case "OPTIONS":
		return pickerColor(th.MethodColors.OPTIONS, lipgloss.Color("#c084fc"))
	case "GRPC":
		return pickerColor(th.MethodColors.GRPC, lipgloss.Color("#22d3ee"))
	case "WS":
		return pickerColor(th.MethodColors.WS, lipgloss.Color("#fb923c"))
	default:
		return pickerColor(th.MethodColors.Default, lipgloss.Color("#9ca3af"))
	}
}

func pickerBorderFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerBorderLite)
	}
	return lipgloss.Color(runPickerBorderDark)
}

func pickerBoxBgFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerBgLite)
	}
	return nil
}

func pickerTextFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerTextLite)
	}
	return lipgloss.Color(runPickerTextDark)
}

func pickerMutedFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerMutedLite)
	}
	return lipgloss.Color(runPickerMutedDark)
}

func pickerSelBgFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerSelBgLite)
	}
	return lipgloss.Color(runPickerSelBgDark)
}

func pickerSelFgFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerSelFgLite)
	}
	return lipgloss.Color(runPickerSelFgDark)
}

func pickerAccentFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerAccLite)
	}
	return lipgloss.Color(runPickerAccDark)
}

func pickerCursorFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerCurLite)
	}
	return lipgloss.Color(runPickerCurDark)
}

func pickerTargetFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerTgtLite)
	}
	return lipgloss.Color(runPickerTgtDark)
}

func pickerErrorFallback(ap theme.Appearance) lipgloss.TerminalColor {
	if ap == theme.AppearanceLight {
		return lipgloss.Color(runPickerErrLite)
	}
	return lipgloss.Color(runPickerErrDark)
}
