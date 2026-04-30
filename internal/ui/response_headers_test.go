package ui

import (
	"net/http"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
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

func TestHeadersDisplayShowsWidthAwareDividerBelowSwitcher(t *testing.T) {
	snap := &responseSnapshot{
		headers: withTrailingNewline("Status: 200 OK\nX-Resp: ok"),
		ready:   true,
	}
	model := newModelWithResponseTab(responseTabHeaders, snap)
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 27

	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		_ = cmd()
	}

	lines := strings.Split(stripANSIEscape(pane.viewport.View()), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected switcher, divider, and content, got %q", pane.viewport.View())
	}
	if !strings.Contains(lines[0], "Response") || !strings.Contains(lines[0], "Request") {
		t.Fatalf("expected switcher on first line, got %q", lines[0])
	}
	if got := lipgloss.Width(lines[1]); got != pane.viewport.Width {
		t.Fatalf("expected divider width %d, got %d in %q", pane.viewport.Width, got, lines[1])
	}
	if strings.Trim(lines[1], "─") != "" {
		t.Fatalf("expected divider to contain only rule characters, got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "Status:") {
		t.Fatalf("expected content immediately after divider, got %q", lines[2])
	}
}

func TestHeadersDisplayRendersCountRuleToPaneWidth(t *testing.T) {
	resp := &httpclient.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Headers: http.Header{
			"X-Long": {strings.Repeat("v", 80)},
		},
	}
	snap := &responseSnapshot{
		ready:  true,
		source: newHTTPResponseRenderSource(resp, nil, nil),
	}
	model := newModelWithResponseTab(responseTabHeaders, snap)
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 34

	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		_ = cmd()
	}

	for _, line := range strings.Split(stripANSIEscape(pane.viewport.View()), "\n") {
		if !strings.Contains(line, "1 HEADER") {
			continue
		}
		if got := lipgloss.Width(line); got != pane.viewport.Width {
			t.Fatalf("expected count rule width %d, got %d in %q", pane.viewport.Width, got, line)
		}
		if !strings.Contains(line, "─") {
			t.Fatalf("expected count embedded in rule, got %q", line)
		}
		return
	}
	t.Fatalf("expected count rule in %q", pane.viewport.View())
}

func TestHeadersDisplayIndentsWrappedValues(t *testing.T) {
	resp := &httpclient.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Headers: http.Header{
			"X-Trace": {"alpha beta gamma delta"},
		},
	}
	snap := &responseSnapshot{
		ready:  true,
		source: newHTTPResponseRenderSource(resp, nil, nil),
	}
	model := newModelWithResponseTab(responseTabHeaders, snap)
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 22

	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		_ = cmd()
	}

	lines := strings.Split(stripANSIEscape(pane.viewport.View()), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "X-Trace: ") {
			continue
		}
		if i+1 >= len(lines) {
			t.Fatalf("expected continuation after %q in %q", line, pane.viewport.View())
		}
		indent := strings.Repeat(" ", len("X-Trace: "))
		if !strings.HasPrefix(lines[i+1], indent) {
			t.Fatalf("expected continuation indent %q, got %q", indent, lines[i+1])
		}
		return
	}
	t.Fatalf("expected wrapped X-Trace row in %q", pane.viewport.View())
}
