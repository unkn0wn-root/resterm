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

func TestHeadersSwitchUsesMarkerAndIgnoresTabStyles(t *testing.T) {
	model := New(Config{})
	model.theme.TabActive = model.theme.TabActive.
		Padding(0, 4).
		Width(8).
		Background(lipgloss.Color("9"))
	model.theme.TabInactive = model.theme.TabInactive.
		Padding(0, 3).
		Width(7).
		Background(lipgloss.Color("8"))
	pane := model.pane(responsePanePrimary)
	pane.headersView = headersViewResponse

	view := stripANSIEscape(model.renderHeaderSubviewSwitch(pane))
	if strings.Contains(view, "\n") {
		t.Fatalf("expected one-line header switch, got %q", view)
	}
	if view != "● Response │ Request" {
		t.Fatalf("expected response marker switch, got %q", view)
	}

	pane.headersView = headersViewRequest
	view = stripANSIEscape(model.renderHeaderSubviewSwitch(pane))
	if strings.Contains(view, "\n") {
		t.Fatalf("expected one-line header switch, got %q", view)
	}
	if view != "Response │● Request" {
		t.Fatalf("expected request marker switch, got %q", view)
	}
}

func TestHeaderSwitchTextMarksActiveItem(t *testing.T) {
	if got := headerSwitchText("Response", true); got != "● Response" {
		t.Fatalf("expected active marker item, got %q", got)
	}
	if got := headerSwitchText("Request", false); got != "Request" {
		t.Fatalf("expected inactive marker spacing, got %q", got)
	}
}

func TestHeaderSwitchStyleUsesTabTextStateOnly(t *testing.T) {
	model := New(Config{})
	active := headerSwitchStyle(model.theme, true)
	inactive := headerSwitchStyle(model.theme, false)

	if got := active.GetForeground(); got != model.theme.TabActive.GetForeground() {
		t.Fatalf("expected active tab foreground, got %v", got)
	}
	if _, ok := active.GetBackground().(lipgloss.NoColor); !ok {
		t.Fatalf("expected active background to be unset, got %v", active.GetBackground())
	}
	if !active.GetBold() {
		t.Fatalf("expected active item to be bold")
	}
	if active.GetFaint() {
		t.Fatalf("expected active item not to be faint")
	}

	if got := inactive.GetForeground(); got != model.theme.TabInactive.GetForeground() {
		t.Fatalf("expected inactive tab foreground, got %v", got)
	}
	if _, ok := inactive.GetBackground().(lipgloss.NoColor); !ok {
		t.Fatalf("expected inactive background to be unset, got %v", inactive.GetBackground())
	}
	if inactive.GetBold() {
		t.Fatalf("expected inactive item not to be bold")
	}
	if !inactive.GetFaint() {
		t.Fatalf("expected inactive item to be faint")
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
