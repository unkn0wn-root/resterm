package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

type statsPalette struct {
	Title       lipgloss.Style
	Heading     lipgloss.Style
	HeadingWarn lipgloss.Style
	Label       lipgloss.Style
	SubLabel    lipgloss.Style
	Value       lipgloss.Style
	Success     lipgloss.Style
	Warn        lipgloss.Style
	Caution     lipgloss.Style
	Neutral     lipgloss.Style
	Message     lipgloss.Style
	HeaderValue lipgloss.Style
	Duration    lipgloss.Style
	Selected    lipgloss.Style
}

func defaultStatsPalette() statsPalette {
	return statsPalette{
		Title:       statsTitleStyle,
		Heading:     statsHeadingStyle,
		HeadingWarn: statsHeadingWarn,
		Label:       statsLabelStyle,
		SubLabel:    statsSubLabelStyle,
		Value:       statsValueStyle,
		Success:     statsSuccessStyle,
		Warn:        statsWarnStyle,
		Caution:     statsCautionStyle,
		Neutral:     statsNeutralStyle,
		Message:     statsMessageStyle,
		HeaderValue: statsHeaderValueStyle,
		Duration:    statsDurationStyle,
		Selected:    statsSelectedStyle,
	}
}

func lightStatsPalette(th theme.Theme) statsPalette {
	label := theme.ForegroundStyle(th.ExplainMuted, lipgloss.Color("#475569"))
	subLabel := theme.ForegroundStyle(th.ExplainMuted, lipgloss.Color("#64748b"))
	value := theme.ActiveTextStyle(th).Bold(true)
	success := foregroundStyle(th.Success, lipgloss.Color("#15803d")).Bold(true)
	warn := foregroundStyle(th.Error, lipgloss.Color("#b91c1c")).Bold(true)
	caution := foregroundStyle(th.StatusBarKey, lipgloss.Color("#b45309")).Bold(true)
	neutral := foregroundStyle(th.HeaderTitle, lipgloss.Color("#1d4ed8"))
	headerValue := foregroundStyle(th.HeaderValue, lipgloss.Color("#334155"))
	duration := foregroundStyle(th.ExplainLabel, lipgloss.Color("#0369a1")).Bold(true)

	selectedBG := th.ResponseSelection.GetBackground()
	if !theme.ColorDefined(selectedBG) {
		selectedBG = lipgloss.Color("#e2e8f0")
	}

	return statsPalette{
		Title:       neutral.Bold(true),
		Heading:     label.Bold(true),
		HeadingWarn: warn,
		Label:       label,
		SubLabel:    subLabel,
		Value:       value,
		Success:     success,
		Warn:        warn,
		Caution:     caution,
		Neutral:     neutral,
		Message:     subLabel,
		HeaderValue: headerValue,
		Duration:    duration,
		Selected: lipgloss.NewStyle().
			Foreground(foregroundColor(value, lipgloss.Color("#0f172a"))).
			Background(selectedBG),
	}
}

func foregroundStyle(base lipgloss.Style, fallback lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(foregroundColor(base, fallback))
}

func foregroundColor(base lipgloss.Style, fallback lipgloss.Color) lipgloss.TerminalColor {
	if fg := base.GetForeground(); theme.ColorDefined(fg) {
		return fg
	}
	return fallback
}
