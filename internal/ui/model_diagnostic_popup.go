package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/helpdoc"
)

const (
	diagnosticPopupWidth  = 72
	diagnosticPopupHeight = 12
)

type diagnosticPopup struct {
	open bool
	diagnosticContext
	items    []int
	topic    helpdoc.Topic
	hasTopic bool
	scroll   int
}

type diagnosticPopupLayout struct {
	box         hintOverlayBox
	page, limit int
}

func (m *Model) openDiagnosticPopup() bool {
	s := m.currentDiagnostics()
	items := s.at(m.editor.caretPosition())
	if len(items) == 0 {
		return false
	}
	topic, ok := m.contextHelpTopic()
	m.editor.closeCompletions()
	m.diagnostics.popup = diagnosticPopup{
		open: true, diagnosticContext: m.diagnosticContext(), items: items, topic: topic, hasTopic: ok,
	}
	if _, ok := m.diagnosticPopupLayout(m.editor.ViewWidth(), m.editor.Height()); !ok {
		m.openDiagnosticList(s)
	}
	return true
}

func (m *Model) handleDiagnosticPopupKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	p := &m.diagnostics.popup
	if !p.open {
		return nil, false
	}
	if m.currentDiagnostics() == nil || !m.editorIdle() || p.diagnosticContext != m.diagnosticContext() {
		*p = diagnosticPopup{}
		return nil, false
	}
	switch msg.String() {
	case "esc":
		*p = diagnosticPopup{}
	case "enter":
		topic, ok := p.topic, p.hasTopic
		*p = diagnosticPopup{}
		if ok {
			m.openHelpTopic(topic)
		}
	case "pgup", "pgdown":
		layout, _ := m.diagnosticPopupLayout(m.editor.ViewWidth(), m.editor.Height())
		delta := max(layout.page, 1)
		if msg.String() == "pgup" {
			delta = -delta
		}
		p.scroll = clamp(p.scroll+delta, 0, layout.limit)
	default:
		*p = diagnosticPopup{}
		return nil, false
	}
	// Modal returns skip clearing this flag, which would swallow the next editor key.
	if !m.mouseModalActive() {
		m.suppressEditorKey = true
	}
	return nil, true
}

func (m *Model) diagnosticPopupLines(width int) []string {
	s := m.currentDiagnostics()
	if s == nil {
		return nil
	}
	var lines []string
	for _, i := range m.diagnostics.popup.items {
		item := s.report.Items[i]
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		style, label := m.theme.EditorDiagnosticWarning, "Warning"
		if item.Severity == diag.SeverityError {
			style, label = m.theme.EditorDiagnosticError, "Error"
		}
		head := fmt.Sprintf("%s · line %d:%d", label, item.Span.Start.Line, item.Span.Start.Col)
		lines = append(lines, style.UnsetUnderline().Bold(true).Render(ansi.Truncate(head, width, "…")))
		body := ansi.Hardwrap(ansi.Wordwrap(displayLines(item.Message), width, ""), width, true)
		lines = append(lines, strings.Split(body, "\n")...)
	}
	return lines
}

// Coordinates use textarea viewport cells, before pane padding.
func (m *Model) diagnosticPopupLayout(width, height int) (diagnosticPopupLayout, bool) {
	frame := m.theme.EditorHintBox
	w := min(width, diagnosticPopupWidth)
	inner := w - frame.GetHorizontalFrameSize()
	x, y := m.editor.ViewportCursor()
	above, below := max(y, 0), max(height-y-1, 0)
	page := min(max(above, below), diagnosticPopupHeight) - frame.GetVerticalFrameSize() - 1
	if inner < 1 || page < 1 {
		return diagnosticPopupLayout{}, false
	}
	lines := m.diagnosticPopupLines(inner)
	page = min(page, len(lines))
	if page == 0 {
		return diagnosticPopupLayout{}, false
	}
	limit := max(len(lines)-page, 0)
	start := clamp(m.diagnostics.popup.scroll, 0, limit)
	footer := "Esc close"
	if m.diagnostics.popup.hasTopic {
		footer = "Enter docs · " + footer
	}
	if limit > 0 {
		footer = "PgUp/PgDn · " + footer
	}
	body := strings.Join(lines[start:start+page], "\n") + "\n" +
		m.theme.EditorHintAnnotation.Render(ansi.Truncate(footer, inner, "…"))
	box := frame.Width(w - frame.GetHorizontalBorderSize()).Render(body)
	boxW, boxH := lipgloss.Width(box), lipgloss.Height(box)
	top := y + 1
	if boxH > below {
		top = y - boxH
	}
	return diagnosticPopupLayout{
		box: hintOverlayBox{
			x: clamp(x, 0, max(width-boxW, 0)), y: clamp(top, 0, max(height-boxH, 0)),
			w: boxW, h: boxH, lines: strings.Split(box, "\n"),
		},
		page:  page,
		limit: limit,
	}, true
}

func (m *Model) renderDiagnosticPopup(content string) string {
	if !m.diagnostics.popup.open || m.currentDiagnostics() == nil {
		return content
	}
	w, h := lipgloss.Width(content), lipgloss.Height(content)
	layout, ok := m.diagnosticPopupLayout(w, h)
	if !ok {
		return content
	}
	box := layout.box
	return overlayHintPopup(content, box.lines, box.x, box.y, w, h)
}
