package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestResponseSelectionDoesNotShiftLines(t *testing.T) {
	model := New(Config{})
	model.ready = true

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 10
	pane.snapshot = &responseSnapshot{ready: true, id: "snap"}

	content := "abc\ndef"
	pane.wrapCache[responseTabPretty] = wrapCache(responseTabPretty, content, 80)
	pane.sel = respSel{
		on:  true,
		a:   0,
		c:   0,
		tab: responseTabPretty,
		sid: "snap",
	}

	got := model.decorateResponseSelection(pane, responseTabPretty, content)
	if stripped := stripANSIEscape(got); stripped != content {
		t.Fatalf("expected content unchanged after selection, got %q", stripped)
	}
}

func TestResponseSelectionRestoresBaseStyleAfterLine(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.theme.ResponseContent = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff00ff"))
	model.theme.ResponseSelection = lipgloss.NewStyle().Background(lipgloss.Color("#00ff00"))

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 10
	pane.snapshot = &responseSnapshot{ready: true, id: "snap"}

	content := "one\ntwo"
	pane.wrapCache[responseTabPretty] = wrapCache(responseTabPretty, content, 80)
	pane.sel = respSel{
		on:  true,
		a:   0,
		c:   0,
		tab: responseTabPretty,
		sid: "snap",
	}

	styled := model.applyResponseContentStyles(responseTabPretty, content)
	got := model.decorateResponseSelection(pane, responseTabPretty, styled)

	_, selSuffix := styleSGR(model.respSelStyle(responseTabPretty))
	basePrefix, _ := styleSGR(model.respBaseStyle(responseTabPretty))
	if selSuffix == "" || basePrefix == "" {
		t.Fatalf("expected selection and base styles to emit SGR sequences")
	}
	expected := selSuffix + basePrefix + "\n" + "two"
	if !strings.Contains(got, expected) {
		t.Fatalf("expected base style to resume after selection line, got %q", got)
	}
}
