package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, 80),
	)
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
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

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
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, 80),
	)
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
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least two lines, got %q", got)
	}
	if !strings.HasSuffix(lines[0], selSuffix+basePrefix) {
		t.Fatalf("expected base style to resume after selection line, got %q", got)
	}
	if !strings.HasPrefix(lines[1], basePrefix) {
		t.Fatalf("expected base style prefix on following line, got %q", got)
	}
	if stripped := stripANSIEscape(lines[1]); stripped != "two" {
		t.Fatalf("expected follow-up line text %q, got %q", "two", stripped)
	}
}

func TestResponseCursorDecoratesGutter(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.theme.ResponseContent = lipgloss.NewStyle()
	model.theme.ResponseCursor = lipgloss.NewStyle()

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 10
	pane.snapshot = &responseSnapshot{ready: true, id: "snap"}

	content := "one\ntwo\nthree"
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, 80),
	)
	pane.cursor = respCursor{
		on:   true,
		line: 1,
		tab:  responseTabPretty,
		sid:  "snap",
	}

	got := model.decorateResponseCursor(pane, responseTabPretty, content)
	want := " one\n▌two\n three"
	if got != want {
		t.Fatalf("expected cursor gutter %q, got %q", want, got)
	}
}

func TestResponseSelectionStartsAtCursorLine(t *testing.T) {
	model := New(Config{})
	model.ready = true

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 10
	pane.snapshot = &responseSnapshot{ready: true, id: "snap"}

	content := "one\ntwo\nthree"
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, 80),
	)
	pane.cursor = respCursor{
		on:   true,
		line: 2,
		tab:  responseTabPretty,
		sid:  "snap",
	}

	_ = model.startRespSel(pane)
	if !pane.sel.on {
		t.Fatal("expected selection to start")
	}
	if pane.sel.a != 2 || pane.sel.c != 2 {
		t.Fatalf("expected selection at line 2, got anchor=%d caret=%d", pane.sel.a, pane.sel.c)
	}
}

func TestMoveResponseCursorSeedsFromTop(t *testing.T) {
	model := New(Config{})
	model.ready = true

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 10
	pane.snapshot = &responseSnapshot{ready: true, id: "snap"}

	content := "one\ntwo\nthree"
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, 80),
	)

	_ = model.moveRespCursor(pane, 1)
	if !pane.cursor.on {
		t.Fatal("expected cursor to be active")
	}
	if pane.cursor.line != 0 {
		t.Fatalf("expected cursor line 0, got %d", pane.cursor.line)
	}
}

func TestMoveResponseCursorSeedsFromBottom(t *testing.T) {
	model := New(Config{})
	model.ready = true

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 10
	pane.snapshot = &responseSnapshot{ready: true, id: "snap"}

	content := "one\ntwo\nthree"
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, 80),
	)

	_ = model.moveRespCursor(pane, -1)
	if !pane.cursor.on {
		t.Fatal("expected cursor to be active")
	}
	if pane.cursor.line != 2 {
		t.Fatalf("expected cursor line 2, got %d", pane.cursor.line)
	}
}
