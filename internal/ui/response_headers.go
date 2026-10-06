package ui

import (
	"net/http"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	headerResponseLabel = "Response"
	headerRequestLabel  = "Request"
	headerActiveMark    = "●"
)

func (m *Model) renderHeaderSubviewSwitch(pane *responsePaneState) string {
	if pane == nil {
		return ""
	}
	sep := " " + m.theme.PaneDivider.Render("│") + " "
	return strings.Join([]string{
		m.renderHeaderSwitchItem(pane, headersViewResponse),
		m.renderHeaderSwitchItem(pane, headersViewRequest),
	}, sep)
}

func (m *Model) renderHeaderSubviewHead(pane *responsePaneState, width int) string {
	sw := m.renderHeaderSubviewSwitch(pane)
	if sw == "" {
		return ""
	}
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	rule := m.theme.PaneDivider.Render(strings.Repeat("─", width))
	return sw + "\n" + rule
}

// Only the inactive side shows a count. The active side has it in its rule.
func (m *Model) renderHeaderSwitchItem(pane *responsePaneState, view headersViewMode) string {
	label := headerResponseLabel
	if view == headersViewRequest {
		label = headerRequestLabel
	}
	active := pane.headersView == view
	item := headerSwitchStyle(m.theme, active).Render(headerSwitchText(label, active))
	if active {
		return item
	}
	if n, ok := headerRows(pane.snapshot, view); ok {
		item += " " + m.themeRuntime.subtleTextStyle(m.theme).Render(strconv.Itoa(n))
	}
	return item
}

func headerSwitchText(label string, active bool) string {
	if active {
		return headerActiveMark + " " + label
	}
	return label
}

// Take only the text color from the tab styles, not their padding or borders.
func headerSwitchStyle(th theme.Theme, active bool) lipgloss.Style {
	if active {
		return lipgloss.NewStyle().Foreground(th.TabActive.GetForeground()).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(th.TabInactive.GetForeground()).Faint(true)
}

func (m *Model) cycleHeaderSubview() tea.Cmd {
	m.ensurePaneFocusValid()
	pane := m.focusedPane()
	if !headerSubviewAvailable(pane) {
		return nil
	}

	pane.setCurrPosition()
	next := headersViewRequest
	note := "Headers: request"
	if pane.headersView == headersViewRequest {
		next = headersViewResponse
		note = "Headers: response"
	}
	pane.setHeadersView(next)
	pane.restoreScrollForActiveTab()
	pane.setCurrPosition()

	return batchCommands(
		m.syncResponsePane(m.responsePaneFocus),
		func() tea.Msg { return statusMsg{text: note, level: statusInfo} },
	)
}

func (m *Model) activateHeaderSubviewFromBinding() tea.Cmd {
	focusCmd := m.setFocus(focusResponse)
	m.ensurePaneFocusValid()

	pane := m.focusedPane()
	if pane == nil {
		return batchCommands(
			focusCmd,
			func() tea.Msg { return statusMsg{text: "Response pane unavailable", level: statusWarn} },
		)
	}
	if pane.snapshot == nil || !pane.snapshot.ready {
		return batchCommands(
			focusCmd,
			func() tea.Msg { return statusMsg{text: "No response available", level: statusWarn} },
		)
	}
	if pane.activeTab != responseTabHeaders {
		pane.setActiveTab(responseTabHeaders)
	}
	return batchCommands(focusCmd, m.cycleHeaderSubview())
}

func headerSubviewAvailable(pane *responsePaneState) bool {
	if pane == nil || pane.activeTab != responseTabHeaders {
		return false
	}
	return pane.snapshot != nil && pane.snapshot.ready
}

func headerSnap(s *responseSnapshot, view headersViewMode) string {
	if s == nil {
		return ""
	}
	if view == headersViewRequest {
		if strings.TrimSpace(s.requestHeaders) == "" {
			return "<no request headers>\n"
		}
		return s.requestHeaders
	}
	if strings.TrimSpace(s.headers) == "" {
		return "<no headers>\n"
	}
	return s.headers
}

func headerCopy(s *responseSnapshot, view headersViewMode) string {
	if h := headerMap(s, view); len(h) > 0 {
		return bodyfmt.FormatHeaders(h)
	}
	return headerSnap(s, view)
}

func headerMap(s *responseSnapshot, view headersViewMode) http.Header {
	if s == nil {
		return nil
	}
	if view == headersViewRequest {
		switch {
		case s.source.hasHTTP():
			return s.source.http.SentHeaders()
		case s.source.hasGRPC():
			return grpcRequestHeaderMap(s.source.grpcReq)
		default:
			return nil
		}
	}
	switch {
	case s.source.hasHTTP():
		return s.source.http.Headers
	case s.source.hasGRPC():
		return s.source.grpc.HeaderMap()
	case len(s.responseHeaders) > 0:
		return s.responseHeaders
	default:
		return nil
	}
}

// headerMap adds a Content-Type to gRPC metadata that the panels do not show,
// so gRPC counts the metadata it received.
func headerRows(s *responseSnapshot, view headersViewMode) (int, bool) {
	if s != nil && view == headersViewResponse && s.source.hasGRPC() {
		return valueCount(s.source.grpc.Headers) + valueCount(s.source.grpc.Trailers), true
	}
	h := headerMap(s, view)
	return valueCount(h), h != nil
}

func valueCount(h map[string][]string) int {
	n := 0
	for _, vals := range h {
		n += len(vals)
	}
	return n
}

func (m *Model) headerContent(pane *responsePaneState, width int) string {
	if pane == nil || pane.snapshot == nil || !pane.snapshot.ready {
		return ""
	}
	snap := pane.snapshot
	r := m.themeRuntime.responseRenderer(m.theme)

	if pane.headersView == headersViewRequest {
		switch {
		case snap.source.hasHTTP():
			return r.renderHTTPReqHdrs(snap.source.http, width)
		case snap.source.hasGRPC():
			return r.renderGRPCReqHdrs(snap.source.grpcReq, width)
		}
		return headerSnap(snap, pane.headersView)
	}

	switch {
	case snap.source.hasHTTP():
		return r.renderHTTPRespHdrs(
			snap.source.http,
			snap.source.tests,
			snap.source.scriptErr,
			width,
		)
	case snap.source.hasGRPC():
		return r.renderGRPCRespHdrs(
			snap.source.grpc,
			snap.source.grpcMethod,
			width,
		)
	case len(snap.responseHeaders) > 0:
		return r.renderHdrDoc("", []hdrPanel{{
			fields: bodyfmt.HeaderFields(snap.responseHeaders),
			empty:  "No response headers captured",
		}}, width)
	}

	return headerSnap(snap, pane.headersView)
}
