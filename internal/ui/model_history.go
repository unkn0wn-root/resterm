package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/pkg/restfile"
	"google.golang.org/grpc/codes"
)

const responseLoadingTickInterval = 200 * time.Millisecond

type responseLoadingTickMsg struct{}

func (m *Model) handleResponseMessage(msg responseMsg) tea.Cmd {
	m.lastError = nil
	m.testResults = msg.tests
	m.scriptError = msg.scriptErr

	if msg.grpc != nil {
		if msg.err != nil {
			m.lastError = msg.err
		} else {
			m.lastError = nil
		}
		cmd := m.consumeGRPCResponse(msg.grpc, msg.tests, msg.scriptErr, msg.executed)
		m.recordGRPCHistory(msg.grpc, msg.executed, msg.requestText, msg.environment)
		return cmd
	}

	if msg.err != nil {
		m.lastError = msg.err
		m.lastResponse = nil
		m.lastGRPC = nil
		code := errdef.CodeOf(msg.err)
		level := statusError
		if code == errdef.CodeScript {
			level = statusWarn
		}
		m.statusMessage = statusMsg{text: errdef.Message(msg.err), level: level}
		return nil
	}

	cmd := m.consumeHTTPResponse(msg.response, msg.tests, msg.scriptErr)
	m.recordHTTPHistory(msg.response, msg.executed, msg.requestText, msg.environment)
	return cmd
}

func (m *Model) consumeHTTPResponse(resp *httpclient.Response, tests []scripts.TestResult, scriptErr error) tea.Cmd {
	m.lastGRPC = nil
	m.lastResponse = resp
	m.resetResponseViews()

	if resp == nil {
		m.responseLoading = false
		m.responseLoadingFrame = 0
		width := m.responseViewport.Width
		if width <= 0 {
			width = defaultResponseViewportWidth
		}
		centered := centerContent(noResponseMessage, width, m.responseViewport.Height)
		m.responseViewport.SetContent(wrapToWidth(centered, width))
		m.responseViewport.GotoTop()
		return nil
	}

	failureCount := 0
	for _, result := range tests {
		if !result.Passed {
			failureCount++
		}
	}

	switch {
	case scriptErr != nil:
		m.statusMessage = statusMsg{text: fmt.Sprintf("Tests error: %v", scriptErr), level: statusWarn}
	case failureCount > 0:
		m.statusMessage = statusMsg{text: fmt.Sprintf("%s (%d) – %d test(s) failed", resp.Status, resp.StatusCode, failureCount), level: statusWarn}
	case len(tests) > 0:
		m.statusMessage = statusMsg{text: fmt.Sprintf("%s (%d) – all tests passed", resp.Status, resp.StatusCode), level: statusSuccess}
	default:
		m.statusMessage = statusMsg{text: fmt.Sprintf("%s (%d)", resp.Status, resp.StatusCode), level: statusSuccess}
	}

	token := nextResponseRenderToken()
	m.responseRenderToken = token
	m.responseLoading = true
	m.responseLoadingFrame = 0
	m.showResponseLoadingMessage()
	m.responseViewport.GotoTop()

	width := m.responseViewport.Width
	if width <= 0 {
		width = defaultResponseViewportWidth
	}

	cmds := []tea.Cmd{renderHTTPResponseCmd(token, resp, tests, scriptErr, width)}
	if tick := m.scheduleResponseLoadingTick(); tick != nil {
		cmds = append(cmds, tick)
	}
	return tea.Batch(cmds...)
}

func (m *Model) resetResponseViews() {
	m.prettyView = ""
	m.rawView = ""
	m.headersView = ""
	m.prettyWrapCache = cachedWrap{}
	m.rawWrapCache = cachedWrap{}
	m.headersWrapCache = cachedWrap{}
	m.responseLoadingFrame = 0
}

func (m *Model) showResponseLoadingMessage() {
	m.responseViewport.SetContent(m.responseLoadingMessage())
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

	m.responseLoading = false
	m.responseLoadingFrame = 0
	m.prettyView = msg.pretty
	m.rawView = msg.raw
	m.headersView = msg.headers

	if msg.width > 0 {
		m.prettyWrapCache = cachedWrap{width: msg.width, content: msg.prettyWrapped, valid: true}
		m.rawWrapCache = cachedWrap{width: msg.width, content: msg.rawWrapped, valid: true}
		m.headersWrapCache = cachedWrap{width: msg.width, content: msg.headersWrapped, valid: true}
	} else {
		m.prettyWrapCache = cachedWrap{}
		m.rawWrapCache = cachedWrap{}
		m.headersWrapCache = cachedWrap{}
	}

	currentWidth := m.responseViewport.Width
	if currentWidth <= 0 {
		currentWidth = defaultResponseViewportWidth
	}

	if msg.width != currentWidth {
		m.responseViewport.SetContent(responseReflowingMessage)
		m.responseViewport.GotoTop()
		return wrapResponseContentCmd(m.responseRenderToken, m.prettyView, m.rawView, m.headersView, currentWidth)
	}

	if cmd := m.syncResponseContent(); cmd != nil {
		return cmd
	}
	m.responseViewport.GotoTop()
	return nil
}

func (m *Model) handleResponseLoadingTick() tea.Cmd {
	if !m.responseLoading {
		return nil
	}
	m.responseLoadingFrame = (m.responseLoadingFrame + 1) % 3
	m.showResponseLoadingMessage()
	return m.scheduleResponseLoadingTick()
}

func (m *Model) handleResponseWrap(msg responseWrapMsg) tea.Cmd {
	if msg.token == "" || msg.token != m.responseRenderToken {
		return nil
	}

	m.prettyWrapCache = cachedWrap{width: msg.width, content: msg.prettyWrapped, valid: true}
	m.rawWrapCache = cachedWrap{width: msg.width, content: msg.rawWrapped, valid: true}
	m.headersWrapCache = cachedWrap{width: msg.width, content: msg.headersWrapped, valid: true}

	m.responseViewport.GotoTop()
	return m.syncResponseContent()
}

func (m *Model) consumeGRPCResponse(resp *grpcclient.Response, tests []scripts.TestResult, scriptErr error, req *restfile.Request) tea.Cmd {
	m.lastResponse = nil
	m.lastGRPC = resp
	m.responseLoading = false
	m.responseRenderToken = ""
	m.resetResponseViews()

	if resp == nil {
		m.responseViewport.SetContent("No gRPC response")
		return nil
	}

	headersBuilder := strings.Builder{}
	if len(resp.Headers) > 0 {
		headersBuilder.WriteString("Headers:\n")
		for name, values := range resp.Headers {
			headersBuilder.WriteString(fmt.Sprintf("%s: %s\n", name, strings.Join(values, ", ")))
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

	body := strings.TrimSpace(resp.Message)
	if body == "" {
		body = "<empty>"
	}
	statusLine := fmt.Sprintf("gRPC %s - %s", strings.TrimPrefix(req.GRPC.FullMethod, "/"), resp.StatusCode.String())
	if resp.StatusMessage != "" {
		statusLine += " (" + resp.StatusMessage + ")"
	}
	m.prettyView = joinSections(statusLine, body)
	m.rawView = joinSections(statusLine, body)
	m.headersView = joinSections(statusLine, headersContent)

	switch {
	case resp.StatusCode != codes.OK:
		m.statusMessage = statusMsg{text: statusLine, level: statusWarn}
	default:
		m.statusMessage = statusMsg{text: statusLine, level: statusSuccess}
	}
	m.responseViewport.GotoTop()
	return m.syncResponseContent()
}

func (m *Model) recordHTTPHistory(resp *httpclient.Response, req *restfile.Request, requestText string, environment string) {
	if m.historyStore == nil || resp == nil || req == nil {
		return
	}

	snippet := string(resp.Body)
	if req.Metadata.NoLog {
		snippet = "<body suppressed>"
	} else if len(snippet) > 2000 {
		snippet = snippet[:2000]
	}

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
		RequestText: requestText,
	}

	if err := m.historyStore.Append(entry); err != nil {
		m.statusMessage = statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn}
	}
	m.historySelectedID = entry.ID
	m.syncHistory()
}

func (m *Model) recordGRPCHistory(resp *grpcclient.Response, req *restfile.Request, requestText string, environment string) {
	if m.historyStore == nil || resp == nil || req == nil {
		return
	}

	snippet := resp.Message
	if req.Metadata.NoLog {
		snippet = "<body suppressed>"
	} else if len(snippet) > 2000 {
		snippet = snippet[:2000]
	}

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
		RequestText: requestText,
	}

	if err := m.historyStore.Append(entry); err != nil {
		m.statusMessage = statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn}
	}
	m.historySelectedID = entry.ID
	m.syncHistory()
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

	entries := m.historyStore.ByRequest(identifier)
	m.historyEntries = entries
	m.historyList.SetItems(makeHistoryItems(entries))
	m.restoreHistorySelection()
}

func (m *Model) syncRequestList(doc *restfile.Document) {
	items, listItems := buildRequestItems(doc)
	m.requestItems = items
	if len(listItems) == 0 {
		m.requestList.SetItems(nil)
		m.requestList.Select(-1)
		return
	}
	m.requestList.SetItems(listItems)
	if m.selectRequestItemByKey(m.activeRequestKey) {
		if idx := m.requestList.Index(); idx >= 0 && idx < len(m.requestItems) {
			m.currentRequest = m.requestItems[idx].request
		}
		return
	}
	if len(m.requestItems) > 0 {
		m.requestList.Select(0)
		m.currentRequest = m.requestItems[0].request
		m.activeRequestTitle = requestDisplayName(m.requestItems[0].request)
		m.activeRequestKey = requestKey(m.requestItems[0].request)
	}
}

func (m *Model) setActiveRequest(req *restfile.Request) {
	if req == nil {
		m.activeRequestTitle = ""
		m.activeRequestKey = ""
		return
	}
	m.activeRequestTitle = requestDisplayName(req)
	m.activeRequestKey = requestKey(req)
	_ = m.selectRequestItemByKey(m.activeRequestKey)
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
	m.currentRequest = item.request
}

func (m *Model) previewRequest(req *restfile.Request) tea.Cmd {
	if req == nil {
		return nil
	}
	preview := renderRequestText(req)
	m.activeTab = responseTabPretty
	m.responseRenderToken = ""
	m.responseLoading = false
	m.resetResponseViews()
	m.prettyView = preview
	m.rawView = preview
	m.headersView = preview
	width := m.responseViewport.Width
	if width <= 0 {
		width = defaultResponseViewportWidth
	}
	wrapped := wrapToWidth(preview, width)
	m.prettyWrapCache = cachedWrap{width: width, content: wrapped, valid: true}
	m.rawWrapCache = cachedWrap{width: width, content: wrapped, valid: true}
	m.headersWrapCache = cachedWrap{width: width, content: wrapped, valid: true}
	m.responseViewport.SetContent(wrapped)
	m.responseViewport.GotoTop()
	m.testResults = nil
	m.scriptError = nil
	return func() tea.Msg {
		return statusMsg{text: fmt.Sprintf("Previewing %s", requestDisplayName(req)), level: statusInfo}
	}
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

func requestDisplayName(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if name := strings.TrimSpace(req.Metadata.Name); name != "" {
		return name
	}
	url := strings.TrimSpace(req.URL)
	if len(url) > 60 {
		url = url[:57] + "..."
	}
	return fmt.Sprintf("%s %s", req.Method, url)
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
	item, ok := m.historyList.SelectedItem().(historyItem)
	if !ok {
		return nil
	}
	entry := item.entry
	if entry.RequestText == "" {
		return func() tea.Msg {
			return statusMsg{text: "History entry missing request payload", level: statusWarn}
		}
	}

	doc := parser.Parse(m.currentFile, []byte(entry.RequestText))
	if len(doc.Requests) == 0 {
		return func() tea.Msg {
			return statusMsg{text: "Unable to parse stored request", level: statusError}
		}
	}

	docReq := doc.Requests[0]
	if entry.Environment != "" {
		m.cfg.EnvironmentName = entry.Environment
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
	m.editor.SetValue(entry.RequestText)
	m.editor.SetCursor(0)
	m.testResults = nil
	m.scriptError = nil
	m.sending = true
	m.statusMessage = statusMsg{text: fmt.Sprintf("Replaying %s", req.URL), level: statusInfo}

	return m.executeRequest(doc, req, options)
}
