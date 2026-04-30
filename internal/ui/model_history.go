package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"google.golang.org/grpc/codes"
)

const responseLoadingTickInterval = 200 * time.Millisecond

type responseLoadingTickMsg struct{}

func (m *Model) handleResponseMessage(msg responseMsg) tea.Cmd {
	m.recordResponseLatency(msg)

	if state := m.compareRun; state != nil {
		if !state.core &&
			(state.matches(msg.executed) || (msg.executed == nil && state.current != nil)) {
			return m.handleCompareUIDrivenResponse(msg)
		}
	}
	if state := m.workflowRun; state != nil {
		if !state.core &&
			(state.matches(msg.executed) || (msg.executed == nil && state.current != nil)) {
			return m.handleWorkflowUIDrivenResponse(msg)
		}
	}
	if state := m.profileRun; state != nil {
		if !state.core &&
			(state.matches(msg.executed) || (msg.executed == nil && state.current != nil)) {
			return m.handleProfileUIDrivenResponse(msg)
		}
	}

	m.lastError = nil
	m.testResults = msg.tests
	m.scriptError = msg.scriptErr

	if msg.preview {
		m.lastError = nil
		m.lastResponse = nil
		m.lastGRPC = nil
		m.testResults = nil
		m.scriptError = nil
		return m.consumeExplainPreview(msg.environment, msg.explain)
	}

	if msg.skipped {
		m.lastError = nil
		m.testResults = nil
		m.scriptError = nil
		cmd := m.consumeSkippedRequest(msg.skipReason, msg.explain)
		if msg.historyDone {
			m.syncRecordedHistory()
		} else {
			m.recordSkippedHistory(
				msg.executed,
				msg.requestText,
				msg.environment,
				msg.skipReason,
				msg.runtimeSecrets...,
			)
		}
		return cmd
	}

	if msg.grpc != nil {
		if msg.err != nil {
			m.lastError = msg.err
		} else {
			m.lastError = nil
		}
		cmd := m.consumeGRPCResponse(
			msg.grpc,
			msg.tests,
			msg.scriptErr,
			msg.executed,
			msg.environment,
			msg.explain,
		)
		if msg.historyDone {
			m.syncRecordedHistory()
		} else {
			m.recordGRPCHistory(
				msg.grpc,
				msg.executed,
				msg.requestText,
				msg.environment,
				msg.runtimeSecrets...,
			)
		}
		return cmd
	}

	if msg.err != nil {
		canceled := errors.Is(msg.err, context.Canceled)
		if !canceled {
			m.lastError = msg.err
		} else {
			m.lastError = nil
		}
		m.lastResponse = nil
		m.lastGRPC = nil

		code := errdef.CodeOf(msg.err)
		level := statusError
		if code == errdef.CodeScript || code == errdef.CodeCanceled || canceled {
			level = statusWarn
		}

		text := msg.err.Error()
		if canceled {
			text = "Request canceled"
		}

		cmd := m.consumeRequestError(msg.err, msg.explain)
		m.suppressNextErrorModal = true
		m.setStatusMessage(statusMsg{text: text, level: level})
		return cmd
	}

	cmd := m.consumeHTTPResponse(
		msg.response,
		msg.tests,
		msg.scriptErr,
		msg.environment,
		msg.explain,
	)
	if msg.historyDone {
		m.syncRecordedHistory()
	} else {
		m.recordHTTPHistory(
			msg.response,
			msg.executed,
			msg.requestText,
			msg.environment,
			msg.runtimeSecrets...,
		)
	}
	return cmd
}

func (m *Model) recordResponseLatency(msg responseMsg) {
	if msg.response != nil {
		m.addLatency(msg.response.Duration)
		return
	}
	if msg.grpc != nil {
		m.addLatency(msg.grpc.Duration)
	}
}

func (m *Model) consumeRequestError(err error, rep *xplain.Report) tea.Cmd {
	if err == nil {
		return nil
	}

	canceled := errors.Is(err, context.Canceled)

	if m.responseLatest != nil && m.responseLatest.ready {
		m.responsePrevious = m.responseLatest
	}

	m.responseLoading = false
	m.responseLoadingFrame = 0
	m.responsePending = nil
	m.responseRenderToken = ""
	if m.responseTokens != nil {
		for key := range m.responseTokens {
			delete(m.responseTokens, key)
		}
	}

	code := errdef.CodeOf(err)
	title := requestErrorTitle(code)
	detail := err.Error()
	if canceled {
		title = "Request Canceled"
		detail = "Request was canceled by user."
	}
	if detail == "" {
		detail = "Request failed with no additional details."
	}
	note := requestErrorNote(code)
	pretty := joinSections(title, detail, note)
	raw := joinSections(title, detail)

	var meta []string
	if code != errdef.CodeUnknown && string(code) != "" && !canceled {
		meta = append(meta, fmt.Sprintf("Code: %s", strings.ToUpper(string(code))))
	}
	if strings.TrimSpace(note) != "" && !canceled {
		meta = append(meta, note)
	}
	metaText := strings.Join(meta, "\n")
	headers := joinSections(title, metaText, detail)

	snapshot := &responseSnapshot{
		id:      nextResponseRenderToken(),
		pretty:  pretty,
		raw:     raw,
		headers: headers,
		explain: explainState{
			report: rep,
		},
		ready: true,
	}
	m.responseLatest = snapshot
	m.responsePending = nil

	target := m.responseTargetPane()
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		pane.snapshot = snapshot
		pane.invalidateCaches()
		pane.viewport.SetContent(pretty)
		pane.viewport.GotoTop()
		pane.setCurrPosition()
	}
	m.setLivePane(target)

	return m.syncResponsePanes()
}

func (m *Model) consumeSkippedRequest(reason string, rep *xplain.Report) tea.Cmd {
	if m.responseLatest != nil && m.responseLatest.ready {
		m.responsePrevious = m.responseLatest
	}

	m.responseLoading = false
	m.responseLoadingFrame = 0
	m.responsePending = nil
	m.responseRenderToken = ""
	if m.responseTokens != nil {
		for key := range m.responseTokens {
			delete(m.responseTokens, key)
		}
	}

	title := "Request Skipped"
	detail := strings.TrimSpace(reason)
	if detail == "" {
		detail = "Condition evaluated to false."
	}
	pretty := joinSections(title, detail)
	raw := joinSections(title, detail)
	headers := joinSections(title, detail)

	snapshot := &responseSnapshot{
		id:      nextResponseRenderToken(),
		pretty:  pretty,
		raw:     raw,
		headers: headers,
		explain: explainState{
			report: rep,
		},
		ready: true,
	}
	m.responseLatest = snapshot
	m.responsePending = nil

	target := m.responseTargetPane()
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		pane.snapshot = snapshot
		pane.invalidateCaches()
		pane.viewport.SetContent(pretty)
		pane.viewport.GotoTop()
		pane.setCurrPosition()
	}
	m.setLivePane(target)

	m.setStatusMessage(statusMsg{text: detail, level: statusWarn})
	return m.syncResponsePanes()
}

func (m *Model) consumeExplainPreview(env string, rep *xplain.Report) tea.Cmd {
	if rep == nil {
		return nil
	}
	if m.responseLatest != nil && m.responseLatest.ready {
		m.responsePrevious = m.responseLatest
	}

	m.responseLoading = false
	m.responseLoadingFrame = 0
	m.responsePending = nil
	m.responseRenderToken = ""
	if m.responseTokens != nil {
		for key := range m.responseTokens {
			delete(m.responseTokens, key)
		}
	}

	detail := "Explain preview ready. No request was sent."
	if txt := strings.TrimSpace(rep.Decision); txt != "" {
		detail = txt
	}

	pretty := joinSections(
		"Explain Preview",
		detail,
		"Open the Explain tab to inspect the prepared request.",
	)
	raw := joinSections("Explain Preview", detail)
	headers := joinSections("Explain Preview", "No request was sent.")

	snapshot := &responseSnapshot{
		id:      nextResponseRenderToken(),
		pretty:  pretty,
		raw:     raw,
		headers: headers,
		explain: explainState{
			report: rep,
		},
		ready:       true,
		environment: env,
	}
	m.responseLatest = snapshot
	m.responsePending = nil

	target := m.responseTargetPane()
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		if id == target {
			pane.snapshot = snapshot
			pane.invalidateCaches()
			pane.setActiveTab(responseTabExplain)
			pane.viewport.GotoTop()
			pane.setCurrPosition()
		}
	}
	m.setLivePane(target)
	m.setStatusMessage(statusMsg{text: detail, level: statusInfo})
	return m.syncResponsePanes()
}

func requestErrorTitle(code errdef.Code) string {
	switch code {
	case errdef.CodeCanceled:
		return "Request Canceled"
	case errdef.CodeTimeout:
		return "Request Timeout"
	case errdef.CodeScript:
		return "Request Script Error"
	case errdef.CodeAuth:
		return "Request Auth Error"
	case errdef.CodeRoute:
		return "Request Route Error"
	case errdef.CodeProtocol:
		return "Request Protocol Error"
	case errdef.CodeNetwork:
		return "Request Network Error"
	case errdef.CodeTLS:
		return "Request TLS Error"
	case errdef.CodeHTTP:
		return "HTTP Request Error"
	case errdef.CodeParse:
		return "Request Parse Error"
	}
	if code != errdef.CodeUnknown && string(code) != "" {
		return fmt.Sprintf("Request Error (%s)", strings.ToUpper(string(code)))
	}
	return "Request Error"
}

func requestErrorNote(code errdef.Code) string {
	switch code {
	case errdef.CodeCanceled:
		return "Request was canceled before completion."
	case errdef.CodeTimeout:
		return "Request timed out before a response payload was available."
	case errdef.CodeScript:
		return "Request scripts failed before completion."
	case errdef.CodeAuth:
		return "Request authentication failed before a response payload was available."
	case errdef.CodeRoute:
		return "Request route setup failed before a response payload was available."
	case errdef.CodeProtocol, errdef.CodeNetwork, errdef.CodeTLS, errdef.CodeHTTP:
		return "No response payload received."
	default:
		return "Request did not produce a response payload."
	}
}

func (m *Model) consumeHTTPResponse(
	resp *httpclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
	environment string,
	rep *xplain.Report,
) tea.Cmd {
	m.lastGRPC = nil
	m.lastResponse = resp

	if m.responseLatest != nil && m.responseLatest.ready {
		m.responsePrevious = m.responseLatest
	}

	if resp == nil {
		m.abortResponseFormatting()
		m.responseLatest = nil
		m.responsePending = nil
		target := m.responseTargetPane()
		for _, id := range m.visiblePaneIDs() {
			pane := m.pane(id)
			if pane == nil {
				continue
			}
			if id == target {
				pane.snapshot = nil
				pane.invalidateCaches()
				width := pane.viewport.Width
				if width <= 0 {
					width = defaultResponseViewportWidth
				}
				pane.viewport.SetContent(logoPlaceholder(width, pane.viewport.Height))
				pane.viewport.GotoTop()
				pane.setCurrPosition()
			}
		}
		m.setLivePane(target)
		return nil
	}

	m.abortResponseFormatting()

	failureCount := 0
	for _, result := range tests {
		if !result.Passed {
			failureCount++
		}
	}

	var traceSpec *restfile.TraceSpec
	if resp != nil {
		if cloned := cloneTraceSpec(
			traceSpecFromRequest(resp.Request),
		); cloned != nil &&
			cloned.Enabled {
			traceSpec = cloned
		}
	}
	var timeline timelineReport
	if resp != nil && resp.Timeline != nil {
		timeline = buildTimelineReport(
			resp.Timeline,
			traceSpec,
			resp.TraceReport,
			newTimelineStyles(&m.theme, m.themeRuntime.appearance),
		)
	}

	statusLevel := statusSuccess
	statusText := ""
	if resp != nil {
		statusText = fmt.Sprintf("%s (%d)", resp.Status, resp.StatusCode)
	}

	switch {
	case scriptErr != nil:
		statusText = fmt.Sprintf("%s – tests error: %v", statusText, scriptErr)
		statusLevel = statusWarn
	case failureCount > 0:
		statusText = fmt.Sprintf("%s – %d test(s) failed", statusText, failureCount)
		statusLevel = statusWarn
	case len(tests) > 0:
		statusText = fmt.Sprintf("%s – all tests passed", statusText)
	default:
		if statusText == "" {
			statusText = "Request completed"
			statusLevel = statusSuccess
		}
	}

	if len(timeline.breaches) > 0 {
		primary := timeline.breaches[0]
		overrun := primary.Over.Round(time.Millisecond)
		statusText = fmt.Sprintf(
			"%s – trace budget breach %s (+%s)",
			statusText,
			humanPhaseName(primary.Kind),
			overrun,
		)
		if len(timeline.breaches) > 1 {
			statusText = fmt.Sprintf("%s (%d total)", statusText, len(timeline.breaches))
		}
		statusLevel = statusWarn
	}

	m.setStatusMessage(statusMsg{text: statusText, level: statusLevel})

	token := nextResponseRenderToken()
	snapshot := &responseSnapshot{
		id:          token,
		environment: environment,
		explain: explainState{
			report: rep,
		},
		source: newHTTPResponseRenderSource(resp, tests, scriptErr),
	}
	m.responseRenderToken = token
	m.responsePending = snapshot
	m.responseLatest = snapshot
	if traceSpec != nil {
		snapshot.traceSpec = traceSpec
	}
	if resp != nil && resp.Timeline != nil {
		snapshot.timeline = resp.Timeline.Clone()
		snapshot.traceReport = timeline
		snapshot.traceData = resp.TraceReport.Clone()
	}
	if m.responseTokens == nil {
		m.responseTokens = make(map[string]*responseSnapshot)
	}
	m.responseTokens[token] = snapshot
	m.responseLoading = true
	m.responseLoadingFrame = 0

	target := m.responseTargetPane()
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		if id == target {
			pane.snapshot = snapshot
			pane.invalidateCaches()
			pane.viewport.SetContent(m.responseLoadingMessage())
			pane.viewport.GotoTop()
			pane.setCurrPosition()
		}
	}
	m.setLivePane(target)

	primaryWidth := m.pane(responsePanePrimary).viewport.Width
	if primaryWidth <= 0 {
		primaryWidth = defaultResponseViewportWidth
	}

	formatCtx, cancel := context.WithCancel(context.Background())
	m.responseRenderCancel = cancel

	return m.respCmd(m.respFmtCmd(formatCtx, token, resp, tests, scriptErr, primaryWidth))
}

func (m *Model) responseLoadingMessage() string {
	spin := m.tabSpinner()
	if spin == "" {
		return responseFormattingBase
	}
	return responseFormattingBase + " " + spin
}

func (m *Model) responseReflowMessage() string {
	spin := m.tabSpinner()
	if spin == "" {
		return responseReflowingMessage
	}
	return responseReflowingMessage + " " + spin
}

func (m *Model) abortResponseFormatting() {
	if m.responseRenderCancel != nil {
		m.responseRenderCancel()
		m.responseRenderCancel = nil
	}
	if m.responseRenderToken != "" && m.responseTokens != nil {
		delete(m.responseTokens, m.responseRenderToken)
	}
	m.responseRenderToken = ""
	m.responseLoading = false
	m.responseLoadingFrame = 0
	m.respSpinStop()
}

func (m *Model) cancelResponseFormatting(reason string) tea.Cmd {
	pending := m.responsePending
	previous := m.responsePrevious
	m.abortResponseFormatting()
	m.responsePending = nil

	if pending != nil && !pending.ready {
		switch {
		case previous != nil && previous.ready:
			for _, id := range m.visiblePaneIDs() {
				pane := m.pane(id)
				if pane == nil || pane.snapshot != pending {
					continue
				}
				pane.snapshot = previous
				pane.invalidateCaches()
			}
			m.responseLatest = previous
		default:
			canceled := responseFormattingCanceledText
			pending.pretty = canceled
			pending.raw = canceled
			pending.rawSummary = canceled
			pending.headers = canceled
			pending.requestHeaders = canceled
			pending.ready = true
			m.responseLatest = pending
		}
	}

	if strings.TrimSpace(reason) != "" {
		m.setStatusMessage(statusMsg{text: reason, level: statusInfo})
	}
	return m.syncResponsePanes()
}

func (m *Model) scheduleResponseLoadingTick() tea.Cmd {
	if !m.responseLoading {
		return nil
	}
	return tea.Tick(responseLoadingTickInterval, func(time.Time) tea.Msg {
		return responseLoadingTickMsg{}
	})
}

func (m *Model) handleResponseRendered(msg responseRenderedMsg) tea.Cmd {
	if msg.token == "" || msg.token != m.responseRenderToken {
		return nil
	}

	snapshot, ok := m.responseTokens[msg.token]
	if !ok {
		snapshot = m.responseLatest
	}
	if snapshot == nil {
		return nil
	}

	snapshot.pretty = msg.pretty
	snapshot.raw = msg.raw
	snapshot.rawSummary = msg.rawSummary
	snapshot.headers = msg.headers
	snapshot.requestHeaders = msg.requestHeaders
	snapshot.body = append([]byte(nil), msg.body...)
	snapshot.bodyMeta = msg.meta
	snapshot.contentType = msg.contentType
	snapshot.rawText = msg.rawText
	snapshot.rawHex = msg.rawHex
	snapshot.rawBase64 = msg.rawBase64
	if msg.rawMode != 0 {
		snapshot.rawMode = msg.rawMode
	} else {
		snapshot.rawMode = rawViewText
	}
	snapshot.responseHeaders = cloneHeaders(msg.headersMap)
	snapshot.effectiveURL = msg.effectiveURL
	applyRawViewMode(snapshot, snapshot.rawMode)
	snapshot.ready = true

	delete(m.responseTokens, msg.token)
	if m.responsePending == snapshot {
		m.responsePending = nil
	}
	m.responseRenderToken = ""
	m.responseLoading = false
	m.responseLoadingFrame = 0
	m.responseRenderCancel = nil
	m.responseLatest = snapshot
	m.respSpinStop()

	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot != snapshot {
			continue
		}
		pane.invalidateCaches()
		if pane.sel.on {
			pane.sel.clear()
		}
		if pane.cursor.on {
			pane.cursor.clear()
		}
		if pane.cursorStore != nil {
			pane.cursorStore = make(map[respCursorKey]respCursor)
		}
		if msg.width > 0 && pane.viewport.Width == msg.width {
			prettyWidth := responseWrapWidth(responseTabPretty, msg.width)
			prettyBase := displayContent(msg.pretty)
			if shouldInlineWrap(responseTabPretty, prettyBase) {
				pane.setCacheForTab(
					responseTabPretty,
					rawViewText,
					pane.headersView,
					wrapCache(responseTabPretty, prettyBase, prettyWidth),
				)
			}
			rawWidth := responseWrapWidth(responseTabRaw, msg.width)
			rawBase := displayContent(snapshot.raw)
			if shouldInlineWrap(responseTabRaw, rawBase) {
				pane.setCacheForTab(
					responseTabRaw,
					snapshot.rawMode,
					pane.headersView,
					wrapCache(responseTabRaw, rawBase, rawWidth),
				)
			}

			headersWidth := responseWrapWidth(responseTabHeaders, msg.width)
			headersBase, _ := m.paneDisplayContent(
				id,
				responseTabHeaders,
				headersWidth,
			)
			if shouldInlineWrap(responseTabHeaders, headersBase) {
				pane.setCacheForTab(
					responseTabHeaders,
					rawViewText,
					pane.headersView,
					wrapCache(responseTabHeaders, headersBase, headersWidth),
				)
			}
		}
		if strings.TrimSpace(snapshot.stats) != "" {
			pane.wrapCache[responseTabStats] = cachedWrap{}
		}
		if snapshot.timeline != nil {
			pane.wrapCache[responseTabTimeline] = cachedWrap{}
		}
		pane.viewport.GotoTop()
		pane.setCurrPosition()
	}
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane != nil {
			pane.wrapCache[responseTabDiff] = cachedWrap{}
			pane.wrapCache[responseTabTimeline] = cachedWrap{}
		}
	}

	return m.syncResponsePanes()
}

func (m *Model) handleResponseLoadingTick() tea.Cmd {
	if !m.responseLoading {
		return nil
	}
	m.responseLoadingFrame = (m.responseLoadingFrame + 1) % 3
	message := m.responseLoadingMessage()
	updated := false
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot == nil || pane.snapshot.ready {
			continue
		}
		pane.viewport.SetContent(message)
		updated = true
	}
	if !updated {
		return nil
	}
	return m.scheduleResponseLoadingTick()
}

func (m *Model) consumeGRPCResponse(
	resp *grpcclient.Response,
	tests []scripts.TestResult,
	scriptErr error,
	req *restfile.Request,
	environment string,
	rep *xplain.Report,
) tea.Cmd {
	m.lastResponse = nil
	m.lastGRPC = resp
	m.responseLoading = false
	m.responseRenderToken = ""
	m.responsePending = nil
	if m.responseLatest != nil && m.responseLatest.ready {
		m.responsePrevious = m.responseLatest
	}

	if resp == nil {
		target := m.responseTargetPane()
		for _, id := range m.visiblePaneIDs() {
			pane := m.pane(id)
			if pane == nil {
				continue
			}
			if id == target {
				pane.snapshot = nil
				pane.invalidateCaches()
				pane.viewport.SetContent("No gRPC response")
				pane.viewport.GotoTop()
				pane.setCurrPosition()
			}
		}
		m.setLivePane(target)
		return nil
	}

	renderer := m.themeRuntime.responseRenderer(m.theme)
	fullMethod := ""
	if req != nil && req.GRPC != nil {
		fullMethod = req.GRPC.FullMethod
	}
	views := renderer.buildGRPCResponseViews(resp, fullMethod)
	statusLine := views.rawSummary

	snapshot := &responseSnapshot{
		pretty:     views.pretty,
		raw:        views.raw,
		rawSummary: views.rawSummary,
		headers:    views.headers,
		explain: explainState{
			report: rep,
		},
		ready:           true,
		environment:     environment,
		body:            append([]byte(nil), resp.Wire...),
		bodyMeta:        views.meta,
		contentType:     views.contentType,
		rawText:         views.rawText,
		rawHex:          views.rawHex,
		rawBase64:       views.rawBase64,
		rawMode:         views.rawMode,
		responseHeaders: grpcResponseHeaderMap(resp),
		requestHeaders:  renderer.renderGRPCReqHdrs(req, defaultResponseViewportWidth),
		source:          newGRPCResponseRenderSource(resp, fullMethod, req),
	}
	if len(snapshot.body) == 0 {
		snapshot.body = append([]byte(nil), resp.Body...)
	}
	applyRawViewMode(snapshot, snapshot.rawMode)
	m.responseLatest = snapshot
	m.responsePending = nil

	if m.responseTokens != nil {
		for key := range m.responseTokens {
			delete(m.responseTokens, key)
		}
	}

	switch {
	case resp.StatusCode != codes.OK:
		m.setStatusMessage(statusMsg{text: statusLine, level: statusWarn})
	default:
		m.setStatusMessage(statusMsg{text: statusLine, level: statusSuccess})
	}

	target := m.responseTargetPane()
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		if id == target {
			pane.snapshot = snapshot
		}
		pane.invalidateCaches()
		pane.viewport.GotoTop()
		pane.setCurrPosition()
	}
	m.setLivePane(target)

	return m.syncResponsePanes()
}

func (m *Model) recordHTTPHistory(
	resp *httpclient.Response,
	req *restfile.Request,
	requestText string,
	environment string,
	extraSecrets ...string,
) {
	hs := m.historyStore()
	if hs == nil || resp == nil || req == nil {
		return
	}

	secrets := m.secretValuesForRedaction(req, extraSecrets...)
	maskHeaders := !req.Metadata.AllowSensitiveHeaders

	snippet := "<body suppressed>"
	if !req.Metadata.NoLog {
		ct := ""
		if resp.Headers != nil {
			ct = resp.Headers.Get("Content-Type")
		}
		meta := binaryview.Analyze(resp.Body, ct)
		if meta.Kind == binaryview.KindBinary || !meta.Printable {
			snippet = formatBinaryHistorySnippet(meta, len(resp.Body))
		} else {
			snippet = redactHistoryText(string(resp.Body), secrets, false)
			if len(snippet) > 2000 {
				snippet = snippet[:2000]
			}
		}
	}
	desc := strings.TrimSpace(req.Metadata.Description)
	tags := normalizedTags(req.Metadata.Tags)

	redacted := redactHistoryText(requestText, secrets, maskHeaders)

	entry := history.Entry{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		ExecutedAt:  time.Now(),
		Environment: environment,
		RequestName: requestIdentifier(req),
		FilePath:    m.historyFilePath(),
		Method:      req.Method,
		URL:         req.URL,
		Status:      resp.Status,
		StatusCode:  resp.StatusCode,
		Duration:    resp.Duration,
		BodySnippet: snippet,
		RequestText: redacted,
		Description: desc,
		Tags:        tags,
	}
	entry.Trace = history.NewTraceSummary(resp.Timeline, resp.TraceReport)
	if err := hs.Append(entry); err != nil {
		m.setStatusMessage(
			statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn},
		)
	}
	m.historySelectedID = entry.ID
	m.syncHistory()
}

func (m *Model) recordSkippedHistory(
	req *restfile.Request,
	requestText, environment, reason string,
	extraSecrets ...string,
) {
	hs := m.historyStore()
	if hs == nil || req == nil {
		return
	}

	if strings.TrimSpace(requestText) == "" {
		requestText = renderRequestText(req)
	}

	secrets := m.secretValuesForRedaction(req, extraSecrets...)
	maskHeaders := !req.Metadata.AllowSensitiveHeaders
	redacted := redactHistoryText(requestText, secrets, maskHeaders)

	snippet := strings.TrimSpace(reason)
	if snippet == "" {
		snippet = "<skipped>"
	}
	if len(snippet) > 2000 {
		snippet = snippet[:2000]
	}

	desc := strings.TrimSpace(req.Metadata.Description)
	tags := normalizedTags(req.Metadata.Tags)

	entry := history.Entry{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		ExecutedAt:  time.Now(),
		Environment: environment,
		RequestName: requestIdentifier(req),
		FilePath:    m.historyFilePath(),
		Method:      req.Method,
		URL:         req.URL,
		Status:      "SKIPPED",
		StatusCode:  0,
		Duration:    0,
		BodySnippet: snippet,
		RequestText: redacted,
		Description: desc,
		Tags:        tags,
	}
	if err := hs.Append(entry); err != nil {
		m.setStatusMessage(
			statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn},
		)
	}
	m.historySelectedID = entry.ID
	m.syncHistory()
}

func formatBinaryHistorySnippet(meta binaryview.Meta, size int) string {
	sizeText := formatByteSize(int64(size))
	mime := strings.TrimSpace(meta.MIME)
	if mime != "" {
		return fmt.Sprintf("<binary body %s, %s>", sizeText, mime)
	}
	return fmt.Sprintf("<binary body %s>", sizeText)
}

func (m *Model) recordGRPCHistory(
	resp *grpcclient.Response,
	req *restfile.Request,
	requestText string,
	environment string,
	extraSecrets ...string,
) {
	hs := m.historyStore()
	if hs == nil || resp == nil || req == nil {
		return
	}

	secrets := m.secretValuesForRedaction(req, extraSecrets...)
	maskHeaders := !req.Metadata.AllowSensitiveHeaders

	snippet := resp.Message
	if req.Metadata.NoLog {
		snippet = "<body suppressed>"
	} else {
		snippet = redactHistoryText(snippet, secrets, false)
		if len(snippet) > 2000 {
			snippet = snippet[:2000]
		}
	}
	desc := strings.TrimSpace(req.Metadata.Description)
	tags := normalizedTags(req.Metadata.Tags)

	redacted := redactHistoryText(requestText, secrets, maskHeaders)

	entry := history.Entry{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		ExecutedAt:  time.Now(),
		Environment: environment,
		RequestName: requestIdentifier(req),
		FilePath:    m.historyFilePath(),
		Method:      req.Method,
		URL:         req.URL,
		Status:      resp.StatusCode.String(),
		StatusCode:  int(resp.StatusCode),
		Duration:    resp.Duration,
		BodySnippet: snippet,
		RequestText: redacted,
		Description: desc,
		Tags:        tags,
	}

	if err := hs.Append(entry); err != nil {
		m.setStatusMessage(
			statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn},
		)
	}
	m.historySelectedID = entry.ID
	m.syncHistory()
}

func (m *Model) secretValuesForRedaction(req *restfile.Request, extraSecrets ...string) []string {
	values := make(map[string]struct{})
	add := func(value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		values[value] = struct{}{}
	}

	if req != nil {
		for _, v := range req.Variables {
			if v.Secret {
				add(v.Value)
			}
		}
	}

	if doc := m.doc; doc != nil {
		for _, v := range doc.Variables {
			if v.Secret {
				add(v.Value)
			}
		}
		for _, v := range doc.Globals {
			if v.Secret {
				add(v.Value)
			}
		}
	}

	if fs := m.fileStore(); fs != nil {
		path := m.documentRuntimePath(m.doc)
		if snapshot := fs.Snapshot(m.cfg.EnvironmentName, path); len(snapshot) > 0 {
			for _, entry := range snapshot {
				if entry.Secret {
					add(entry.Value)
				}
			}
		}
	}

	if gs := m.globalsStore(); gs != nil {
		if snapshot := gs.Snapshot(m.cfg.EnvironmentName); len(snapshot) > 0 {
			for _, entry := range snapshot {
				if entry.Secret {
					add(entry.Value)
				}
			}
		}
	}
	for _, value := range extraSecrets {
		add(value)
	}

	if len(values) == 0 {
		return nil
	}

	secrets := make([]string, 0, len(values))
	for value := range values {
		secrets = append(secrets, value)
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets
}

func (m *Model) secretValuesForEnvironment(env string, req *restfile.Request) []string {
	if strings.TrimSpace(env) == "" {
		return m.secretValuesForRedaction(req)
	}

	prev := m.cfg.EnvironmentName
	m.cfg.EnvironmentName = env
	defer func() {
		m.cfg.EnvironmentName = prev
	}()
	return m.secretValuesForRedaction(req)
}

func redactHistoryText(text string, secrets []string, maskHeaders bool) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" && len(secrets) == 0 {
		return text
	}

	redacted := text
	if len(secrets) > 0 {
		mask := maskSecret("", true)
		for _, value := range secrets {
			if value == "" || !strings.Contains(redacted, value) {
				continue
			}
			redacted = strings.ReplaceAll(redacted, value, mask)
		}
	}

	if maskHeaders {
		redacted = redactSensitiveHeaders(redacted)
	}

	return redacted
}

func redactSensitiveHeaders(text string) string {
	lines := strings.Split(text, "\n")
	mask := maskSecret("", true)
	changed := false
	for idx, line := range lines {
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if name == "" {
			continue
		}
		if !shouldMaskHistoryHeader(name) {
			continue
		}
		rest := line[colon+1:]
		leadingSpaces := len(rest) - len(strings.TrimLeft(rest, " \t"))
		prefix := line[:colon+1]
		pad := ""
		if leadingSpaces > 0 {
			pad = rest[:leadingSpaces]
		}
		lines[idx] = prefix + pad + mask
		changed = true
	}
	if !changed {
		return text
	}
	return strings.Join(lines, "\n")
}

// Prefer failures and then the recorded baseline so reopening a history entry
// highlights the most useful diff without guesswork.
func selectCompareHistoryResult(entry history.Entry) *history.CompareResult {
	if entry.Compare == nil || len(entry.Compare.Results) == 0 {
		return nil
	}

	for idx := range entry.Compare.Results {
		res := &entry.Compare.Results[idx]
		if res == nil {
			continue
		}
		if res.Error != "" || res.StatusCode >= 400 {
			return res
		}
	}
	if baseline := strings.TrimSpace(entry.Compare.Baseline); baseline != "" {
		for idx := range entry.Compare.Results {
			res := &entry.Compare.Results[idx]
			if res == nil {
				continue
			}
			if strings.EqualFold(res.Environment, baseline) {
				return res
			}
		}
	}
	return &entry.Compare.Results[0]
}

func bundleFromHistory(entry history.Entry) *compareBundle {
	if entry.Compare == nil || len(entry.Compare.Results) == 0 {
		return nil
	}

	bundle := &compareBundle{Baseline: entry.Compare.Baseline}
	rows := make([]compareRow, 0, len(entry.Compare.Results))
	for idx := range entry.Compare.Results {
		res := entry.Compare.Results[idx]
		code := "-"
		if res.StatusCode > 0 {
			code = fmt.Sprintf("%d", res.StatusCode)
		}
		summary := strings.TrimSpace(res.Error)
		if summary == "" {
			summary = strings.TrimSpace(res.BodySnippet)
		}
		if summary == "" {
			summary = strings.TrimSpace(res.Status)
		}
		if summary == "" {
			summary = "n/a"
		}
		row := compareRow{
			Result:   &compareResult{Environment: res.Environment},
			Status:   res.Status,
			Code:     code,
			Duration: res.Duration,
			Summary:  condense(summary, 80),
		}
		rows = append(rows, row)
	}
	bundle.Rows = rows
	return bundle
}

// Hydrate compare snapshots straight from history so the compare tab can render
// immediately even when no live response is available.
func (m *Model) populateCompareSnapshotsFromHistory(
	entry history.Entry,
	bundle *compareBundle,
	preferredEnv string,
) string {
	if entry.Compare == nil || len(entry.Compare.Results) == 0 {
		return strings.TrimSpace(preferredEnv)
	}

	selected := strings.TrimSpace(preferredEnv)
	for idx := range entry.Compare.Results {
		res := entry.Compare.Results[idx]
		snap := buildHistoryCompareSnapshot(res, bundle)
		if snap == nil {
			continue
		}
		env := strings.TrimSpace(res.Environment)
		m.setCompareSnapshot(env, snap)
		if selected == "" {
			selected = env
		}
	}
	return selected
}

func buildHistoryCompareSnapshot(
	res history.CompareResult,
	bundle *compareBundle,
) *responseSnapshot {
	env := strings.TrimSpace(res.Environment)
	if env == "" {
		return nil
	}
	summary := formatHistoryCompareSummary(env, res)
	headers := formatHistoryCompareHeaders(env, res)
	return &responseSnapshot{
		id:            nextResponseRenderToken(),
		pretty:        summary,
		raw:           summary,
		headers:       headers,
		ready:         true,
		environment:   env,
		compareBundle: bundle,
	}
}

func formatHistoryCompareSummary(env string, res history.CompareResult) string {
	lines := []string{fmt.Sprintf("Environment: %s", env)}
	if status := historyResultStatus(res); status != "" {
		lines = append(lines, fmt.Sprintf("Status: %s", status))
	}
	if res.Duration > 0 {
		lines = append(lines, fmt.Sprintf("Duration: %s", formatDurationShort(res.Duration)))
	}
	if errText := strings.TrimSpace(res.Error); errText != "" {
		lines = append(lines, "", "Error:", errText)
	}
	if body := strings.TrimSpace(res.BodySnippet); body != "" {
		lines = append(lines, "", body)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func formatHistoryCompareHeaders(env string, res history.CompareResult) string {
	lines := []string{fmt.Sprintf("Environment: %s", env)}
	if status := historyResultStatus(res); status != "" {
		lines = append(lines, fmt.Sprintf("Status: %s", status))
	}
	if res.Duration > 0 {
		lines = append(lines, fmt.Sprintf("Duration: %s", formatDurationShort(res.Duration)))
	}
	return strings.Join(lines, "\n")
}

func historyResultStatus(res history.CompareResult) string {
	status := strings.TrimSpace(res.Status)
	code := ""
	if res.StatusCode > 0 {
		code = fmt.Sprintf("%d", res.StatusCode)
	}
	switch {
	case status != "" && code != "":
		if strings.Contains(status, code) {
			return status
		}
		return fmt.Sprintf("%s (%s)", status, code)
	case status != "":
		return status
	case code != "":
		return fmt.Sprintf("Code %s", code)
	case strings.TrimSpace(res.Error) != "":
		return "Error"
	default:
		return ""
	}
}

func (m *Model) syncHistory() {
	hs := m.historyStore()
	if hs == nil {
		m.historyEntries = nil
		m.historyScopeCount = 0
		m.historyList.SetItems(nil)
		m.historySelectedID = ""
		m.historyList.Select(-1)
		return
	}

	entries, err := m.historyEntriesForScope()
	if err != nil {
		m.historyEntries = nil
		m.historyScopeCount = 0
		m.historyList.SetItems(nil)
		m.historySelectedID = ""
		m.historyList.Select(-1)
		m.setStatusMessage(
			statusMsg{
				text:  fmt.Sprintf("History query failed: %v", err),
				level: statusWarn,
			},
		)
		return
	}
	m.historyScopeCount = len(entries)
	filter := strings.TrimSpace(m.historyFilterInput.Value())
	if filter != "" {
		entries = filterHistoryEntries(entries, filter)
	}
	entries = sortHistoryEntries(entries, m.historySort)
	m.historyEntries = entries
	m.pruneHistorySelections()
	m.historyList.SetItems(makeHistoryItems(m.historyEntries, m.historyScope))
	m.restoreHistorySelection()
}

func (m *Model) historyEntriesForScope() ([]history.Entry, error) {
	hs := m.historyStore()
	if hs == nil {
		return nil, nil
	}
	switch m.historyScope {
	case historyScopeWorkflow:
		name := history.NormalizeWorkflowName(m.historyWorkflowName)
		if name == "" {
			return nil, nil
		}
		return hs.ByWorkflow(name)
	case historyScopeRequest:
		if m.currentRequest == nil {
			return nil, nil
		}
		identifier := requestIdentifier(m.currentRequest)
		if identifier == "" {
			return nil, nil
		}
		return hs.ByRequest(identifier)
	case historyScopeFile:
		return m.historyEntriesForFileScope()
	default:
		return hs.Entries()
	}
}

func (m Model) historyHeaderHeight() int {
	return 3
}

func (m Model) historyEmptyMessage() string {
	filter := strings.TrimSpace(m.historyFilterInput.Value())
	if filter != "" && m.historyScopeCount > 0 {
		return "No history entries match this filter."
	}
	switch m.historyScope {
	case historyScopeWorkflow:
		if strings.TrimSpace(m.historyWorkflowName) == "" {
			return "No workflow selected."
		}
	case historyScopeRequest:
		if m.currentRequest == nil {
			return "No request selected."
		}
	case historyScopeFile:
		if strings.TrimSpace(m.historyFilePath()) == "" {
			return "No file selected."
		}
	}
	return "No history yet. Execute a request to populate this view."
}

func (m *Model) blockHistoryKey() {
	if m.focus != focusResponse {
		return
	}
	pane := m.focusedPane()
	if pane == nil || pane.activeTab != responseTabHistory {
		return
	}
	m.historyBlockKey = true
}

func (m *Model) cycleHistoryScope() {
	m.historyScope = m.historyScope.next()
	if m.ready {
		m.syncHistory()
	}
	m.setStatusMessage(
		statusMsg{
			text:  fmt.Sprintf("History scope: %s", historyScopeLabel(m.historyScope)),
			level: statusInfo,
		},
	)
}

func (m *Model) toggleHistorySort() {
	m.historySort = m.historySort.toggle()
	if m.ready {
		m.syncHistory()
	}
	m.setStatusMessage(
		statusMsg{
			text:  fmt.Sprintf("History sort: %s", historySortLabel(m.historySort)),
			level: statusInfo,
		},
	)
}

func (m *Model) openHistoryFilter() {
	m.historyFilterActive = true
	m.historyFilterInput.CursorEnd()
	m.historyFilterInput.Focus()
	m.blockHistoryKey()
	m.setStatusMessage(
		statusMsg{text: "Filter history (Enter to apply, Esc to clear)", level: statusInfo},
	)
}

func (m *Model) clearHistoryFilter(force bool) bool {
	hasFilter := strings.TrimSpace(m.historyFilterInput.Value()) != ""
	if !force && !hasFilter {
		return false
	}
	m.historyFilterActive = false
	m.historyFilterInput.SetValue("")
	m.historyFilterInput.Blur()
	m.syncHistory()
	m.setStatusMessage(statusMsg{text: "History filter cleared", level: statusInfo})
	return true
}

func (m *Model) toggleHistorySelection() {
	entry, ok := m.selectedHistoryEntry()
	if !ok || entry.ID == "" {
		m.setStatusMessage(statusMsg{text: "No history entry selected", level: statusWarn})
		return
	}
	if _, ok := m.historySelected[entry.ID]; ok {
		delete(m.historySelected, entry.ID)
	} else {
		m.historySelected[entry.ID] = struct{}{}
	}
	if len(m.historySelected) == 0 {
		m.setStatusMessage(statusMsg{text: "History selection cleared", level: statusInfo})
		return
	}
	label := "entries"
	if len(m.historySelected) == 1 {
		label = "entry"
	}
	m.setStatusMessage(
		statusMsg{
			text:  fmt.Sprintf("Selected %d history %s", len(m.historySelected), label),
			level: statusInfo,
		},
	)
}

func (m *Model) pruneHistorySelections() {
	if len(m.historySelected) == 0 {
		return
	}
	keep := make(map[string]struct{}, len(m.historyEntries))
	for _, entry := range m.historyEntries {
		if entry.ID != "" {
			keep[entry.ID] = struct{}{}
		}
	}
	for id := range m.historySelected {
		if _, ok := keep[id]; !ok {
			delete(m.historySelected, id)
		}
	}
}

func (m *Model) clearHistorySelections() {
	for id := range m.historySelected {
		delete(m.historySelected, id)
	}
}

func (m *Model) deleteSelectedHistoryEntries() (int, int, error) {
	hs := m.historyStore()
	if hs == nil || len(m.historySelected) == 0 {
		return 0, 0, nil
	}
	ids := make([]string, 0, len(m.historySelected))
	for id := range m.historySelected {
		ids = append(ids, id)
	}
	deleted := 0
	failed := 0
	var firstErr error
	for _, id := range ids {
		ok, err := hs.Delete(id)
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if ok {
			deleted++
		}
	}
	return deleted, failed, firstErr
}

func (m *Model) handleHistoryFilterKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !m.historyFilterActive {
		return nil, false
	}
	if m.focus != focusResponse {
		return nil, false
	}
	pane := m.focusedPane()
	if pane == nil || pane.activeTab != responseTabHistory {
		return nil, false
	}

	switch msg.String() {
	case "enter":
		m.historyFilterActive = false
		m.historyFilterInput.Blur()
		m.syncHistory()
		val := strings.TrimSpace(m.historyFilterInput.Value())
		if val == "" {
			m.setStatusMessage(statusMsg{text: "History filter cleared", level: statusInfo})
		} else {
			_, invalid := parseHistoryFilterAt(val, time.Now())
			if len(invalid) > 0 {
				m.setStatusMessage(
					statusMsg{
						text: fmt.Sprintf(
							"Invalid date filter: %s (use DD-MM-YYYY, MM-DD-YYYY, or DD-MMM-YYYY)",
							strings.Join(invalid, ", "),
						),
						level: statusWarn,
					},
				)
			} else {
				m.setStatusMessage(
					statusMsg{text: fmt.Sprintf("History filter: %s", val), level: statusInfo},
				)
			}
		}
		return nil, true
	case "esc":
		m.clearHistoryFilter(true)
		return nil, true
	default:
		updated := m.historyFilterInput
		updated, cmd := updated.Update(msg)
		m.historyFilterInput = updated
		m.syncHistory()
		return cmd, true
	}
}

func (m *Model) historyFilePath() string {
	if path := strings.TrimSpace(m.currentFile); path != "" {
		return path
	}
	if m.doc != nil {
		return strings.TrimSpace(m.doc.Path)
	}
	return ""
}

func (m *Model) syncRequestList(doc *restfile.Document) {
	_ = m.syncWorkflowList(doc)
	items, listItems := m.buildRequestItems(doc)
	m.requestItems = items
	if len(listItems) == 0 {
		m.requestList.SetItems(nil)
		m.requestList.Select(-1)
		if m.ready {
			m.applyLayout()
		}
		return
	}
	m.requestList.SetItems(listItems)
	if m.selectRequestItemByKey(m.activeRequestKey) {
		if idx := m.requestList.Index(); idx >= 0 && idx < len(m.requestItems) {
			m.currentRequest = m.requestItems[idx].request
		}
		if m.ready {
			m.applyLayout()
		}
		return
	}
	if len(m.requestItems) > 0 {
		m.requestList.Select(0)
		m.currentRequest = m.requestItems[0].request
		m.activeRequestTitle = requestDisplayName(m.requestItems[0].request)
		m.activeRequestKey = requestKey(m.requestItems[0].request)
	}
	if m.ready {
		m.applyLayout()
	}
}

func (m *Model) setActiveRequest(req *restfile.Request) {
	if req == nil {
		m.activeRequestTitle = ""
		m.activeRequestKey = ""
		m.currentRequest = nil
		m.streamFilterActive = false
		m.streamFilterInput.SetValue("")
		m.streamFilterInput.Blur()
		if m.historyScope == historyScopeRequest && m.ready {
			m.syncHistory()
		}
		return
	}
	prev := m.activeRequestKey
	m.currentRequest = req
	if m.wsConsole != nil {
		sessionID := m.sessionIDForRequest(req)
		if sessionID == "" || m.wsConsole.sessionID != sessionID {
			m.wsConsole = nil
		}
	}
	if m.requestSessions != nil && m.requestKeySessions != nil {
		if key := requestKey(req); key != "" {
			if id, ok := m.requestKeySessions[key]; ok {
				if existing := m.requestSessions[req]; existing == "" {
					m.requestSessions[req] = id
				}
			}
		}
	}
	m.streamFilterActive = false
	m.streamFilterInput.SetValue("")
	m.streamFilterInput.Blur()
	m.activeRequestTitle = requestDisplayName(req)
	m.activeRequestKey = requestKey(req)
	_ = m.selectRequestItemByKey(m.activeRequestKey)
	if prev != m.activeRequestKey {
		summary := requestMetaSummary(req)
		if summary == "" {
			summary = requestBaseTitle(req)
		}
		if summary != "" {
			m.setStatusMessage(statusMsg{text: summary, level: statusInfo})
		}
		if m.historyScope == historyScopeRequest && m.ready {
			m.syncHistory()
		}
	}
}

func (m *Model) selectRequestItemByKey(key string) bool {
	if key == "" {
		return false
	}
	for idx, item := range m.requestItems {
		if requestKey(item.request) == key {
			m.requestList.Select(idx)
			return true
		}
	}
	return false
}

func (m *Model) sendRequestFromList(execute bool) tea.Cmd {
	item, ok := m.requestList.SelectedItem().(requestListItem)
	if !ok {
		return nil
	}

	m.moveCursorToLine(item.line)
	m.setActiveRequest(item.request)

	if !execute {
		return m.previewRequest(item.request)
	}
	return m.sendActiveRequest()
}

func (m *Model) syncEditorWithRequestSelection(previousIndex int) {
	idx := m.requestList.Index()
	if idx == previousIndex {
		return
	}
	if idx < 0 || idx >= len(m.requestItems) {
		m.currentRequest = nil
		return
	}
	item := m.requestItems[idx]
	m.moveCursorToLine(item.line)
	m.setActiveRequest(item.request)
}

func (m *Model) previewRequest(req *restfile.Request) tea.Cmd {
	if req == nil {
		return nil
	}
	preview := renderRequestText(req)
	title := strings.TrimSpace(m.statusRequestTitle(m.doc, req, ""))
	if title == "" {
		title = requestDisplayName(req)
	}
	statusText := fmt.Sprintf("Previewing %s", title)
	return m.applyPreview(preview, statusText)
}

func (m *Model) applyPreview(preview string, statusText string) tea.Cmd {
	snapshot := &responseSnapshot{
		pretty:         preview,
		raw:            preview,
		headers:        preview,
		requestHeaders: preview,
		ready:          true,
	}
	m.responseRenderToken = ""
	m.responsePending = nil
	m.responseLoading = false
	m.responseLatest = snapshot

	targetPaneID := m.responseTargetPane()

	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		if id == targetPaneID {
			pane.snapshot = snapshot
		}
		pane.invalidateCaches()
		if id == targetPaneID {
			pane.setActiveTab(responseTabPretty)
		}
		pane.viewport.GotoTop()
		pane.setCurrPosition()
	}
	m.setLivePane(targetPaneID)

	if pane := m.pane(targetPaneID); pane != nil {
		displayWidth := pane.viewport.Width
		if displayWidth <= 0 {
			displayWidth = defaultResponseViewportWidth
		}
		content := displayContent(preview)
		prettyCache := wrapCache(
			responseTabPretty,
			content,
			responseWrapWidth(responseTabPretty, displayWidth),
		)
		pane.setCacheForTab(responseTabPretty, rawViewText, pane.headersView, prettyCache)
		pane.setCacheForTab(
			responseTabRaw,
			snapshot.rawMode,
			pane.headersView,
			wrapCache(responseTabRaw, content, responseWrapWidth(responseTabRaw, displayWidth)),
		)
		pane.setCacheForTab(
			responseTabHeaders,
			rawViewText,
			pane.headersView,
			wrapCache(
				responseTabHeaders,
				content,
				responseWrapWidth(responseTabHeaders, displayWidth),
			),
		)
		pane.wrapCache[responseTabDiff] = cachedWrap{}
		pane.wrapCache[responseTabStats] = cachedWrap{}
		pane.viewport.SetContent(prettyCache.content)
	}

	m.testResults = nil
	m.scriptError = nil

	var status tea.Cmd
	if strings.TrimSpace(statusText) != "" {
		status = func() tea.Msg {
			return statusMsg{text: statusText, level: statusInfo}
		}
	}
	if cmd := m.syncResponsePanes(); cmd != nil {
		if status != nil {
			return tea.Batch(cmd, status)
		}
		return cmd
	}
	return status
}

func (m *Model) moveCursorToLine(target int) {
	if target < 1 {
		target = 1
	}
	total := m.editor.LineCount()
	if total == 0 {
		return
	}
	if target > total {
		target = total
	}
	current := currentCursorLine(m.editor)
	if current == target {
		return
	}
	wasFocused := m.editor.Focused()
	if !wasFocused {
		_ = m.editor.Focus()
	}
	defer func() {
		if !wasFocused {
			m.editor.Blur()
		}
	}()
	for current < target {
		m.editor, _ = m.editor.Update(tea.KeyMsg{Type: tea.KeyDown})
		current++
	}
	for current > target {
		m.editor, _ = m.editor.Update(tea.KeyMsg{Type: tea.KeyUp})
		current--
	}
	m.editor, _ = m.editor.Update(tea.KeyMsg{Type: tea.KeyHome})
	m.syncNavigatorWithEditorCursor()
}

func requestBaseTitle(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = "REQ"
	}
	name := strings.TrimSpace(req.Metadata.Name)
	if name == "" {
		url := strings.TrimSpace(req.URL)
		if len(url) > 60 {
			url = url[:57] + "..."
		}
		name = url
	}
	return fmt.Sprintf("%s %s", method, name)
}

func requestDisplayName(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	base := requestBaseTitle(req)
	desc := strings.TrimSpace(req.Metadata.Description)
	tags := joinTags(req.Metadata.Tags, 3)
	var extra []string
	if desc != "" {
		extra = append(extra, condense(desc, 60))
	}
	if tags != "" {
		extra = append(extra, tags)
	}
	if len(extra) == 0 {
		return base
	}
	return fmt.Sprintf("%s - %s", base, strings.Join(extra, " | "))
}

func requestKey(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if name := strings.TrimSpace(req.Metadata.Name); name != "" {
		return "name:" + name
	}
	return fmt.Sprintf("line:%d:%s", req.LineRange.Start, req.Method)
}

func requestMetaSummary(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	return joinTags(req.Metadata.Tags, 5)
}

func normalizedTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (m *Model) findRequestByKey(key string) *restfile.Request {
	if key == "" || m.doc == nil {
		return nil
	}
	for _, req := range m.doc.Requests {
		if requestKey(req) == key {
			return req
		}
	}
	return nil
}

func (m *Model) captureHistorySelection() {
	idx := m.historyList.Index()
	if idx >= 0 && idx < len(m.historyEntries) {
		m.historySelectedID = m.historyEntries[idx].ID
	}
}

func (m *Model) restoreHistorySelection() {
	if len(m.historyEntries) == 0 {
		m.historySelectedID = ""
		m.historyList.Select(-1)
		return
	}
	if m.historySelectedID == "" {
		m.historyList.Select(0)
		m.historySelectedID = m.historyEntries[0].ID
		return
	}
	for idx, entry := range m.historyEntries {
		if entry.ID == m.historySelectedID {
			m.historyList.Select(idx)
			return
		}
	}
	m.historyList.Select(0)
	m.historySelectedID = m.historyEntries[0].ID
}

func (m *Model) selectNewestHistoryEntry() {
	if len(m.historyEntries) == 0 {
		m.historySelectedID = ""
		m.historyList.Select(-1)
		return
	}
	idx := 0
	if m.historySort == historySortOldest {
		idx = len(m.historyEntries) - 1
	}
	m.historyList.Select(idx)
	m.historySelectedID = m.historyEntries[idx].ID
}

func (m *Model) replayHistorySelection() tea.Cmd {
	return m.loadHistorySelection(true)
}

func (m *Model) deleteHistoryEntry(id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	hs := m.historyStore()
	if hs == nil {
		return false, nil
	}
	deleted, err := hs.Delete(id)
	if err != nil || !deleted {
		return deleted, err
	}
	if m.historySelectedID == id {
		m.historySelectedID = ""
	}
	if m.showHistoryPreview {
		m.closeHistoryPreview()
	}
	return true, nil
}

func traceSpecFromRequest(req *restfile.Request) *restfile.TraceSpec {
	if req == nil {
		return nil
	}
	return req.Metadata.Trace
}

func (m *Model) loadHistorySelection(send bool) tea.Cmd {
	item, ok := m.historyList.SelectedItem().(historyItem)
	if !ok {
		return nil
	}
	entry := item.entry
	requestText := entry.RequestText
	targetEnv := entry.Environment
	var compareBundle *compareBundle
	if entry.Compare != nil {
		if selected := selectCompareHistoryResult(entry); selected != nil {
			if strings.TrimSpace(selected.RequestText) != "" {
				requestText = selected.RequestText
			}
			if strings.TrimSpace(selected.Environment) != "" {
				targetEnv = selected.Environment
			}
		}
		if strings.TrimSpace(requestText) == "" {
			requestText = entry.RequestText
		}
		compareBundle = bundleFromHistory(entry)
	}
	if strings.TrimSpace(requestText) == "" {
		m.setStatusMessage(
			statusMsg{text: "History entry missing request payload", level: statusWarn},
		)
		return nil
	}

	doc := parser.Parse(m.currentFile, []byte(requestText))
	if len(doc.Requests) == 0 {
		m.setStatusMessage(statusMsg{text: "Unable to parse stored request", level: statusError})
		return nil
	}

	docReq := doc.Requests[0]
	if targetEnv != "" {
		m.cfg.EnvironmentName = targetEnv
	}

	options := m.cfg.HTTPOptions
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}

	m.doc = doc
	m.syncRequestList(doc)
	m.setActiveRequest(docReq)

	req := cloneRequest(docReq)
	m.currentRequest = req
	m.editor.SetValue(requestText)
	m.editor.SetCursor(0)
	m.testResults = nil
	m.scriptError = nil

	if !send {
		m.stopSending()
		label := strings.TrimSpace(m.statusRequestTitle(doc, req, targetEnv))
		if label == "" {
			label = "history request"
		}
		m.setStatusMessage(
			statusMsg{text: fmt.Sprintf("Loaded %s from history", label), level: statusInfo},
		)
		if compareBundle != nil {
			focusEnv := strings.TrimSpace(targetEnv)
			if focusEnv == "" && len(compareBundle.Rows) > 0 {
				focusEnv = strings.TrimSpace(compareBundle.Rows[0].Result.Environment)
			}
			m.resetCompareState()
			hydrated := m.populateCompareSnapshotsFromHistory(entry, compareBundle, focusEnv)
			if hydrated != "" {
				focusEnv = hydrated
			}
			m.compareBundle = compareBundle
			if focusEnv != "" {
				m.compareSelectedEnv = focusEnv
				m.compareFocusedEnv = focusEnv
				m.compareRowIndex = compareRowIndexForEnv(compareBundle, focusEnv)
			} else {
				m.compareRowIndex = 0
			}
			m.invalidateCompareTabCaches()
			if focusEnv == "" {
				focusEnv = targetEnv
			}
			content := renderCompareBundle(compareBundle, focusEnv)
			snap := &responseSnapshot{
				id:            nextResponseRenderToken(),
				pretty:        content,
				raw:           content,
				headers:       "",
				ready:         true,
				compareBundle: compareBundle,
				environment:   focusEnv,
			}
			m.applyHistorySnapshot(snap)
			return m.syncResponsePanes()
		}
		return m.presentHistoryEntry(entry, req)
	}

	spin := m.startSending()
	replayTarget := m.statusRequestTarget(doc, req, targetEnv)
	replayText := "Replaying"
	if trimmed := strings.TrimSpace(replayTarget); trimmed != "" {
		replayText = fmt.Sprintf("Replaying %s", trimmed)
	}
	m.statusPulseBase = replayText
	m.statusPulseFrame = -1
	m.setStatusMessage(statusMsg{text: replayText, level: statusInfo})
	cmd := m.execRunReq(doc, req, options, "", nil)
	return batchCmds([]tea.Cmd{cmd, m.startStatusPulse(), spin})
}

func (m *Model) presentHistoryEntry(entry history.Entry, req *restfile.Request) tea.Cmd {
	if entry.Trace == nil {
		return nil
	}

	tl := entry.Trace.Timeline()
	if tl == nil {
		return nil
	}
	rep := entry.Trace.Report()
	traceSpec := traceSpecFromSummary(entry.Trace)
	if traceSpec == nil {
		if clone := cloneTraceSpec(traceSpecFromRequest(req)); clone != nil && clone.Enabled {
			traceSpec = clone
		}
	}
	report := buildTimelineReport(
		tl,
		traceSpec,
		rep,
		newTimelineStyles(&m.theme, m.themeRuntime.appearance),
	)
	summary := historyEntrySummary(entry)
	snap := &responseSnapshot{
		id:          nextResponseRenderToken(),
		pretty:      summary,
		raw:         summary,
		headers:     "",
		ready:       true,
		timeline:    tl,
		traceData:   rep,
		traceReport: report,
		traceSpec:   traceSpec,
	}

	m.applyHistorySnapshot(snap)
	return m.syncResponsePanes()
}

func (m *Model) applyHistorySnapshot(snap *responseSnapshot) {
	if snap == nil {
		return
	}

	m.responsePending = nil
	m.responseLatest = snap
	if m.responseTokens != nil {
		for key := range m.responseTokens {
			delete(m.responseTokens, key)
		}
	}

	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		pane.snapshot = snap
		pane.invalidateCaches()
		if pane.activeTab == responseTabHistory {
			continue
		}
		pane.viewport.SetContent(displayContent(snap.pretty))
		pane.viewport.GotoTop()
		pane.setCurrPosition()
	}
}

func traceSpecFromSummary(summary *history.TraceSummary) *restfile.TraceSpec {
	if summary == nil || summary.Budgets == nil {
		return nil
	}
	spec := &restfile.TraceSpec{Enabled: true}
	spec.Budgets.Total = summary.Budgets.Total
	spec.Budgets.Tolerance = summary.Budgets.Tolerance
	if len(summary.Budgets.Phases) > 0 {
		phases := make(map[string]time.Duration, len(summary.Budgets.Phases))
		for name, limit := range summary.Budgets.Phases {
			phases[name] = limit
		}
		spec.Budgets.Phases = phases
	}
	return spec
}

func historyEntrySummary(entry history.Entry) string {
	var lines []string
	label := strings.TrimSpace(entry.RequestName)
	if label == "" {
		parts := strings.TrimSpace(strings.Join([]string{entry.Method, entry.URL}, " "))
		if parts != "" {
			label = parts
		}
	}
	if label != "" {
		lines = append(lines, label)
	}
	if entry.Status != "" && entry.StatusCode > 0 {
		lines = append(lines, fmt.Sprintf("Status: %s (%d)", entry.Status, entry.StatusCode))
	} else if entry.Status != "" {
		lines = append(lines, fmt.Sprintf("Status: %s", entry.Status))
	} else if entry.StatusCode > 0 {
		lines = append(lines, fmt.Sprintf("Status code: %d", entry.StatusCode))
	}
	if entry.Duration > 0 {
		lines = append(lines, fmt.Sprintf("Duration: %s", entry.Duration))
	}
	if !entry.ExecutedAt.IsZero() {
		lines = append(lines, "Recorded: "+entry.ExecutedAt.Format(time.RFC3339))
	}
	lines = append(lines, "Timeline: open the Timeline tab for phase details.")
	return strings.Join(lines, "\n")
}
