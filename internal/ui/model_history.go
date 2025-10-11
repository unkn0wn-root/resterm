package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
		cmd := m.consumeRequestError(msg.err)
		m.suppressNextErrorModal = true
		m.setStatusMessage(statusMsg{text: errdef.Message(msg.err), level: level})
		return cmd
	}

	cmd := m.consumeHTTPResponse(msg.response, msg.tests, msg.scriptErr)
	m.recordHTTPHistory(msg.response, msg.executed, msg.requestText, msg.environment)
	return cmd
}

func (m *Model) consumeRequestError(err error) tea.Cmd {
	if err == nil {
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

	code := errdef.CodeOf(err)
	title := requestErrorTitle(code)
	detail := strings.TrimSpace(errdef.Message(err))
	if detail == "" {
		detail = "Request failed with no additional details."
	}
	note := requestErrorNote(code)
	pretty := joinSections(title, detail, note)
	raw := joinSections(title, detail)

	var meta []string
	if code != errdef.CodeUnknown && string(code) != "" {
		meta = append(meta, fmt.Sprintf("Code: %s", strings.ToUpper(string(code))))
	}
	if strings.TrimSpace(note) != "" {
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

func (m *Model) consumeHTTPResponse(resp *httpclient.Response, tests []scripts.TestResult, scriptErr error) tea.Cmd {
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

	switch {
	case scriptErr != nil:
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("Tests error: %v", scriptErr), level: statusWarn})
	case failureCount > 0:
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("%s (%d) – %d test(s) failed", resp.Status, resp.StatusCode, failureCount), level: statusWarn})
	case len(tests) > 0:
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("%s (%d) – all tests passed", resp.Status, resp.StatusCode), level: statusSuccess})
	default:
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("%s (%d)", resp.Status, resp.StatusCode), level: statusSuccess})
	}

	token := nextResponseRenderToken()
	snapshot := &responseSnapshot{id: token}
	m.responseRenderToken = token
	m.responsePending = snapshot
	m.responseLatest = snapshot
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
	snapshot.headers = msg.headers
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
			pane.wrapCache[responseTabPretty] = cachedWrap{width: msg.width, content: msg.prettyWrapped, valid: true}
			pane.wrapCache[responseTabRaw] = cachedWrap{width: msg.width, content: msg.rawWrapped, valid: true}
			pane.wrapCache[responseTabHeaders] = cachedWrap{width: msg.width, content: msg.headersWrapped, valid: true}
		}
		if strings.TrimSpace(snapshot.stats) != "" {
			pane.wrapCache[responseTabStats] = cachedWrap{}
		}
		pane.viewport.GotoTop()
		pane.setCurrPosition()
	}
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane != nil {
			pane.wrapCache[responseTabDiff] = cachedWrap{}
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

func (m *Model) consumeGRPCResponse(resp *grpcclient.Response, tests []scripts.TestResult, scriptErr error, req *restfile.Request) tea.Cmd {
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
	snapshot := &responseSnapshot{
		pretty:  joinSections(statusLine, body),
		raw:     joinSections(statusLine, body),
		headers: joinSections(statusLine, headersContent),
		ready:   true,
	}
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

	snippet := string(resp.Body)
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
		Status:      resp.Status,
		StatusCode:  resp.StatusCode,
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
		m.currentRequest = nil
		return
	}
	prev := m.activeRequestKey
	m.currentRequest = req
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
	statusText := fmt.Sprintf("Previewing %s", requestDisplayName(req))
	return m.applyPreview(preview, statusText)
}

func (m *Model) applyPreview(preview string, statusText string) tea.Cmd {
	snapshot := &responseSnapshot{
		pretty:  preview,
		raw:     preview,
		headers: preview,
		ready:   true,
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
		pane.wrapCache[responseTabPretty] = cachedWrap{width: displayWidth, content: wrapped, valid: true}
		pane.wrapCache[responseTabRaw] = cachedWrap{width: displayWidth, content: wrapped, valid: true}
		pane.wrapCache[responseTabHeaders] = cachedWrap{width: displayWidth, content: wrapped, valid: true}
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
	var parts []string
	if desc := strings.TrimSpace(req.Metadata.Description); desc != "" {
		parts = append(parts, condense(desc, 90))
	}
	if tags := joinTags(req.Metadata.Tags, 5); tags != "" {
		parts = append(parts, tags)
	}
	return strings.Join(parts, " | ")
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

func (m *Model) loadHistorySelection(send bool) tea.Cmd {
	item, ok := m.historyList.SelectedItem().(historyItem)
	if !ok {
		return nil
	}
	entry := item.entry
	if entry.RequestText == "" {
		m.setStatusMessage(statusMsg{text: "History entry missing request payload", level: statusWarn})
		return nil
	}

	doc := parser.Parse(m.currentFile, []byte(entry.RequestText))
	if len(doc.Requests) == 0 {
		m.setStatusMessage(statusMsg{text: "Unable to parse stored request", level: statusError})
		return nil
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

	if !send {
		m.sending = false
		label := strings.TrimSpace(requestDisplayName(req))
		if label == "" {
			label = strings.TrimSpace(fmt.Sprintf("%s %s", req.Method, req.URL))
		}
		if label == "" {
			label = "history request"
		}
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("Loaded %s from history", label), level: statusInfo})
		return nil
	}

	m.sending = true
	m.setStatusMessage(statusMsg{text: fmt.Sprintf("Replaying %s", req.URL), level: statusInfo})
	return m.executeRequest(doc, req, options)
}
