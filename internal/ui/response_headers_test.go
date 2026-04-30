package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestHeadersDisplayShowsSwitcher(t *testing.T) {
	snap := &responseSnapshot{
		headers:        withTrailingNewline("Response headers\nX-Resp: ok"),
		requestHeaders: withTrailingNewline("Request headers\nCookie: demo=1"),
		ready:          true,
	}
	model := newModelWithResponseTab(responseTabHeaders, snap)
	pane := model.pane(responsePanePrimary)

	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		_ = cmd()
	}

	view := stripANSIEscape(pane.viewport.View())
	if !strings.Contains(view, "Response") || !strings.Contains(view, "Request") {
		t.Fatalf("expected response/request switcher, got %q", view)
	}
	if !strings.Contains(view, "X-Resp: ok") {
		t.Fatalf("expected response headers in default subview, got %q", view)
	}
	if strings.Contains(view, "Cookie: demo=1") {
		t.Fatalf("unexpected request headers in default subview, got %q", view)
	}
}

func TestHeadersSubviewSwitchesWithEnterAndSpace(t *testing.T) {
	snap := &responseSnapshot{
		headers:        withTrailingNewline("Response headers\nX-Resp: ok"),
		requestHeaders: withTrailingNewline("Request headers\nCookie: demo=1"),
		ready:          true,
	}
	model := newModelWithResponseTab(responseTabHeaders, snap)
	pane := model.pane(responsePanePrimary)

	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if pane.headersView != headersViewRequest {
		t.Fatalf("expected request header subview")
	}
	view := stripANSIEscape(pane.viewport.View())
	if !strings.Contains(view, "Cookie: demo=1") {
		t.Fatalf("expected request headers after enter, got %q", view)
	}

	_ = model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if pane.headersView != headersViewResponse {
		t.Fatalf("expected response header subview")
	}
	view = stripANSIEscape(pane.viewport.View())
	if !strings.Contains(view, "X-Resp: ok") {
		t.Fatalf("expected response headers after space, got %q", view)
	}
}

func TestHeadersSwitchDoesNotWrapWithPaddedTabStyles(t *testing.T) {
	model := New(Config{})
	model.theme.TabActive = model.theme.TabActive.Padding(0, 4).Width(8)
	model.theme.TabInactive = model.theme.TabInactive.Padding(0, 3).Width(7)
	pane := model.pane(responsePanePrimary)
	pane.headersView = headersViewResponse

	view := stripANSIEscape(model.renderHeaderSubviewSwitch(pane))
	if strings.Contains(view, "\n") {
		t.Fatalf("expected one-line header switch, got %q", view)
	}

	parts := strings.Split(view, "│")
	if len(parts) != 2 {
		t.Fatalf("expected two switch cells, got %q", view)
	}
	left := strings.TrimRight(parts[0], " ")
	right := strings.TrimLeft(parts[1], " ")
	if lipgloss.Width(left) != lipgloss.Width(right) {
		t.Fatalf("expected equal cell widths, got %q (%d) and %q (%d)",
			left,
			lipgloss.Width(left),
			right,
			lipgloss.Width(right),
		)
	}
	if !strings.Contains(left, "Response") || !strings.Contains(right, "Request") {
		t.Fatalf("expected complete labels, got %q", view)
	}
}

func TestHeaderSwitchCellPadsPlainTextBeforeStyling(t *testing.T) {
	cell := headerSwitchCell("Request", lipgloss.Width("Response"))
	if got := lipgloss.Width(cell); got != lipgloss.Width("Response")+2 {
		t.Fatalf("expected padded cell width 10, got %d (%q)", got, cell)
	}
	if !strings.HasPrefix(cell, " ") || !strings.HasSuffix(cell, "  ") {
		t.Fatalf("expected centered padding around shorter label, got %q", cell)
	}
}
