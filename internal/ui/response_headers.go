package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
)

const (
	headerResponseLabel = "Response"
	headerRequestLabel  = "Request"
)

func (m *Model) renderHeaderSubviewSwitch(pane *responsePaneState) string {
	if pane == nil {
		return ""
	}
	w := headerSwitchWidth(headerResponseLabel, headerRequestLabel)
	resCell := headerSwitchCell(headerResponseLabel, w)
	reqCell := headerSwitchCell(headerRequestLabel, w)
	active := headerSwitchStyle(m.theme.TabActive)
	inactive := headerSwitchStyle(m.theme.TabInactive)

	res := inactive.Render(resCell)
	req := inactive.Render(reqCell)
	if pane.headersView == headersViewRequest {
		req = active.Render(reqCell)
	} else {
		res = active.Render(resCell)
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		res,
		m.theme.PaneDivider.Render("│"),
		req,
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

func headerSwitchStyle(st lipgloss.Style) lipgloss.Style {
	return st.
		UnsetWidth().
		UnsetMaxWidth().
		UnsetMargins().
		Padding(0, 0)
}

func headerSwitchWidth(labels ...string) int {
	w := 0
	for _, label := range labels {
		if v := lipgloss.Width(label); v > w {
			w = v
		}
	}
	return w
}

func headerSwitchCell(label string, w int) string {
	if v := lipgloss.Width(label); w < v {
		w = v
	}
	w += 2
	pad := w - lipgloss.Width(label)
	left := pad / 2
	right := pad - left
	return strings.Repeat(" ", left) + label + strings.Repeat(" ", right)
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
		if strings.TrimSpace(snap.requestHeaders) == "" {
			return "<no request headers>\n"
		}
		return snap.requestHeaders
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

	if strings.TrimSpace(snap.headers) == "" {
		return "<no headers>\n"
	}
	return snap.headers
}
