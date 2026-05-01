package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

type textInputKind int

const (
	textInputKindGeneric textInputKind = iota
	textInputKindNavigator
	textInputKindHistory
	textInputKindHelp
)

type themeRuntime struct {
	definition theme.Definition
	appearance theme.Appearance
}

func newThemeRuntime(def theme.Definition) themeRuntime {
	return themeRuntime{
		definition: def,
		appearance: def.Appearance(),
	}
}

func (rt themeRuntime) isLight() bool {
	return rt.appearance == theme.AppearanceLight
}

func (rt themeRuntime) inactiveStyle(style lipgloss.Style) lipgloss.Style {
	if rt.isLight() {
		return style
	}
	return style.Faint(true)
}

func (rt themeRuntime) inactiveRendered(content string) string {
	if rt.isLight() {
		return content
	}
	prefix, suffix := styleSGR(lipgloss.NewStyle().Faint(true))
	return applyPersistentANSIStyle(content, prefix, suffix)
}

func (rt themeRuntime) subtleTextStyle(th theme.Theme) lipgloss.Style {
	if rt.isLight() {
		return theme.ForegroundStyle(th.ExplainMuted, lipgloss.Color("#64748b"))
	}
	return lipgloss.NewStyle().Faint(true)
}

func (rt themeRuntime) historyPlaceholderStyle(th theme.Theme) lipgloss.Style {
	if rt.isLight() {
		return rt.subtleTextStyle(th)
	}
	return lipgloss.NewStyle().Faint(true)
}

func (rt themeRuntime) helpHintStyle(th theme.Theme) lipgloss.Style {
	if rt.isLight() {
		return rt.subtleTextStyle(th)
	}
	return lipgloss.NewStyle().Faint(true)
}

func (rt themeRuntime) inputLabelStyle(th theme.Theme) lipgloss.Style {
	if fg := th.ExplainMuted.GetForeground(); theme.ColorDefined(fg) {
		return lipgloss.NewStyle().Foreground(fg)
	}
	if rt.isLight() {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))
	}
	if fg := th.HeaderValue.GetForeground(); theme.ColorDefined(fg) {
		return lipgloss.NewStyle().Foreground(fg)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#A6A1BB"))
}

func (rt themeRuntime) modalBackdropColor(th theme.Theme) lipgloss.TerminalColor {
	if theme.ColorDefined(th.ModalBackdrop) {
		return th.ModalBackdrop
	}
	if !rt.isLight() {
		return lipgloss.Color("#1A1823")
	}
	if bg := th.CommandBar.GetBackground(); theme.ColorDefined(bg) {
		return bg
	}
	if bg := th.ResponseSelection.GetBackground(); theme.ColorDefined(bg) {
		return bg
	}
	return lipgloss.Color("#E2E8F0")
}

func (rt themeRuntime) modalInputBackground(th theme.Theme) lipgloss.TerminalColor {
	if theme.ColorDefined(th.ModalInputBackground) {
		return th.ModalInputBackground
	}
	if !rt.isLight() {
		return lipgloss.Color("#1c1a23")
	}
	if bg := th.ResponseSelection.GetBackground(); theme.ColorDefined(bg) {
		return bg
	}
	if bg := th.CommandBar.GetBackground(); theme.ColorDefined(bg) {
		return bg
	}
	return lipgloss.Color("#E2E8F0")
}

func (rt themeRuntime) modalOptionStyle(th theme.Theme) lipgloss.Style {
	if theme.ColorDefined(th.ModalOption) {
		return lipgloss.NewStyle().Foreground(th.ModalOption)
	}
	if rt.isLight() {
		return theme.ForegroundStyle(th.ExplainMuted, lipgloss.Color("#64748b"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#4D4663"))
}

func (rt themeRuntime) editorSelectionBackground(th theme.Theme) lipgloss.TerminalColor {
	if bg := th.ResponseSelection.GetBackground(); theme.ColorDefined(bg) {
		return bg
	}
	if bg := th.CommandBar.GetBackground(); theme.ColorDefined(bg) {
		return bg
	}
	return lipgloss.Color("#E2E8F0")
}

func (rt themeRuntime) statsPalette(th theme.Theme) statsPalette {
	if rt.isLight() {
		return lightStatsPalette(th)
	}
	return defaultStatsPalette()
}

func (rt themeRuntime) syntaxHighlightStyle() string {
	return theme.SyntaxHighlightStyle(rt.definition)
}

func (rt themeRuntime) responseRenderer(th theme.Theme) responseRenderer {
	return newResponseRenderer(rt.statsPalette(th), rt.syntaxHighlightStyle())
}

func (rt themeRuntime) applyTextInput(
	ti *textinput.Model,
	th theme.Theme,
	kind textInputKind,
) {
	if ti == nil {
		return
	}
	switch kind {
	case textInputKindNavigator:
		ti.TextStyle = th.NavigatorTitle
		ti.PromptStyle = th.NavigatorTitle
		ti.PlaceholderStyle = th.NavigatorSubtitle
		ti.Cursor.Style = th.NavigatorTitle
	case textInputKindHelp:
		textStyle := theme.ActiveTextStyle(th)
		ti.TextStyle = textStyle
		ti.PromptStyle = rt.inputLabelStyle(th)
		ti.PlaceholderStyle = rt.subtleTextStyle(th)
		ti.Cursor.Style = textStyle
	case textInputKindHistory:
		textStyle := theme.ActiveTextStyle(th)
		ti.TextStyle = textStyle
		ti.PromptStyle = rt.inputLabelStyle(th)
		ti.PlaceholderStyle = rt.historyPlaceholderStyle(th)
		ti.Cursor.Style = textStyle
	default:
		if rt.isLight() {
			textStyle := theme.ActiveTextStyle(th)
			ti.TextStyle = textStyle
			ti.PromptStyle = th.HeaderTitle
			ti.PlaceholderStyle = rt.subtleTextStyle(th)
			ti.Cursor.Style = th.HeaderTitle
			return
		}
		ti.TextStyle = lipgloss.Style{}
		ti.PromptStyle = lipgloss.Style{}
		ti.PlaceholderStyle = lipgloss.Style{}
		ti.Cursor.Style = lipgloss.Style{}
	}
}

func (rt themeRuntime) applyRequestEditor(ed *requestEditor, th theme.Theme) {
	if ed == nil {
		return
	}
	if !rt.isLight() {
		focused, blurred := textarea.DefaultStyles()
		ed.FocusedStyle = focused
		ed.BlurredStyle = blurred
		ed.SetSelectionStyle(lipgloss.NewStyle().Background(lipgloss.Color("#4C3F72")))
		if ed.Focused() {
			ed.Focus()
		} else {
			ed.Blur()
		}
		return
	}

	textStyle := theme.ActiveTextStyle(th)
	mutedStyle := rt.subtleTextStyle(th)
	promptStyle := th.HeaderTitle
	cursorLine := textStyle
	if bg := th.ResponseSelection.GetBackground(); theme.ColorDefined(bg) {
		cursorLine = cursorLine.Background(bg)
	}

	focused := textarea.Style{
		Base:             lipgloss.NewStyle(),
		CursorLine:       cursorLine,
		CursorLineNumber: mutedStyle,
		EndOfBuffer:      mutedStyle,
		LineNumber:       mutedStyle,
		Placeholder:      mutedStyle,
		Prompt:           promptStyle,
		Text:             textStyle,
	}
	blurred := focused
	blurred.CursorLine = textStyle
	ed.FocusedStyle = focused
	ed.BlurredStyle = blurred
	ed.SetSelectionStyle(lipgloss.NewStyle().Background(rt.editorSelectionBackground(th)))
	if ed.Focused() {
		ed.Focus()
	} else {
		ed.Blur()
	}
}

func (m *Model) applyThemeDefinition(def theme.Definition) {
	m.theme = def.Theme
	m.activeThemeDef = def
	m.activeThemeKey = def.Key
	m.themeRuntime = newThemeRuntime(def)
	m.editor.SetRuneStyler(selectEditorRuneStyler(m.currentFile, m.theme.EditorMetadata))
	m.applyThemeToInputs()
	m.applyThemeToLists()
	m.invalidateThemedCaches()
}

func (m *Model) applyThemeToInputs() {
	m.themeRuntime.applyRequestEditor(&m.editor, m.theme)
	m.themeRuntime.applyTextInput(&m.searchInput, m.theme, textInputKindGeneric)
	m.themeRuntime.applyTextInput(&m.helpFilter, m.theme, textInputKindHelp)
	m.themeRuntime.applyTextInput(&m.newFileInput, m.theme, textInputKindGeneric)
	m.themeRuntime.applyTextInput(&m.openPathInput, m.theme, textInputKindGeneric)
	m.themeRuntime.applyTextInput(&m.responseSaveInput, m.theme, textInputKindGeneric)
	m.themeRuntime.applyTextInput(&m.streamFilterInput, m.theme, textInputKindGeneric)
	m.themeRuntime.applyTextInput(&m.historyFilterInput, m.theme, textInputKindHistory)
	m.themeRuntime.applyTextInput(&m.navigatorFilter, m.theme, textInputKindNavigator)
}

func (m *Model) invalidateThemedCaches() {
	snapshots := m.collectResponseSnapshots()
	renderer := m.themeRuntime.responseRenderer(m.theme)
	for _, snapshot := range snapshots {
		m.rerenderThemedSnapshot(snapshot, renderer)
		snapshot.statsColored = ""
		snapshot.traceReport = timelineReport{}
		snapshot.explain.cache = explainRenderCache{}
	}
	for i := range m.responsePanes {
		m.responsePanes[i].invalidateCaches()
	}
}
