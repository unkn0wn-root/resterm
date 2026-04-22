package theme

import "github.com/charmbracelet/lipgloss"

type CLIPalette struct {
	SyntaxStyle string
	Heading     string
	Label       string
	Value       string
	Success     string
	Warn        string
	Caution     string
	Neutral     string
	Message     string
	HeaderValue string
	Duration    string
}

func CLIPaletteFor(def *Definition) CLIPalette {
	df := OrDefault(def)
	th := df.Theme
	ap := df.Appearance()
	activeText := ActiveTextStyle(th)
	return CLIPalette{
		SyntaxStyle: SyntaxHighlightStyle(df),
		Heading: colorText(
			th.ExplainMuted.GetForeground(),
			th.ListItemDescription.GetForeground(),
			th.NavigatorTag.GetForeground(),
			ColorForAppearance(ap, "#64748b", "#A6A1BB"),
		),
		Label: colorText(
			th.ExplainMuted.GetForeground(),
			th.ListItemDescription.GetForeground(),
			th.NavigatorTag.GetForeground(),
			ColorForAppearance(ap, "#64748b", "#A6A1BB"),
		),
		Value: colorText(
			activeText.GetForeground(),
			th.ListItemTitle.GetForeground(),
			th.HeaderValue.GetForeground(),
			ColorForAppearance(ap, "#0f172a", "#E8E9F0"),
		),
		Success: colorText(
			th.Success.GetForeground(),
			ColorForAppearance(ap, "#15803d", "#44C25B"),
		),
		Warn: colorText(
			th.Error.GetForeground(),
			ColorForAppearance(ap, "#b91c1c", "#F25F5C"),
		),
		Caution: colorText(
			th.ExplainWarning.GetForeground(),
			th.StatusBarKey.GetForeground(),
			ColorForAppearance(ap, "#b45309", "#FFD46A"),
		),
		Neutral: colorText(
			th.HeaderTitle.GetForeground(),
			th.ExplainSectionTitle.GetForeground(),
			ColorForAppearance(ap, "#1d4ed8", "#7D56F4"),
		),
		Message: colorText(
			th.ExplainMuted.GetForeground(),
			th.ListItemDescription.GetForeground(),
			ColorForAppearance(ap, "#64748b", "#A6A1BB"),
		),
		HeaderValue: colorText(
			th.ResponseContentHeaders.GetForeground(),
			th.HeaderValue.GetForeground(),
			ColorForAppearance(ap, "#334155", "#D2D4F5"),
		),
		Duration: colorText(
			th.ExplainLabel.GetForeground(),
			ColorForAppearance(ap, "#0369a1", "#56C2F4"),
		),
	}
}

func SyntaxHighlightStyle(def Definition) string {
	if def.Appearance() == AppearanceLight {
		return "github"
	}
	return "monokai"
}

func ColorText(c lipgloss.TerminalColor) string {
	if !ColorDefined(c) {
		return ""
	}
	if v, ok := c.(lipgloss.Color); ok {
		return string(v)
	}
	return ""
}

func colorText(cs ...lipgloss.TerminalColor) string {
	for _, c := range cs {
		if v := ColorText(c); v != "" {
			return v
		}
	}
	return ""
}
