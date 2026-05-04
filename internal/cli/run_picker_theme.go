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
	runPickerTitleDark  = "#F4E27A"
	runPickerMutedDark  = "#8E88A7"
	runPickerMutedLite  = "#64748B"
	runPickerSelBgDark  = ""
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

	df := theme.OrDefault(def)
	th := df.Theme
	ap := df.Appearance()
	activeText := theme.ActiveTextStyle(th)
	customThemeOverride := def != nil && def.Source == theme.SourceUser

	boxBd := pickerColor(
		pickerBorderColor(
			th.CLIRunPicker,
			th.AppFrame,
			th.EditorHintBox,
			th.BrowserBorder,
			th.EditorBorder,
			th.ResponseBorder,
		),
		theme.ColorForAppearance(ap, runPickerBorderLite, runPickerBorderDark),
	)
	boxBg := pickerColor(
		th.CLIRunPicker.GetBackground(),
		th.ModalInputBackground,
		th.CommandBar.GetBackground(),
		theme.ColorForAppearance(ap, runPickerBgLite, ""),
	)
	title := pickerTitleColor(th, ap, customThemeOverride)
	path := pickerColor(
		th.CLIRunPicker.GetForeground(),
		th.HeaderValue.GetForeground(),
		activeText.GetForeground(),
		theme.ColorForAppearance(ap, runPickerTextLite, runPickerTextDark),
	)
	mute := pickerColor(
		th.ExplainMuted.GetForeground(),
		th.ListItemDescription.GetForeground(),
		th.NavigatorTag.GetForeground(),
		theme.ColorForAppearance(ap, runPickerMutedLite, runPickerMutedDark),
	)
	text := pickerColor(
		th.CLIRunPicker.GetForeground(),
		th.ListItemTitle.GetForeground(),
		activeText.GetForeground(),
		th.HeaderValue.GetForeground(),
		theme.ColorForAppearance(ap, runPickerTextLite, runPickerTextDark),
	)
	selBg := pickerColor(
		th.CLIRunPickerSelected.GetBackground(),
		th.ListItemSelectedTitle.GetBackground(),
		th.ListItemSelectedDescription.GetBackground(),
		theme.ColorForAppearance(ap, runPickerSelBgLite, runPickerSelBgDark),
	)
	selFg := pickerColor(
		th.CLIRunPickerSelected.GetForeground(),
		th.ListItemSelectedTitle.GetForeground(),
		th.ListItemSelectedDescription.GetForeground(),
		activeText.GetForeground(),
		theme.ColorForAppearance(ap, runPickerSelFgLite, runPickerSelFgDark),
	)
	cur := pickerColor(
		th.CLIRunPickerCursor.GetForeground(),
		th.ResponseCursor.GetForeground(),
		th.NavigatorTag.GetForeground(),
		th.ExplainMuted.GetForeground(),
		theme.ColorForAppearance(ap, runPickerCurLite, runPickerCurDark),
	)
	acc := pickerAccentColor(th, ap, customThemeOverride)
	line := pickerColor(
		th.ListItemDescription.GetForeground(),
		th.ExplainMuted.GetForeground(),
		mute,
	)
	lineSel := pickerColor(
		th.ListItemSelectedDescription.GetForeground(),
		th.ListItemSelectedTitle.GetForeground(),
		selFg,
	)
	tgt := pickerColor(
		th.ExplainLabel.GetForeground(),
		th.HeaderTitle.GetForeground(),
		theme.ColorForAppearance(ap, runPickerTgtLite, runPickerTgtDark),
	)
	note := pickerColor(
		th.Error.GetForeground(),
		theme.ColorForAppearance(ap, runPickerErrLite, runPickerErrDark),
	)

	titleStyle := textOnlyStyle(r, th.HeaderTitle, title).Bold(true)
	cursorSelStyle := textOnlyStyle(r, th.CLIRunPickerCursorSelected, acc).Bold(true)
	numberSelStyle := textOnlyStyle(r, th.HeaderTitle, title).Bold(true)
	if !customThemeOverride {
		titleStyle = forceTextStyle(r, th.HeaderTitle, title).Bold(true)
		cursorSelStyle = forceTextStyle(r, th.CLIRunPickerCursorSelected, acc).Bold(true)
		numberSelStyle = forceTextStyle(r, th.HeaderTitle, title).Bold(true)
	}

	st := runRequestPickerStyle{
		box: withBackground(textOnlyStyle(r, th.CLIRunPicker, pickerColor(
			th.CommandBar.GetForeground(),
			boxBd,
		)).
			Border(lipgloss.NormalBorder()).
			BorderForeground(boxBd).
			Padding(0, 1), boxBg),
		title:     titleStyle,
		path:      textOnlyStyle(r, th.HeaderValue, path),
		meta:      textOnlyStyle(r, th.ExplainMuted, mute),
		row:       r.NewStyle(),
		rowSel:    withForeground(withBackground(r.NewStyle(), selBg), selFg).Bold(true),
		cursor:    textOnlyStyle(r, th.CLIRunPickerCursor, cur),
		cursorSel: cursorSelStyle,
		number:    textOnlyStyle(r, th.ExplainMuted, mute),
		numberSel: numberSelStyle,
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

func pickerTitleColor(
	th theme.Theme,
	ap theme.Appearance,
	customThemeOverride bool,
) lipgloss.TerminalColor {
	if customThemeOverride {
		return pickerColor(
			th.HeaderTitle.GetForeground(),
			th.CommandBarHint.GetForeground(),
			th.ExplainSectionTitle.GetForeground(),
			theme.ColorForAppearance(ap, runPickerAccLite, runPickerAccDark),
		)
	}
	return theme.ColorForAppearance(ap, runPickerAccLite, runPickerTitleDark)
}

func pickerAccentColor(
	th theme.Theme,
	ap theme.Appearance,
	customThemeOverride bool,
) lipgloss.TerminalColor {
	if customThemeOverride {
		return pickerColor(
			th.CLIRunPickerCursorSelected.GetForeground(),
			th.HeaderTitle.GetForeground(),
			th.CommandBarHint.GetForeground(),
			theme.ColorForAppearance(ap, runPickerAccLite, runPickerAccDark),
		)
	}
	return pickerColor(
		th.CLIRunPickerCursorSelected.GetForeground(),
		theme.ColorForAppearance(ap, runPickerAccLite, runPickerAccDark),
	)
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

func forceTextStyle(
	r *lipgloss.Renderer,
	base lipgloss.Style,
	fg lipgloss.TerminalColor,
) lipgloss.Style {
	st := textOnlyStyle(r, base, nil)
	if theme.ColorDefined(fg) {
		st = st.Foreground(fg)
	}
	return st
}

func withBackground(st lipgloss.Style, bg lipgloss.TerminalColor) lipgloss.Style {
	if theme.ColorDefined(bg) {
		return st.Background(bg)
	}
	return st
}

func withForeground(st lipgloss.Style, fg lipgloss.TerminalColor) lipgloss.Style {
	if theme.ColorDefined(fg) {
		return st.Foreground(fg)
	}
	return st
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
