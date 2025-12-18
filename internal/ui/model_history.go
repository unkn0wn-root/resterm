package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/errdef"
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
	if state := m.compareRun; state != nil {
		if state.matches(msg.executed) || (msg.executed == nil && state.current != nil) {
			return m.handleCompareResponse(msg)
		}
	}
	if state := m.workflowRun; state != nil {
		if state.matches(msg.executed) || (msg.executed == nil && state.current != nil) {
			return m.handleWorkflowResponse(msg)
		}
	}
	if state := m.profileRun; state != nil {
		if state.matches(msg.executed) || (msg.executed == nil && state.current != nil) {
			return m.handleProfileResponse(msg)
		}
	}

	m.lastError = nil
	m.testResults = msg.tests
	m.scriptError = msg.scriptErr

	if msg.grpc != nil {
		if msg.err != nil {
			m.lastError = msg.err
		} else {
			m.lastError = nil
		}
		cmd := m.consumeGRPCResponse(msg.grpc, msg.tests, msg.scriptErr, msg.executed, msg.environment)
		m.recordGRPCHistory(msg.grpc, msg.executed, msg.requestText, msg.environment)
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
		if code == errdef.CodeScript || canceled {
			level = statusWarn
		}

		text := errdef.Message(msg.err)
		if canceled {
			text = "Request canceled"
		}

		cmd := m.consumeRequestError(msg.err)
		m.suppressNextErrorModal = true
		m.setStatusMessage(statusMsg{text: text, level: level})
		return cmd
	}

	cmd := m.consumeHTTPResponse(msg.response, msg.tests, msg.scriptErr, msg.environment)
	m.recordHTTPHistory(msg.response, msg.executed, msg.requestText, msg.environment)
	return cmd
}

func (m *Model) consumeRequestError(err error) tea.Cmd {
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
	detail := strings.TrimSpace(errdef.Message(err))
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
		ready:   true,
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

func requestErrorTitle(code errdef.Code) string {
	switch code {
	case errdef.CodeScript:
		return "Request Script Error"
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
	case errdef.CodeScript:
		return "Request scripts failed before completion."
	case errdef.CodeHTTP:
		return "No response payload received."
	default:
		return "Request did not produce a response payload."
	}
}

func (m *Model) consumeHTTPResponse(resp *httpclient.Response, tests []scripts.TestResult, scriptErr error, environment string) tea.Cmd {
	m.lastGRPC = nil
	m.lastResponse = resp

	if m.responseLatest != nil && m.responseLatest.ready {
		m.responsePrevious = m.responseLatest
	}

	if resp == nil {
		m.responseLoading = false
		m.responseLoadingFrame = 0
		m.responseRenderToken = ""
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
				centered := centerContent(noResponseMessage, width, pane.viewport.Height)
				pane.viewport.SetContent(wrapToWidth(centered, width))
				pane.viewport.GotoTop()
				pane.setCurrPosition()
			}
		}
		m.setLivePane(target)
		return nil
	}

	failureCount := 0
	for _, result := range tests {
		if !result.Passed {
			failureCount++
		}
	}

	var traceSpec *restfile.TraceSpec
	if resp != nil {
		if cloned := cloneTraceSpec(traceSpecFromRequest(resp.Request)); cloned != nil && cloned.Enabled {
			traceSpec = cloned
		}
	}
	var timeline timelineReport
	if resp != nil && resp.Timeline != nil {
		timeline = buildTimelineReport(resp.Timeline, traceSpec, resp.TraceReport, newTimelineStyles(&m.theme))
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
		statusText = fmt.Sprintf("%s – trace budget breach %s (+%s)", statusText, humanPhaseName(primary.Kind), overrun)
		if len(timeline.breaches) > 1 {
			statusText = fmt.Sprintf("%s (%d total)", statusText, len(timeline.breaches))
		}
		statusLevel = statusWarn
	}

	m.setStatusMessage(statusMsg{text: statusText, level: statusLevel})

	token := nextResponseRenderToken()
	snapshot := &responseSnapshot{id: token, environment: environment}
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

	cmds := []tea.Cmd{renderHTTPResponseCmd(token, resp, tests, scriptErr, primaryWidth)}
	if tick := m.scheduleResponseLoadingTick(); tick != nil {
		cmds = append(cmds, tick)
	}
	return tea.Batch(cmds...)
}

func (m *Model) responseLoadingMessage() string {
	dots := (m.responseLoadingFrame % 3) + 1
	return responseFormattingBase + strings.Repeat(".", dots)
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
	m.responseLatest = snapshot

	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil || pane.snapshot != snapshot {
			continue
		}
		pane.invalidateCaches()
		if msg.width > 0 && pane.viewport.Width == msg.width {
			pane.wrapCache[responseTabPretty] = cachedWrap{width: msg.width, content: msg.prettyWrapped, base: ensureTrailingNewline(msg.pretty), valid: true}
			rawWrapped := wrapContentForTab(responseTabRaw, snapshot.raw, msg.width)
			pane.ensureRawWrapCache()
			pane.rawWrapCache[snapshot.rawMode] = cachedWrap{width: msg.width, content: rawWrapped, base: ensureTrailingNewline(snapshot.raw), valid: true}

			headersBase := ensureTrailingNewline(msg.headers)
			headersContent := msg.headersWrapped
			if pane.headersView == headersViewRequest {
				headersBase = ensureTrailingNewline(msg.requestHeaders)
				headersContent = msg.requestHeadersWrapped
			}
			pane.wrapCache[responseTabHeaders] = cachedWrap{width: msg.width, content: headersContent, base: headersBase, valid: true}
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

func (m *Model) consumeGRPCResponse(resp *grpcclient.Response, tests []scripts.TestResult, scriptErr error, req *restfile.Request, environment string) tea.Cmd {
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

	headersBuilder := strings.Builder{}
	contentType := strings.TrimSpace(resp.ContentType)
	if len(resp.Headers) > 0 {
		headersBuilder.WriteString("Headers:\n")
		for name, values := range resp.Headers {
			headersBuilder.WriteString(fmt.Sprintf("%s: %s\n", name, strings.Join(values, ", ")))
			if strings.EqualFold(name, "Content-Type") && contentType == "" && len(values) > 0 {
				contentType = strings.TrimSpace(values[0])
			}
		}
	}
	if len(resp.Trailers) > 0 {
		if headersBuilder.Len() > 0 {
			headersBuilder.WriteString("\n")
		}
		headersBuilder.WriteString("Trailers:\n")
		for name, values := range resp.Trailers {
			headersBuilder.WriteString(fmt.Sprintf("%s: %s\n", name, strings.Join(values, ", ")))
		}
	}
	headersContent := strings.TrimRight(headersBuilder.String(), "\n")

	statusLine := fmt.Sprintf("gRPC %s - %s", strings.TrimPrefix(req.GRPC.FullMethod, "/"), resp.StatusCode.String())
	if resp.StatusMessage != "" {
		statusLine += " (" + resp.StatusMessage + ")"
	}

	viewBody := append([]byte(nil), resp.Body...)
	if len(viewBody) == 0 && strings.TrimSpace(resp.Message) != "" {
		viewBody = []byte(resp.Message)
	}
	viewContentType := strings.TrimSpace(resp.ContentType)
	if viewContentType == "" && len(viewBody) > 0 {
		viewContentType = "application/json"
	}

	rawBody := append([]byte(nil), resp.Wire...)
	if len(rawBody) == 0 {
		rawBody = append([]byte(nil), viewBody...)
	}
	rawContentType := strings.TrimSpace(resp.WireContentType)
	if rawContentType == "" {
		rawContentType = contentType
	}
	if rawContentType == "" {
		rawContentType = viewContentType
	}

	meta := binaryview.Analyze(viewBody, viewContentType)
	bv := buildBodyViews(rawBody, rawContentType, &meta, viewBody, viewContentType)

	snapshot := &responseSnapshot{
		pretty:      joinSections(statusLine, bv.pretty),
		raw:         joinSections(statusLine, bv.raw),
		rawSummary:  statusLine,
		headers:     joinSections(statusLine, headersContent),
		ready:       true,
		environment: environment,
		body:        rawBody,
		bodyMeta:    meta,
		contentType: rawContentType,
		rawText:     bv.rawText,
		rawHex:      bv.rawHex,
		rawBase64:   bv.rawBase64,
		rawMode:     bv.mode,
		responseHeaders: func() http.Header {
			if len(resp.Headers) == 0 && len(resp.Trailers) == 0 {
				return nil
			}
			h := make(http.Header, len(resp.Headers)+len(resp.Trailers))
			for k, v := range resp.Headers {
				h[k] = append([]string(nil), v...)
			}
			for k, v := range resp.Trailers {
				h["Grpc-Trailer-"+k] = append([]string(nil), v...)
			}
			return h
		}(),
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

func (m *Model) recordHTTPHistory(resp *httpclient.Response, req *restfile.Request, requestText string, environment string) {
	if m.historyStore == nil || resp == nil || req == nil {
		return
	}

	secrets := m.secretValuesForRedaction(req)
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
	if err := m.historyStore.Append(entry); err != nil {
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn})
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

func (m *Model) recordGRPCHistory(resp *grpcclient.Response, req *restfile.Request, requestText string, environment string) {
	if m.historyStore == nil || resp == nil || req == nil {
		return
	}

	secrets := m.secretValuesForRedaction(req)
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

	if err := m.historyStore.Append(entry); err != nil {
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn})
	}
	m.historySelectedID = entry.ID
	m.syncHistory()
}

// Store one bundled history entry per compare sweep so later views can rebuild
// the tab and metadata without rerunning anything.
func (m *Model) recordCompareHistory(state *compareState) {
	if m.historyStore == nil || state == nil || len(state.results) == 0 {
		return
	}

	baseReq := state.base
	if baseReq == nil {
		for _, res := range state.results {
			if res.Request != nil {
				baseReq = res.Request
				break
			}
		}
	}
	if baseReq == nil {
		return
	}

	entry := history.Entry{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		ExecutedAt:  time.Now(),
		RequestName: requestIdentifier(baseReq),
		Method:      restfile.HistoryMethodCompare,
		URL:         baseReq.URL,
		Description: strings.TrimSpace(baseReq.Metadata.Description),
		Tags:        normalizedTags(baseReq.Metadata.Tags),
		Status:      state.progressSummary(),
		RequestText: renderRequestText(baseReq),
		Compare:     &history.CompareEntry{},
	}
	if state.canceled {
		status := fmt.Sprintf("canceled after %d/%d", len(state.results), len(state.envs))
		if strings.TrimSpace(state.label) != "" {
			status = fmt.Sprintf("%s | %s", strings.TrimSpace(state.label), status)
		}
		entry.Status = status
	}
	if state.spec != nil {
		entry.Compare.Baseline = state.spec.Baseline
	}

	var totalDur time.Duration
	results := make([]history.CompareResult, 0, len(state.results))
	for _, res := range state.results {
		resultEntry := m.buildCompareHistoryResult(res)
		if resultEntry.Duration > 0 {
			totalDur += resultEntry.Duration
		}
		results = append(results, resultEntry)
	}
	entry.Compare.Results = results
	entry.Duration = totalDur
	if entry.Status == "" {
		entry.Status = fmt.Sprintf("Compare %d env", len(results))
	}

	if err := m.historyStore.Append(entry); err != nil {
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn})
		return
	}
	m.historySelectedID = entry.ID
	m.syncHistory()
}

// Redact sensitive values and condense each env run into the snippet the
// history list needs to show meaningful context.
func (m *Model) buildCompareHistoryResult(result compareResult) history.CompareResult {
	env := strings.TrimSpace(result.Environment)
	status, _ := compareRowStatus(&result)

	entry := history.CompareResult{
		Environment: env,
		Status:      status,
		Duration:    compareRowDuration(&result),
		RequestText: strings.TrimSpace(result.RequestText),
	}

	req := result.Request
	if req != nil && strings.TrimSpace(entry.RequestText) == "" {
		entry.RequestText = renderRequestText(req)
	}
	if req != nil {
		secrets := m.secretValuesForEnvironment(env, req)
		maskHeaders := !req.Metadata.AllowSensitiveHeaders
		entry.RequestText = redactHistoryText(entry.RequestText, secrets, maskHeaders)
	}

	switch {
	case result.Canceled:
		entry.Error = "canceled"
		entry.BodySnippet = entry.Error
		entry.StatusCode = 0
	case result.Err != nil:
		entry.Error = errdef.Message(result.Err)
		entry.BodySnippet = entry.Error
		entry.StatusCode = 0
	case result.Response != nil:
		req := result.Request
		entry.BodySnippet = buildCompareHTTPSnippet(result.Response, req, env, m)
		entry.StatusCode = result.Response.StatusCode
	case result.GRPC != nil:
		req := result.Request
		entry.BodySnippet = buildCompareGRPCSnippet(result.GRPC, req, env, m)
		entry.StatusCode = int(result.GRPC.StatusCode)
	default:
		entry.BodySnippet = "No response captured"
		entry.StatusCode = 0
	}

	const limit = 2000
	if len(entry.BodySnippet) > limit {
		entry.BodySnippet = entry.BodySnippet[:limit]
	}
	return entry
}

func buildCompareHTTPSnippet(resp *httpclient.Response, req *restfile.Request, env string, m *Model) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	secrets := m.secretValuesForEnvironment(env, req)
	snippet := string(resp.Body)
	return redactHistoryText(snippet, secrets, false)
}

func buildCompareGRPCSnippet(resp *grpcclient.Response, req *restfile.Request, env string, m *Model) string {
	if resp == nil {
		return ""
	}
	if req != nil && req.Metadata.NoLog {
		return "<body suppressed>"
	}
	secrets := m.secretValuesForEnvironment(env, req)
	return redactHistoryText(resp.Message, secrets, false)
}

func (m *Model) secretValuesForRedaction(req *restfile.Request) []string {
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

	if m.fileVars != nil {
		path := m.documentRuntimePath(m.doc)
		if snapshot := m.fileVars.snapshot(m.cfg.EnvironmentName, path); len(snapshot) > 0 {
			for _, entry := range snapshot {
				if entry.Secret {
					add(entry.Value)
				}
			}
		}
	}

	if m.globals != nil {
		if snapshot := m.globals.snapshot(m.cfg.EnvironmentName); len(snapshot) > 0 {
			for _, entry := range snapshot {
				if entry.Secret {
					add(entry.Value)
				}
			}
		}
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
func (m *Model) populateCompareSnapshotsFromHistory(entry history.Entry, bundle *compareBundle, preferredEnv string) string {
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

func buildHistoryCompareSnapshot(res history.CompareResult, bundle *compareBundle) *responseSnapshot {
	env := strings.TrimSpace(res.Environment)
	if env == "" {
		return nil
	}
	summary := formatHistoryCompareSummary(env, res)
	headers := formatHistoryCompareHeaders(env, res)
	return &responseSnapshot{
		id:            nextResponseRenderToken(),
		pretty:        ensureTrailingNewline(summary),
		raw:           ensureTrailingNewline(summary),
		headers:       ensureTrailingNewline(headers),
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
	if m.historyStore == nil {
		m.historyEntries = nil
		m.historyList.SetItems(nil)
		m.historySelectedID = ""
		m.historyList.Select(-1)
		return
	}

	identifier := ""
	if m.currentRequest != nil {
		identifier = requestIdentifier(m.currentRequest)
	}

	var entries []history.Entry
	switch {
	case history.NormalizeWorkflowName(m.historyWorkflowName) != "":
		entries = m.historyStore.ByWorkflow(m.historyWorkflowName)
	case identifier != "":
		entries = m.historyStore.ByRequest(identifier)
	default:
		entries = m.historyStore.Entries()
	}
	m.historyEntries = entries
	m.historyList.SetItems(makeHistoryItems(entries))
	m.restoreHistorySelection()
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
		return
	}
	if m.historyWorkflowName != "" {
		m.historyWorkflowName = ""
		if m.ready {
			m.syncHistory()
		}
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
		wrapped := wrapToWidth(preview, displayWidth)
		pane.wrapCache[responseTabPretty] = cachedWrap{width: displayWidth, content: wrapped, base: ensureTrailingNewline(snapshot.pretty), valid: true}
		pane.ensureRawWrapCache()
		pane.rawWrapCache[snapshot.rawMode] = cachedWrap{width: displayWidth, content: wrapped, base: ensureTrailingNewline(snapshot.raw), valid: true}
		pane.wrapCache[responseTabHeaders] = cachedWrap{width: displayWidth, content: wrapped, base: ensureTrailingNewline(snapshot.headers), valid: true}
		pane.wrapCache[responseTabDiff] = cachedWrap{}
		pane.wrapCache[responseTabStats] = cachedWrap{}
		pane.viewport.SetContent(wrapped)
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
	m.historyList.Select(0)
	if len(m.historyEntries) == 0 {
		m.historySelectedID = ""
		return
	}
	m.historySelectedID = m.historyEntries[0].ID
}

func (m *Model) replayHistorySelection() tea.Cmd {
	return m.loadHistorySelection(true)
}

func (m *Model) deleteHistoryEntry(id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	if m.historyStore == nil {
		return false, nil
	}
	deleted, err := m.historyStore.Delete(id)
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
		m.setStatusMessage(statusMsg{text: "History entry missing request payload", level: statusWarn})
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
		m.sending = false
		label := strings.TrimSpace(m.statusRequestTitle(doc, req, targetEnv))
		if label == "" {
			label = "history request"
		}
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("Loaded %s from history", label), level: statusInfo})
		if compareBundle != nil {
			focusEnv := strings.TrimSpace(targetEnv)
			if focusEnv == "" && len(compareBundle.Rows) > 0 {
				focusEnv = strings.TrimSpace(compareBundle.Rows[0].Result.Environment)
			}
			if m.compareRun == nil {
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
			}
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

	m.sending = true
	replayTarget := m.statusRequestTarget(doc, req, targetEnv)
	replayText := "Replaying"
	if trimmed := strings.TrimSpace(replayTarget); trimmed != "" {
		replayText = fmt.Sprintf("Replaying %s", trimmed)
	}
	m.setStatusMessage(statusMsg{text: replayText, level: statusInfo})
	return m.executeRequest(doc, req, options, "")
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
	report := buildTimelineReport(tl, traceSpec, rep, newTimelineStyles(&m.theme))
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
		pane.viewport.SetContent(ensureTrailingNewline(snap.pretty))
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
