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

func TestResponseCursorNoopWithoutCursorOrSelection(t *testing.T) {
	model := New(Config{})
	model.ready = true

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

	got := model.decorateResponseCursor(pane, responseTabPretty, content)
	if got != content {
		t.Fatalf("expected cursor decoration to be a no-op, got %q", got)
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

func TestResponseCursorDecoratesInline(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	model := New(Config{})
	model.ready = true
	model.theme.ResponseContent = lipgloss.NewStyle()
	model.theme.ResponseCursor = lipgloss.NewStyle().Background(lipgloss.Color("#00ff00"))

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
	if stripped := stripANSIEscape(got); stripped != content {
		t.Fatalf("expected cursor to avoid extra width, got %q", stripped)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected cursor styling to emit ANSI codes")
	}
}

func TestResponseCursorDecoratesEmptyLine(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	model := New(Config{})
	model.ready = true
	model.theme.ResponseContent = lipgloss.NewStyle()
	model.theme.ResponseCursor = lipgloss.NewStyle().Background(lipgloss.Color("#00ff00"))

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 10
	pane.snapshot = &responseSnapshot{ready: true, id: "snap"}

	content := "\n"
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, 80),
	)
	pane.cursor = respCursor{
		on:   true,
		line: 0,
		tab:  responseTabPretty,
		sid:  "snap",
	}

	got := model.decorateResponseCursor(pane, responseTabPretty, content)
	lines := strings.Split(got, "\n")
	if len(lines) == 0 {
		t.Fatalf("expected decorated content to keep empty line")
	}
	if stripped := stripANSIEscape(lines[0]); stripped != " " {
		t.Fatalf("expected cursor to render a single cell on empty line, got %q", stripped)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected cursor styling to emit ANSI codes")
	}
}

func TestResponseSelectionScrollsSmoothlyOnLongLine(t *testing.T) {
	model := New(Config{})
	model.ready = true

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabRaw
	pane.viewport.Width = 8
	pane.viewport.Height = 3
	pane.snapshot = &responseSnapshot{ready: true, id: "snap", rawMode: rawViewText}

	content := "one\n" + strings.Repeat("a", 30) + "\nthree"
	pane.rawWrapCache = map[rawViewMode]cachedWrap{
		rawViewText: wrapCache(
			responseTabRaw,
			content,
			responseWrapWidth(responseTabRaw, pane.viewport.Width),
		),
	}
	pane.sel = respSel{
		on:   true,
		a:    0,
		c:    0,
		tab:  responseTabRaw,
		sid:  "snap",
		mode: rawViewText,
	}

	prevOff := pane.viewport.YOffset
	_ = model.moveRespSel(pane, 1)

	cache := pane.rawWrapCache[rawViewText]
	if !model.selValid(pane, responseTabRaw) {
		t.Fatal("expected selection to be active")
	}
	span := cache.spans[pane.sel.c]
	expected := spanOffsetWithBufDir(
		span,
		prevOff,
		pane.viewport.Height,
		len(cache.rev),
		respScrollBuf(pane.viewport.Height),
		1,
	)
	if pane.viewport.YOffset != expected {
		t.Fatalf("expected smooth scroll offset %d, got %d", expected, pane.viewport.YOffset)
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

func TestResponseCursorStartsAtSafeRow(t *testing.T) {
	model := New(Config{})
	model.ready = true

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 80
	pane.viewport.Height = 10
	pane.viewport.SetYOffset(1)
	pane.snapshot = &responseSnapshot{ready: true, id: "snap"}

	lines := make([]string, 12)
	for i := range lines {
		lines[i] = "line"
	}
	content := strings.Join(lines, "\n")
	cache := wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, 80),
	)
	pane.wrapCache[responseTabPretty] = cache

	_ = model.startRespCursor(pane)
	if !pane.cursor.on {
		t.Fatal("expected cursor to be active")
	}
	row := seedRow(
		pane.viewport.YOffset,
		pane.viewport.Height,
		len(cache.rev),
		respScrollBuf(pane.viewport.Height),
	)
	expected := cache.rev[row]
	if pane.cursor.line != expected {
		t.Fatalf("expected cursor at line %d, got %d", expected, pane.cursor.line)
	}
}
