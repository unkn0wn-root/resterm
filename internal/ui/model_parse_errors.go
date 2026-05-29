package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

type eView struct {
	pretty string
	raw    string
	head   string
}

type eSty struct {
	title lipgloss.Style
	label lipgloss.Style
	loc   lipgloss.Style
	src   lipgloss.Style
	bar   lipgloss.Style
	chain lipgloss.Style
	note  lipgloss.Style
}

const (
	diagWarnLightColor  = "#d97706"
	diagWarnDarkColor   = "#FACC15"
	diagErrorLightColor = "#dc2626"
	diagErrorDarkColor  = "#FF6E6E"
)

func docErr(doc *restfile.Document) error {
	return parser.Check(doc)
}

func (m *Model) failErr(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	return m.handleResponseMessage(responseMsg{err: err})
}

func (m Model) errView(err error) eView {
	if err == nil {
		return eView{}
	}
	rep := diag.ReportOf(err)
	raw := diag.RenderReport(rep)
	return eView{
		pretty: m.styleLines(diag.Lines(rep)),
		raw:    raw,
		head:   raw,
	}
}

func firstErrLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			return line
		}
	}
	return ""
}

func (m Model) styleLines(ls []diag.Line) string {
	st := m.errSty()
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, st.line(l))
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

func (m Model) errSty() eSty {
	err := m.fallbackFg(m.theme.Error, diagErrorLightColor, diagErrorDarkColor)
	warn := m.fallbackFg(m.theme.StatusBarKey, diagWarnLightColor, diagWarnDarkColor)
	return eSty{
		title: err.Bold(true),
		label: warn.Bold(true),
		loc:   warn,
		src:   theme.ActiveTextStyle(m.theme),
		bar:   m.themeRuntime.subtleTextStyle(m.theme),
		chain: m.chainLineStyle(),
		note:  m.themeRuntime.subtleTextStyle(m.theme),
	}
}

func (m Model) chainLineStyle() lipgloss.Style {
	return m.fallbackFg(m.theme.StatusBarKey, diagWarnLightColor, diagWarnDarkColor)
}

func (m Model) fallbackFg(st lipgloss.Style, light, dark string) lipgloss.Style {
	if fg := st.GetForeground(); theme.ColorDefined(fg) {
		return lipgloss.NewStyle().Foreground(fg)
	}
	return lipgloss.NewStyle().
		Foreground(theme.ColorForAppearance(m.themeRuntime.appearance, light, dark))
}

func (st eSty) line(l diag.Line) string {
	switch l.Kind {
	case diag.LineHead:
		return st.title.Render(l.Text)
	case diag.LineLoc:
		return st.loc.Render(l.Text)
	case diag.LineBar, diag.LineSrc:
		return st.src.Render(l.Text)
	case diag.LineMark:
		return st.mark(l.Text)
	case diag.LineChain:
		return st.chain.Render(l.Text)
	case diag.LineNote, diag.LineHelp:
		return st.note.Render(l.Text)
	case diag.LineStack:
		return st.loc.Render(l.Text)
	default:
		if l.Text == "" {
			return l.Text
		}
		return st.bar.Render(l.Text)
	}
}

func (st eSty) mark(text string) string {
	before, after, ok := strings.Cut(text, "^")
	if !ok {
		return st.title.Render(text)
	}
	return st.src.Render(before) + st.title.Render("^"+after)
}
