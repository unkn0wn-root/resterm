package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/unkn0wn-root/resterm/internal/diag"
)

func TestDiagnosticPopupKeysAndDocumentation(t *testing.T) {
	m := newDiagnosticModel(t, "# @sse max-event=5\nGET http://x")
	updateDiagnosticsModel(t, &m, keyMsgFor("K"))
	if !m.diagnostics.popup.open || m.showHelp {
		t.Fatal("K did not prefer diagnostics")
	}
	updateDiagnosticsModel(t, &m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.showHelp || m.helpTopic == nil || m.helpTopic.ID != "streaming" || m.sending {
		t.Fatal("Enter didn't open docs, or sent the request")
	}
	updateDiagnosticsModel(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	updateDiagnosticsModel(t, &m, keyMsgFor("j"))
	if m.showHelp || m.editor.Line() != 1 {
		t.Fatal("closing documentation consumed the next editor movement")
	}
	m.editor.moveCursorTo(0, 0)
	m.openDiagnosticPopup()
	updateDiagnosticsModel(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.diagnostics.popup.open {
		t.Fatal("Esc didn't dismiss")
	}
	m.openDiagnosticPopup()
	updateDiagnosticsModel(t, &m, keyMsgFor("j"))
	if m.diagnostics.popup.open || m.editor.Line() != 1 {
		t.Fatalf(
			"movement wasn't passed through: line=%d popup=%t focused=%t suppressed=%t",
			m.editor.Line(),
			m.diagnostics.popup.open,
			m.editor.Focused(),
			m.suppressEditorKey,
		)
	}
}

func TestDiagnosticPopupConsumesEnterWithoutTopic(t *testing.T) {
	m := newDiagnosticModel(t, "# @nmae typo\nGET http://x")
	m.openDiagnosticPopup()
	if m.diagnostics.popup.hasTopic {
		t.Fatal("unknown directive has unrelated docs")
	}
	updateDiagnosticsModel(t, &m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.showHelp || m.sending || m.diagnostics.popup.open {
		t.Fatal("Enter escaped the popup")
	}
}

func TestDiagnosticPopupFitsViewportAndScrolls(t *testing.T) {
	for _, width := range []int{24, 80} {
		for _, row := range []int{0, 7} {
			m := newDiagnosticModel(t, strings.Repeat("line\n", 8))
			m.editor.SetWidth(width)
			m.editor.SetHeight(9)
			m.editor.moveCursorTo(row, 3)
			rep := diag.Report{Source: []byte(m.editor.Value()), Items: []diag.Diagnostic{{
				Severity: diag.SeverityWarning, Message: strings.Repeat("long diagnostic 漢字 explanation ", 40),
				Span: diag.Span{Start: diag.Pos{Line: row + 1, Col: 1}, End: diag.Pos{Line: row + 1, Col: 5}},
			}}}
			m.publishDiagnostics(newDiagnosticSnapshot(m.diagnosticKey(), rep))
			if !m.openDiagnosticPopup() {
				t.Fatal("no popup")
			}
			base := m.editor.View()
			w, h := lipgloss.Width(base), lipgloss.Height(base)
			layout, ok := m.diagnosticPopupLayout(w, h)
			box, limit := layout.box, layout.limit
			if !ok || box.x < 0 || box.y < 0 || box.x+box.w > w || box.y+box.h > h || box.w > diagnosticPopupWidth ||
				box.h > diagnosticPopupHeight {
				t.Fatalf("box=%+v viewport=%dx%d", box, w, h)
			}
			view := m.renderDiagnosticPopup(base)
			if lipgloss.Width(view) != w || lipgloss.Height(view) != h {
				t.Fatal("popup changed layout")
			}
			if !strings.Contains(ansi.Strip(view), "Warning") {
				t.Fatal("no severity label")
			}
			for range 100 {
				m.handleDiagnosticPopupKey(tea.KeyMsg{Type: tea.KeyPgDown})
			}
			if limit == 0 || m.diagnostics.popup.scroll != limit {
				t.Fatal("cannot reach end of long message")
			}
			for range 100 {
				m.handleDiagnosticPopupKey(tea.KeyMsg{Type: tea.KeyPgUp})
			}
			if m.diagnostics.popup.scroll != 0 {
				t.Fatal("cannot return to top")
			}
		}
	}
}

func TestDiagnosticPopupGroupsMultipleFindingsAndUsesContinuationDocs(t *testing.T) {
	m := newDiagnosticModel(
		t,
		"# @mock method=GET path=/x\n# @match query={\n# \"x\":\"y\"} typo=a typo=b\nHTTP/1.1 200 OK",
	)
	m.editor.moveCursorTo(2, 0)
	m.openDiagnosticPopup()
	if !m.diagnostics.popup.open || len(m.diagnostics.popup.items) != 2 || !m.diagnostics.popup.hasTopic ||
		m.diagnostics.popup.topic.ID != "mocks" {
		t.Fatalf("popup = %+v", m.diagnostics.popup)
	}
	lines := strings.Join(m.diagnosticPopupLines(60), "\n")
	if strings.Count(lines, "is repeated") != 1 || strings.Count(lines, "unknown @match") != 1 {
		t.Fatalf("duplicated findings: %q", lines)
	}
}

func TestContextHelpDoesNotInterpretScriptBodyAsDirective(t *testing.T) {
	m := newDiagnosticModel(t, "GET http://x\n> {%\n// @auth bearer token\n> %}")
	m.editor.moveCursorTo(2, 5)
	if topic, ok := m.contextHelpTopic(); ok {
		t.Fatalf("literal resolved to %+v", topic)
	}
}
