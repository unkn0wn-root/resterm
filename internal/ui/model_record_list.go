package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/recorder"
)

func (m *Model) openRecordList() tea.Cmd {
	m.record.showList = true
	m.syncRecordList()
	return nil
}

func (m *Model) closeRecordList() { m.record.showList = false }

func (m *Model) syncRecordList() {
	if m.record.viewport == nil {
		v := viewport.New(0, 0)
		m.record.viewport = &v
	}

	var b strings.Builder
	b.WriteString(m.recordStatus() + "\n\n")
	if s := m.record.session; s != nil {
		for _, e := range s.Summaries() {
			m.writeRecordRow(&b, e)
		}
	}
	m.record.viewport.SetContent(b.String())
}

func (m *Model) writeRecordRow(b *strings.Builder, e recorder.Entry) {
	fmt.Fprintf(b, "%d  %s  %d  %s\n", e.ID, e.Method, e.Status, oneLine(e.URL))
	if e.Request.Issue != "" {
		fmt.Fprintf(b, "  Request unavailable: %s\n", e.Request.Issue)
	}
	if e.Response.Issue != "" {
		fmt.Fprintf(b, "  Mock unavailable: %s\n", e.Response.Issue)
	}
	for _, note := range m.record.notes[e.ID] {
		fmt.Fprintf(b, "  %s\n", note)
	}
}

func (m Model) renderRecordList() string {
	size := m.modalSize(120, 30)
	v := m.record.viewport
	if v == nil {
		return "No recordings"
	}

	v.Width, v.Height = size.view, size.body
	body := lipgloss.NewStyle().Padding(0, 2).Width(size.content).Render(v.View())
	return m.renderModalBox(
		"Recorded Traffic",
		body,
		"Esc Close · j/k Scroll · :record as-request|as-mock <id>",
		size.width,
	)
}
