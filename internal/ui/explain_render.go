package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func renderExplainStyled(rep *xplain.Report, width int, th theme.Theme) string {
	if rep == nil {
		return ""
	}
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	if width < 36 {
		width = 36
	}

	v := buildExplainView(rep)
	st := newExplainStyles(th)
	bodyW := width - 2
	if bodyW < 20 {
		bodyW = width
	}

	var sections []string
	if sec := renderExplainSummarySection(v, bodyW, st); sec != "" {
		sections = append(sections, renderExplainSection("Summary", sec, width, st))
	}
	if sec := renderExplainDecisionSection(v, bodyW, st); sec != "" {
		sections = append(sections, renderExplainSection("Decision", sec, width, st))
	}
	if sec := renderExplainFinalSection(v, bodyW, st); sec != "" {
		sections = append(sections, renderExplainSection("Final Request", sec, width, st))
	}
	if sec := renderExplainStagesSection(v, bodyW, st); sec != "" {
		sections = append(sections, renderExplainSection("Stages", sec, width, st))
	}
	if sec := renderExplainVarsSection(v, bodyW, st); sec != "" {
		sections = append(sections, renderExplainSection("Variables", sec, width, st))
	}
	if sec := renderExplainWarningsSection(v, bodyW, st); sec != "" {
		sections = append(sections, renderExplainSection("Warnings", sec, width, st))
	}

	return strings.TrimRight(strings.Join(sections, "\n\n"), "\n")
}

func renderExplainSection(title, body string, width int, st explainStyles) string {
	body = strings.TrimRight(body, "\n")
	if strings.TrimSpace(body) == "" {
		return ""
	}
	head := st.sectionTitle.Render(strings.ToUpper(strings.TrimSpace(title)))
	if rem := width - visibleWidth(head) - 1; rem > 4 {
		head = lipgloss.JoinHorizontal(
			lipgloss.Left,
			head,
			" ",
			st.sectionBorder.Render(strings.Repeat("─", rem)),
		)
	}

	prefix := st.sectionBorder.Render("│ ")
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return head + "\n" + strings.Join(lines, "\n")
}

func renderExplainSummarySection(v explainView, width int, st explainStyles) string {
	var lines []string
	title := strings.TrimSpace(v.Title)
	if title == "" {
		title = "Request"
	}
	lines = append(lines, lipgloss.JoinHorizontal(
		lipgloss.Left,
		explainResultBadge(v.Result, st),
		" ",
		st.title.Render(title),
	))
	for _, f := range v.Summary {
		if line := renderExplainField(f, width, st.label, st.value); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func renderExplainDecisionSection(v explainView, width int, st explainStyles) string {
	var lines []string
	for _, msg := range v.Decision {
		if line := renderExplainWrapped(msg, width, st.value, ""); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func renderExplainFinalSection(v explainView, width int, st explainStyles) string {
	if v.Final == nil {
		return ""
	}
	var lines []string
	if strings.TrimSpace(v.Final.Headline) != "" {
		lines = append(lines, st.requestLine.Render(v.Final.Headline))
	}
	for _, f := range v.Final.Fields {
		if line := renderExplainField(f, width, st.label, st.value); line != "" {
			lines = append(lines, line)
		}
	}
	if len(v.Final.Details) > 0 {
		lines = append(lines, st.muted.Render("Details"))
		for _, f := range v.Final.Details {
			if line := renderExplainField(f, width, st.label, st.value); line != "" {
				lines = append(lines, line)
			}
		}
	}
	if len(v.Final.Headers) > 0 {
		lines = append(lines, st.muted.Render("Headers"))
		for _, f := range v.Final.Headers {
			if line := renderExplainField(f, width, st.headerName, st.value); line != "" {
				lines = append(lines, line)
			}
		}
	}
	if strings.TrimSpace(v.Final.BodyNote) != "" {
		lines = append(lines, st.bodyNote.Render(v.Final.BodyNote))
	}
	if strings.TrimSpace(v.Final.Body) != "" {
		lines = append(lines, renderExplainBlock(v.Final.Body, width, st.body))
	}
	if len(v.Final.Steps) > 0 {
		lines = append(lines, st.muted.Render("Steps"))
		for _, step := range v.Final.Steps {
			if line := renderExplainWrapped(step, width, st.value, "• "); line != "" {
				lines = append(lines, line)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func renderExplainStagesSection(v explainView, width int, st explainStyles) string {
	var lines []string
	for i, sv := range v.Stages {
		if i > 0 {
			lines = append(lines, "")
		}
		head := lipgloss.JoinHorizontal(
			lipgloss.Left,
			explainStageBadge(sv.Status, st),
			" ",
			st.title.Render(sv.Name),
		)
		lines = append(lines, head)
		if strings.TrimSpace(sv.Summary) != "" {
			if line := renderExplainWrapped(sv.Summary, width, st.value, "  "); line != "" {
				lines = append(lines, line)
			}
		}
		for _, ch := range sv.Changes {
			if line := renderExplainChange(ch, width, st); line != "" {
				lines = append(lines, line)
			}
		}
		for _, note := range sv.Notes {
			if line := renderExplainWrapped(note, width, st.muted, "note: "); line != "" {
				lines = append(lines, line)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func renderExplainVarsSection(v explainView, width int, st explainStyles) string {
	var lines []string
	for i, vv := range v.Vars {
		if i > 0 {
			lines = append(lines, "")
		}
		head := lipgloss.JoinHorizontal(
			lipgloss.Left,
			st.title.Render(vv.Name),
			" ",
			explainVarChip(vv, st),
		)
		if vv.Uses > 1 {
			head = lipgloss.JoinHorizontal(
				lipgloss.Left,
				head,
				" ",
				st.chipMuted.Render(fmt.Sprintf("x%d", vv.Uses)),
			)
		}
		if vv.Dynamic {
			head = lipgloss.JoinHorizontal(lipgloss.Left, head, " ", st.chipMuted.Render("dynamic"))
		}
		lines = append(lines, head)
		if !vv.Missing && strings.TrimSpace(vv.Value) != "" {
			if line := renderExplainField(
				explainField{Label: "Value", Value: vv.Value},
				width,
				st.muted,
				st.value,
			); line != "" {
				lines = append(lines, line)
			}
		}
		if strings.TrimSpace(vv.Shadowed) != "" {
			if line := renderExplainField(
				explainField{Label: "Shadowed", Value: vv.Shadowed},
				width,
				st.muted,
				st.muted,
			); line != "" {
				lines = append(lines, line)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func renderExplainWarningsSection(v explainView, width int, st explainStyles) string {
	var lines []string
	for _, msg := range v.Warnings {
		if line := renderExplainWrapped(msg, width, st.warning, "• "); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func renderExplainField(
	f explainField,
	width int,
	labelStyle lipgloss.Style,
	valueStyle lipgloss.Style,
) string {
	label := strings.TrimSpace(f.Label)
	val := strings.TrimSpace(f.Value)
	if label == "" || val == "" {
		return ""
	}
	prefix := labelStyle.Render(label + ": ")
	avail := width - visibleWidth(prefix)
	if avail < 12 {
		avail = width
	}
	segs := wrapLineSegments(val, avail)
	if len(segs) == 0 {
		segs = []string{val}
	}
	indent := strings.Repeat(" ", visibleWidth(prefix))
	lines := make([]string, 0, len(segs))
	for i, seg := range segs {
		if i == 0 {
			lines = append(lines, prefix+valueStyle.Render(seg))
			continue
		}
		lines = append(lines, indent+valueStyle.Render(seg))
	}
	return strings.Join(lines, "\n")
}

func renderExplainWrapped(text string, width int, st lipgloss.Style, prefix string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	avail := width - visibleWidth(prefix)
	if avail < 8 {
		avail = width
	}
	segs := wrapLineSegments(text, avail)
	if len(segs) == 0 {
		segs = []string{text}
	}
	indent := strings.Repeat(" ", visibleWidth(prefix))
	lines := make([]string, 0, len(segs))
	for i, seg := range segs {
		if i == 0 {
			lines = append(lines, prefix+st.Render(seg))
			continue
		}
		lines = append(lines, indent+st.Render(seg))
	}
	return strings.Join(lines, "\n")
}

func renderExplainChange(ch explainChangeView, width int, st explainStyles) string {
	sym := "~"
	symStyle := st.changeUpdate
	switch ch.Kind {
	case explainChangeAdd:
		sym = "+"
		symStyle = st.changeAdd
	case explainChangeRemove:
		sym = "-"
		symStyle = st.changeRemove
	}
	return renderExplainWrapped(ch.Text, width, st.value, symStyle.Render(sym)+" ")
}

func renderExplainBlock(text string, width int, st lipgloss.Style) string {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return ""
	}
	prefix := "  "
	avail := width - len(prefix)
	if avail < 8 {
		avail = width
	}
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		segs := wrapLineSegments(line, avail)
		if len(segs) == 0 {
			lines = append(lines, prefix)
			continue
		}
		for _, seg := range segs {
			lines = append(lines, prefix+st.Render(seg))
		}
	}
	return strings.Join(lines, "\n")
}

func explainResultBadge(result string, st explainStyles) string {
	label := strings.ToUpper(strings.TrimSpace(result))
	if label == "" {
		label = "READY"
	}
	switch strings.ToLower(label) {
	case "sent", "prepared", "ready":
		return st.badgeReady.Render(label)
	case "skipped":
		return st.badgeSkipped.Render(label)
	case "error":
		return st.badgeError.Render(label)
	default:
		return st.badgeReady.Render(label)
	}
}

func explainStageBadge(status xplain.StageStatus, st explainStyles) string {
	label := strings.ToUpper(string(status))
	if label == "" {
		label = "OK"
	}
	switch status {
	case xplain.StageSkipped:
		return st.stageSkipped.Render(label)
	case xplain.StageError:
		return st.stageError.Render(label)
	default:
		return st.stageOK.Render(label)
	}
}

func explainVarChip(v explainVarView, st explainStyles) string {
	if v.Missing {
		return st.chipMissing.Render("missing")
	}
	src := strings.TrimSpace(v.Source)
	if src == "" {
		src = "unknown"
	}
	return st.chip.Render(src)
}

func (m *Model) syncExplainPane(
	pane *responsePaneState,
	width int,
	snapshot *responseSnapshot,
) tea.Cmd {
	if pane == nil || snapshot == nil || snapshot.explain.report == nil {
		return nil
	}
	key := strings.TrimSpace(m.activeThemeKey)
	if key == "" {
		key = "default"
	}
	content := snapshot.explain.cache.styled
	if content == "" || snapshot.explain.cache.width != width || snapshot.explain.cache.themeKey != key {
		content = renderExplainStyled(snapshot.explain.report, width, m.theme)
		snapshot.explain.cache = explainRenderCache{
			styled:   content,
			width:    width,
			themeKey: key,
		}
	}
	if strings.TrimSpace(content) == "" {
		content = "<no explain>\n"
	}
	decorated := m.applyResponseContentStyles(responseTabExplain, content)
	pane.viewport.SetContent(decorated)
	pane.restoreScrollForActiveTab()
	pane.setCurrPosition()
	return nil
}
