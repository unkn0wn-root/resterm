package ui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"nhooyr.io/websocket"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

const (
	defaultStreamBatchWindow = 25 * time.Millisecond
	defaultStreamMaxEvents   = 5000
	streamSummaryReasonKey   = "resterm.summary.reason"
	streamSummaryBytesKey    = "resterm.summary.bytes"
	streamSummaryEventsKey   = "resterm.summary.events"
	streamJSONIndent         = "  "
)

func (m *Model) attachStreamSession(session *stream.Session) {
	if session == nil {
		return
	}
	if m.streamMgr != nil {
		m.streamMgr.Register(session)
	}
	if m.liveSessions == nil {
		m.liveSessions = make(map[string]*liveSession)
	}
	if m.sessionHandles == nil {
		m.sessionHandles = make(map[string]*stream.Session)
	}
	id := session.ID()
	ls := newLiveSession(id, m.streamMaxEvents)
	ls.kind = session.Kind()
	m.liveSessions[id] = ls
	m.sessionHandles[id] = session
	go m.runStreamSession(session)
	m.emitStreamMsg(streamReadyMsg{sessionID: id})
}

func (m *Model) runStreamSession(session *stream.Session) {
	if session == nil {
		return
	}
	listener := session.Subscribe()
	defer listener.Cancel()

	if snapshot := listener.Snapshot.Events; len(snapshot) > 0 {
		m.emitStreamMsg(streamEventMsg{sessionID: session.ID(), events: cloneEventSlice(snapshot)})
	}

	batchWindow := m.streamBatchWindow
	if batchWindow <= 0 {
		batchWindow = defaultStreamBatchWindow
	}

	var (
		batch []*stream.Event
		timer *time.Timer
	)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		m.emitStreamMsg(streamEventMsg{sessionID: session.ID(), events: cloneEventSlice(batch)})
		batch = batch[:0]
	}

	for {
		if timer == nil {
			evt, ok := <-listener.C
			if !ok {
				flush()
				state, err := session.State()
				m.emitStreamMsg(streamStateMsg{sessionID: session.ID(), state: state, err: err})
				m.emitStreamMsg(streamCompleteMsg{sessionID: session.ID()})
				return
			}
			batch = append(batch, evt)
			timer = time.NewTimer(batchWindow)
			continue
		}

		select {
		case evt, ok := <-listener.C:
			if !ok {
				if !timer.Stop() {
					<-timer.C
				}
				timer = nil
				flush()
				state, err := session.State()
				m.emitStreamMsg(streamStateMsg{sessionID: session.ID(), state: state, err: err})
				m.emitStreamMsg(streamCompleteMsg{sessionID: session.ID()})
				return
			}
			batch = append(batch, evt)
		case <-timer.C:
			timer = nil
			flush()
		}
	}
}

func (m *Model) emitStreamMsg(msg tea.Msg) {
	if msg == nil || m.streamMsgChan == nil {
		return
	}
	m.streamMsgChan <- msg
}

func (m *Model) nextStreamMsgCmd() tea.Cmd {
	if m.streamMsgChan == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-m.streamMsgChan
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *Model) attachSSEHandle(handle *httpclient.StreamHandle, req *restfile.Request) {
	if handle == nil {
		return
	}
	m.recordSessionMapping(req, handle.Session)
	m.attachStreamSession(handle.Session)
}

func (m *Model) attachWebSocketHandle(handle *httpclient.WebSocketHandle, req *restfile.Request) {
	if handle == nil {
		return
	}
	m.recordSessionMapping(req, handle.Session)
	m.attachStreamSession(handle.Session)
	if m.wsSenders == nil {
		m.wsSenders = make(map[string]*httpclient.WebSocketSender)
	}
	sessionID := ""
	if handle.Session != nil {
		sessionID = handle.Session.ID()
	}
	if sessionID != "" && handle.Sender != nil {
		m.wsSenders[sessionID] = handle.Sender
	}
	baseDir := ""
	if handle.Meta.BaseDir != "" {
		baseDir = handle.Meta.BaseDir
	} else {
		baseDir = m.sessionBaseDir(req)
	}
	m.ensureWebSocketConsole(sessionID, handle.Session, handle.Sender, req, baseDir)
}

func (m *Model) ensureLiveSession(id string) *liveSession {
	if id == "" {
		return nil
	}
	if m.liveSessions == nil {
		m.liveSessions = make(map[string]*liveSession)
	}
	if m.streamMaxEvents <= 0 {
		m.streamMaxEvents = defaultStreamMaxEvents
	}
	ls, ok := m.liveSessions[id]
	if !ok {
		ls = newLiveSession(id, m.streamMaxEvents)
		m.liveSessions[id] = ls
	}
	return ls
}

func (m *Model) handleStreamEvents(msg streamEventMsg) {
	if len(msg.events) == 0 {
		return
	}
	ls := m.ensureLiveSession(msg.sessionID)
	if ls != nil {
		ls.append(msg.events)
		if ls.state == stream.StateConnecting {
			ls.state = stream.StateOpen
		}
	}
	if m.sending {
		m.setStatusMessage(
			statusMsg{text: "Streaming response (receiving events)", level: statusInfo},
		)
	}
	m.refreshStreamPanes()
}

func (m *Model) handleStreamState(msg streamStateMsg) {
	ls := m.ensureLiveSession(msg.sessionID)
	if ls != nil {
		ls.setState(msg.state, msg.err)
	}
	level := statusInfo
	if msg.err != nil || msg.state == stream.StateFailed {
		level = statusWarn
	}
	m.setStatusMessage(
		statusMsg{
			text: fmt.Sprintf(
				"Stream %s: %s",
				msg.sessionID,
				streamStateString(msg.state, msg.err),
			),
			level: level,
		},
	)
	m.refreshStreamPanes()
}

func (m *Model) handleStreamComplete(msg streamCompleteMsg) {
	ls := m.ensureLiveSession(msg.sessionID)
	if ls != nil && ls.state != stream.StateFailed {
		ls.state = stream.StateClosed
	}
	if ls != nil && ls.kind == stream.KindWebSocket {
		if m.wsConsole != nil && m.wsConsole.sessionID == msg.sessionID {
			m.wsConsole = nil
		}
		if m.wsSenders != nil {
			delete(m.wsSenders, msg.sessionID)
		}
	}
	if m.sessionHandles != nil {
		delete(m.sessionHandles, msg.sessionID)
	}
	if m.sessionRequests != nil {
		if req := m.sessionRequests[msg.sessionID]; req != nil {
			delete(m.requestSessions, req)
			if m.requestKeySessions != nil {
				if key := requestKey(req); key != "" {
					delete(m.requestKeySessions, key)
				}
			}
		}
		delete(m.sessionRequests, msg.sessionID)
	}
	if sessionID := m.sessionIDForRequest(m.currentRequest); sessionID == msg.sessionID {
		m.streamFilterActive = false
		m.streamFilterInput.SetValue("")
		m.streamFilterInput.Blur()
	}
	m.setStatusMessage(
		statusMsg{text: fmt.Sprintf("Stream %s completed", msg.sessionID), level: statusSuccess},
	)
	m.refreshStreamPanes()
}

func (m *Model) handleStreamReady(msg streamReadyMsg) {
	sessionID := msg.sessionID
	if sessionID == "" {
		return
	}
	currentID := m.sessionIDForRequest(m.currentRequest)
	if currentID == sessionID {
		if pane := m.pane(responsePanePrimary); pane != nil {
			pane.setActiveTab(responseTabStream)
		}
		if m.focus == focusResponse {
			m.setLivePane(m.responsePaneFocus)
		}
	}
	m.refreshStreamPanes()
}

func (m *Model) handleStreamKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.focus != focusResponse {
		return nil, false
	}
	pane := m.pane(m.responsePaneFocus)
	if pane == nil || pane.activeTab != responseTabStream {
		return nil, false
	}
	sessionID := m.sessionIDForRequest(m.currentRequest)
	ls := m.ensureLiveSession(sessionID)
	if sessionID == "" && !m.streamFilterActive {
		return nil, false
	}
	if m.streamFilterActive {
		switch msg.String() {
		case "enter":
			if ls != nil {
				ls.filter = strings.TrimSpace(m.streamFilterInput.Value())
				if ls.filter != "" {
					m.setStatusMessage(
						statusMsg{
							text:  fmt.Sprintf("Stream filter applied: %s", ls.filter),
							level: statusInfo,
						},
					)
				} else {
					m.setStatusMessage(statusMsg{text: "Stream filter cleared", level: statusInfo})
				}
			}
			m.streamFilterActive = false
			m.streamFilterInput.Blur()
			m.refreshStreamPanes()
			return nil, true
		case "esc":
			m.streamFilterActive = false
			m.streamFilterInput.Blur()
			m.refreshStreamPanes()
			return nil, true
		default:
			updated := m.streamFilterInput
			updated, cmd := updated.Update(msg)
			m.streamFilterInput = updated
			m.refreshStreamPanes()
			return cmd, true
		}
	}

	switch msg.String() {
	case "ctrl+space":
		if ls == nil {
			return nil, false
		}
		ls.setPaused(!ls.paused)
		if ls.paused {
			m.setStatusMessage(statusMsg{text: "Stream paused", level: statusInfo})
			if pane != nil {
				pane.followLatest = false
			}
		} else {
			m.setStatusMessage(statusMsg{text: "Stream resumed", level: statusInfo})
			if pane != nil {
				pane.followLatest = true
			}
		}
		m.refreshStreamPanes()
		return nil, true
	case "ctrl+f":
		m.streamFilterActive = true
		m.streamFilterInput.SetValue("")
		if ls != nil {
			m.streamFilterInput.SetValue(ls.filter)
		}
		m.streamFilterInput.CursorEnd()
		m.streamFilterInput.Focus()
		m.setStatusMessage(
			statusMsg{text: "Filter stream (Enter to apply, Esc to cancel)", level: statusInfo},
		)
		m.refreshStreamPanes()
		return nil, true
	case "ctrl+b":
		if ls == nil {
			return nil, false
		}
		label := fmt.Sprintf("Bookmark %d", len(ls.bookmarks)+1)
		ls.addBookmark(label)
		m.setStatusMessage(
			statusMsg{text: fmt.Sprintf("Bookmark added: %s", label), level: statusInfo},
		)
		m.refreshStreamPanes()
		return nil, true
	case "ctrl+up":
		if ls == nil || len(ls.bookmarks) == 0 {
			m.setStatusMessage(statusMsg{text: "No bookmarks", level: statusInfo})
			return nil, true
		}
		if bm := ls.nextBookmark(false); bm != nil {
			ls.setPaused(true)
			if bm.Index >= 0 && bm.Index <= len(ls.events) {
				ls.pausedIndex = bm.Index
			}
			m.setStatusMessage(
				statusMsg{text: fmt.Sprintf("Bookmark: %s", bookmarkLabel(*bm)), level: statusInfo},
			)
			if pane != nil {
				pane.followLatest = false
			}
			m.refreshStreamPanes()
		}
		return nil, true
	case "ctrl+down":
		if ls == nil || len(ls.bookmarks) == 0 {
			m.setStatusMessage(statusMsg{text: "No bookmarks", level: statusInfo})
			return nil, true
		}
		if bm := ls.nextBookmark(true); bm != nil {
			ls.setPaused(true)
			if bm.Index >= 0 && bm.Index <= len(ls.events) {
				ls.pausedIndex = bm.Index
			}
			m.setStatusMessage(
				statusMsg{text: fmt.Sprintf("Bookmark: %s", bookmarkLabel(*bm)), level: statusInfo},
			)
			if pane != nil {
				pane.followLatest = false
			}
			m.refreshStreamPanes()
		}
		return nil, true
	}

	return nil, false
}

func streamStateString(state stream.State, err error) string {
	switch state {
	case stream.StateConnecting:
		return "connecting"
	case stream.StateOpen:
		return "open"
	case stream.StateClosing:
		return "closing"
	case stream.StateClosed:
		if err != nil {
			return "closed (error)"
		}
		return "closed"
	case stream.StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

func (m *Model) recordSessionMapping(req *restfile.Request, session *stream.Session) {
	if req == nil || session == nil {
		return
	}
	if m.requestSessions == nil {
		m.requestSessions = make(map[*restfile.Request]string)
	}
	if m.sessionRequests == nil {
		m.sessionRequests = make(map[string]*restfile.Request)
	}
	if m.requestKeySessions == nil {
		m.requestKeySessions = make(map[string]string)
	}
	id := session.ID()
	m.requestSessions[req] = id
	m.sessionRequests[id] = req
	if key := requestKey(req); key != "" {
		m.requestKeySessions[key] = id
	}
}

func (m *Model) sessionIDForRequest(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if m.requestSessions != nil {
		if id := m.requestSessions[req]; id != "" {
			return id
		}
	}
	if m.requestKeySessions != nil {
		if id := m.requestKeySessions[requestKey(req)]; id != "" {
			return id
		}
	}
	return ""
}

func (m *Model) hasActiveStream() bool {
	if m.wsConsole != nil {
		return true
	}
	if len(m.liveSessions) == 0 {
		return false
	}
	for _, ls := range m.liveSessions {
		if ls == nil {
			continue
		}
		if ls.state != stream.StateClosed || len(ls.events) > 0 {
			return true
		}
	}
	return false
}

func (m *Model) streamContentForPane(id responsePaneID) string {
	req := m.requestForPane(id)
	if req == nil {
		if id == responsePanePrimary {
			return "No active request\n"
		}
		return "No stream available\n"
	}
	sessionID := m.sessionIDForRequest(req)
	if sessionID == "" {
		return "Waiting for stream session\n"
	}
	ls := m.ensureLiveSession(sessionID)
	if ls == nil {
		return "Waiting for stream session\n"
	}
	return m.formatStreamContent(ls)
}

func (m *Model) requestForPane(id responsePaneID) *restfile.Request {
	switch id {
	case responsePanePrimary:
		return m.currentRequest
	default:
		return nil
	}
}

func (m *Model) consoleForPane(id responsePaneID) *websocketConsole {
	req := m.requestForPane(id)
	if req == nil {
		return nil
	}
	sessionID := m.sessionIDForRequest(req)
	if sessionID == "" {
		if m.wsConsole == nil || m.wsConsole.sessionID == "" {
			return nil
		}
		// Allow console display while mapping is pending as long as we're on the active request.
		sessionID = m.wsConsole.sessionID
	}
	if m.wsConsole == nil {
		return nil
	}
	if m.wsConsole.sessionID != sessionID {
		return nil
	}
	return m.wsConsole
}

func (m *Model) formatStreamContent(ls *liveSession) string {
	if ls == nil {
		return ""
	}
	th := m.theme
	var builder strings.Builder

	state := streamStateString(ls.state, ls.err)
	header := th.StreamSummary.Render(fmt.Sprintf("Session %s (%s)", ls.id, state))
	builder.WriteString(header)
	builder.WriteByte('\n')

	if ls.paused {
		builder.WriteString(th.StreamSummary.Render("[PAUSED]"))
		builder.WriteByte('\n')
	}
	if ls.filter != "" {
		builder.WriteString(th.StreamSummary.Render(fmt.Sprintf("Filter: %s", ls.filter)))
		builder.WriteByte('\n')
	}
	if len(ls.bookmarks) > 0 {
		builder.WriteString(
			th.StreamSummary.Render(fmt.Sprintf("Bookmarks: %d", len(ls.bookmarks))),
		)
		builder.WriteByte('\n')
	}
	if ls.err != nil {
		builder.WriteString(th.StreamError.Render(fmt.Sprintf("Error: %v", ls.err)))
		builder.WriteByte('\n')
	}

	limit := len(ls.events)
	if ls.paused && ls.pausedIndex >= 0 && ls.pausedIndex <= len(ls.events) {
		limit = ls.pausedIndex
	}
	bookmarkMap := make(map[int]string, len(ls.bookmarks))
	for _, bm := range ls.bookmarks {
		bookmarkMap[bm.Index] = bookmarkLabel(bm)
	}
	filter := strings.ToLower(strings.TrimSpace(ls.filter))
	found := false
	for idx := 0; idx < limit; idx++ {
		evt := ls.events[idx]
		if filter != "" && !matchesFilter(filter, evt) {
			continue
		}
		line := m.renderStreamEvent(evt)
		if strings.TrimSpace(line) == "" {
			continue
		}
		if label, ok := bookmarkMap[idx]; ok {
			labelText := strings.TrimSpace(label)
			if labelText != "" {
				labelStyled := th.StreamSummary.Render(fmt.Sprintf("★ %s", labelText))
				line = lipgloss.JoinHorizontal(lipgloss.Left, labelStyled, " ", line)
			} else {
				line = lipgloss.JoinHorizontal(
					lipgloss.Left,
					th.StreamSummary.Render("★"),
					" ",
					line,
				)
			}
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
		found = true
	}
	if !found {
		if ls.filter != "" && len(ls.events) > 0 {
			builder.WriteString(th.StreamSummary.Render("<no events match filter>"))
			builder.WriteByte('\n')
			return builder.String()
		}
		builder.WriteString(th.StreamSummary.Render("<no events>"))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func (m *Model) renderStreamEvent(evt *stream.Event) string {
	if evt == nil {
		return ""
	}
	th := m.theme
	timestamp := th.StreamTimestamp.Render(evt.Timestamp.Format("15:04:05.000"))
	symbol, dirStyle := m.streamDirectionStyle(evt.Direction)
	direction := dirStyle.Render(symbol)
	parts := []string{timestamp, direction}

	switch evt.Kind {
	case stream.KindSSE:
		if evt.Direction == stream.DirNA {
			reason := strings.TrimSpace(evt.Metadata[streamSummaryReasonKey])
			if reason == "" {
				reason = "complete"
			}
			events := evt.Metadata[streamSummaryEventsKey]
			bytes := evt.Metadata[streamSummaryBytesKey]
			summary := fmt.Sprintf("summary reason=%s", truncatePreview(reason))
			if events != "" {
				summary += fmt.Sprintf(" events=%s", events)
			}
			if bytes != "" {
				summary += fmt.Sprintf(" bytes=%s", bytes)
			}
			parts = append(parts, th.StreamSummary.Render(summary))
			return strings.Join(filterEmpty(parts), " ")
		}
		name := evt.SSE.Name
		if name == "" {
			name = "message"
		}
		comment := strings.TrimSpace(evt.SSE.Comment)
		trimmedPayload := strings.TrimSpace(string(evt.Payload))
		if evt.SSE.ID != "" {
			parts = append(
				parts,
				th.StreamSummary.Render(fmt.Sprintf("id=%s", truncatePreview(evt.SSE.ID))),
			)
		}
		nameStyled := th.StreamEventName.Render(name)
		if trimmedPayload == "" {
			payloadStyled := th.StreamData.Render("<empty>")
			if comment != "" {
				payloadStyled = lipgloss.JoinHorizontal(
					lipgloss.Left,
					payloadStyled,
					th.StreamSummary.Render(fmt.Sprintf("// %s", truncatePreview(comment))),
				)
			}
			parts = append(parts, nameStyled, payloadStyled)
			return strings.Join(filterEmpty(parts), " ")
		}
		if formatted, ok := formatJSONForStream(evt.Payload); ok {
			indented := indentMultiline(formatted, streamJSONIndent)
			block := nameStyled + "\n" + th.StreamData.Render(indented)
			if comment != "" {
				block += "\n" + th.StreamSummary.Render(
					fmt.Sprintf("// %s", truncatePreview(comment)),
				)
			}
			parts = append(parts, block)
			return strings.Join(filterEmpty(parts), " ")
		}
		payload := fmt.Sprintf("\"%s\"", truncatePreview(trimmedPayload))
		payloadStyled := th.StreamData.Render(payload)
		if comment != "" {
			payloadStyled = lipgloss.JoinHorizontal(
				lipgloss.Left,
				payloadStyled,
				th.StreamSummary.Render(fmt.Sprintf("// %s", truncatePreview(comment))),
			)
		}
		parts = append(parts, nameStyled, payloadStyled)
		return strings.Join(filterEmpty(parts), " ")
	case stream.KindWebSocket:
		typ := evt.Metadata[wsMetaType]
		if typ == "" {
			typ = opcodeToType(evt.WS.Opcode)
		}
		step := strings.TrimSpace(evt.Metadata[wsMetaStep])
		if step != "" {
			parts = append(parts, th.StreamSummary.Render(fmt.Sprintf("[%s]", step)))
		}
		typeStyled := th.StreamEventName.Render(typ)
		switch typ {
		case "text", "json", "pong", "ping":
			trimmed := strings.TrimSpace(string(evt.Payload))
			if (typ == "json" || typ == "text") && len(evt.Payload) > 0 {
				if formatted, ok := formatJSONForStream(evt.Payload); ok {
					indented := indentMultiline(formatted, streamJSONIndent)
					parts = append(parts, typeStyled+"\n"+th.StreamData.Render(indented))
					break
				}
			}
			if trimmed == "" {
				parts = append(parts, typeStyled, th.StreamData.Render("<empty>"))
				break
			}
			payload := fmt.Sprintf("\"%s\"", truncatePreview(trimmed))
			parts = append(parts, typeStyled, th.StreamData.Render(payload))
		case "binary":
			preview := base64.StdEncoding.EncodeToString(evt.Payload)
			if preview == "" {
				preview = "<empty>"
			}
			size := fmt.Sprintf("%d bytes", len(evt.Payload))
			parts = append(
				parts,
				typeStyled,
				th.StreamBinary.Render(size),
				th.StreamBinary.Render(truncatePreview(preview)),
			)
		case "close":
			reason := strings.TrimSpace(evt.Metadata[wsMetaCloseReason])
			code := evt.Metadata[wsMetaCloseCode]
			if code == "" && evt.WS.Code != 0 {
				code = strconv.Itoa(int(evt.WS.Code))
			}
			info := fmt.Sprintf("close %s", code)
			style := th.StreamSummary
			if evt.WS.Code != 0 && evt.WS.Code != websocket.StatusNormalClosure {
				style = th.StreamError
			}
			if reason != "" {
				info = fmt.Sprintf("%s %s", info, truncatePreview(reason))
			}
			parts = append(parts, style.Render(info))
		default:
			parts = append(
				parts,
				typeStyled,
				th.StreamSummary.Render(fmt.Sprintf("(%d bytes)", len(evt.Payload))),
			)
		}
	default:
		parts = append(parts, th.StreamSummary.Render("event"))
	}

	return strings.Join(filterEmpty(parts), " ")
}

func (m *Model) streamDirectionStyle(dir stream.Direction) (string, lipgloss.Style) {
	switch dir {
	case stream.DirSend:
		return "→", m.theme.StreamDirectionSend
	case stream.DirReceive:
		return "←", m.theme.StreamDirectionReceive
	default:
		return "·", m.theme.StreamDirectionInfo
	}
}

func filterEmpty(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		filtered = append(filtered, v)
	}
	return filtered
}

func formatJSONForStream(raw []byte) (string, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", false
	}
	if !json.Valid(trimmed) {
		return "", false
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, trimmed, "", streamJSONIndent); err != nil {
		return "", false
	}
	formatted := strings.TrimRight(buf.String(), "\n")
	return formatted, true
}

func indentMultiline(value string, indent string) string {
	if value == "" {
		return indent
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

func truncatePreview(value string) string {
	const limit = 120
	clean := strings.ReplaceAll(value, "\r", "")
	clean = strings.ReplaceAll(clean, "\t", " ")
	clean = strings.ReplaceAll(clean, "\n", " ⏎ ")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return ""
	}
	clean = strings.Join(strings.Fields(clean), " ")
	runes := []rune(clean)
	if len(runes) <= limit {
		return clean
	}
	return string(runes[:limit]) + "…"
}

func bookmarkLabel(b streamBookmark) string {
	if strings.TrimSpace(b.Label) != "" {
		return strings.TrimSpace(b.Label)
	}
	return b.Created.Format("15:04:05")
}

func matchesFilter(filter string, evt *stream.Event) bool {
	if filter == "" || evt == nil {
		return true
	}
	filter = strings.ToLower(filter)
	if evt.Metadata != nil {
		for _, v := range evt.Metadata {
			if strings.Contains(strings.ToLower(v), filter) {
				return true
			}
		}
	}
	switch evt.Kind {
	case stream.KindSSE:
		if strings.Contains(strings.ToLower(evt.SSE.Name), filter) {
			return true
		}
		if strings.Contains(strings.ToLower(evt.SSE.Comment), filter) {
			return true
		}
		if strings.Contains(strings.ToLower(evt.SSE.ID), filter) {
			return true
		}
		return strings.Contains(strings.ToLower(string(evt.Payload)), filter)
	case stream.KindWebSocket:
		if strings.Contains(strings.ToLower(string(evt.Payload)), filter) {
			return true
		}
		if strings.Contains(strings.ToLower(evt.WS.Reason), filter) {
			return true
		}
		if evt.Metadata != nil {
			if typ, ok := evt.Metadata[wsMetaType]; ok &&
				strings.Contains(strings.ToLower(typ), filter) {
				return true
			}
		}
		if len(evt.Payload) > 0 {
			encoded := base64.StdEncoding.EncodeToString(evt.Payload)
			if strings.Contains(strings.ToLower(encoded), filter) {
				return true
			}
		}
	}
	return false
}

func opcodeToType(op int) string {
	switch op {
	case 0x1:
		return "text"
	case 0x2:
		return "binary"
	case 0x9:
		return "ping"
	case 0xA:
		return "pong"
	case 0x8:
		return "close"
	default:
		return "unknown"
	}
}

func (m *Model) refreshStreamPanes() {
	for _, id := range m.visiblePaneIDs() {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		if pane.wrapCache == nil {
			pane.wrapCache = make(map[responseTab]cachedWrap)
		}
		if pane.activeTab != responseTabStream {
			pane.wrapCache[responseTabStream] = cachedWrap{}
			continue
		}
		req := m.requestForPane(id)
		sessionID := m.sessionIDForRequest(req)
		streamContent := m.streamContentForPane(id)
		if streamContent == "" {
			streamContent = "<stream idle>\n"
		}
		width := pane.viewport.Width
		if m.streamFilterActive && sessionID != "" &&
			sessionID == m.sessionIDForRequest(m.currentRequest) {
			if !strings.HasSuffix(streamContent, "\n") {
				streamContent += "\n"
			}
			filterInput := m.streamFilterInput
			streamContent += filterInput.View()
			streamContent += "\n"
			m.streamFilterInput = filterInput
		}
		wrappedStream := wrapStructuredContent(streamContent, width)
		pane.wrapCache[responseTabStream] = cachedWrap{
			width:   width,
			content: wrappedStream,
			base:    streamContent,
			valid:   true,
		}
		decorated := m.applyResponseContentStyles(responseTabStream, wrappedStream)
		console := m.consoleForPane(id)
		if console != nil {
			if !strings.HasSuffix(decorated, "\n") {
				decorated += "\n"
			}
			if !strings.HasSuffix(decorated, "\n\n") {
				decorated += "\n"
			}
			consoleView := console.view(width, m.theme)
			decorated += consoleView
		}
		pane.viewport.SetContent(decorated)
		if pane.followLatest {
			pane.viewport.GotoBottom()
		} else {
			pane.restoreScrollForActiveTab()
		}
		pane.setCurrPosition()
	}
}
