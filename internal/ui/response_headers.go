package ui

import (
	"net/http"
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
	reqActive := pane.headersView == headersViewRequest
	sep := " " + m.theme.PaneDivider.Render("│")
	if !reqActive {
		sep += " "
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderHeaderSwitchItem(headerResponseLabel, pane.headersView == headersViewResponse),
		sep,
		m.renderHeaderSwitchItem(headerRequestLabel, reqActive),
	)
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

func (m *Model) renderHeaderSwitchItem(label string, active bool) string {
	return headerSwitchStyle(m.theme, active).Render(headerSwitchText(label, active))
}

func headerSwitchText(label string, active bool) string {
	if active {
		return headerActiveMark + " " + label
	}
	return label
}

func headerSwitchStyle(th theme.Theme, active bool) lipgloss.Style {
	st := th.TabInactive
	if active {
		st = th.TabActive
	}
	st = st.
		UnsetBackground().
		UnsetWidth().
		UnsetMaxWidth().
		UnsetMargins().
		Padding(0, 0)
	if active {
		return st.Bold(true).Faint(false)
	}
	return st.Bold(false).Faint(true)
}

func (m *Model) cycleHeaderSubview() tea.Cmd {
	m.ensurePaneFocusValid()
	paneID := m.responsePaneFocus
	if !m.responseSplit {
		paneID = responsePanePrimary
	}
	pane := m.pane(paneID)
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
		m.syncResponsePane(paneID),
		func() tea.Msg { return statusMsg{text: note, level: statusInfo} },
	)
}

func (m *Model) activateHeaderSubviewFromBinding() tea.Cmd {
	focusCmd := m.setFocus(focusResponse)
	m.ensurePaneFocusValid()

	paneID := m.responsePaneFocus
	if !m.responseSplit {
		paneID = responsePanePrimary
	}
	pane := m.pane(paneID)
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
			return buildRequestHeaderMap(s.source.http)
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
		return grpcResponseHeaderMap(s.source.grpc)
	case len(s.responseHeaders) > 0:
		return s.responseHeaders
	default:
		return nil
	}
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
