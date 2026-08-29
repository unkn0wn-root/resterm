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

	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
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

// attachStreamSession starts tracking a session, returning the existing entry
// if it is already tracked. Subscribing seeds the buffer with whatever the
// stream published before the attachment reached the update loop, so nothing
// that arrived in that window is lost.
func (m *Model) attachStreamSession(session *stream.Session) *liveSession {
	if session == nil {
		return nil
	}
	id := session.ID()
	if ls := m.liveSession(id); ls != nil {
		return ls
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
	ls := newLiveSession(id, m.streamMaxEvents)
	ls.kind = session.Kind()
	listener := session.Subscribe()
	ls.lost.evicted = listener.Snapshot.Evicted
	ls.append(listener.Snapshot.Events)
	ls.setState(listener.Snapshot.State, listener.Snapshot.Err)
	m.liveSessions[id] = ls
	m.sessionHandles[id] = session
	go m.runStreamSession(session, listener)
	return ls
}

// handleStreamAttach takes ownership of a session the engine just opened. Every
// map and pane it touches belongs to the update loop, which is why the engine
// hands the session over as a message instead of registering it itself.
func (m *Model) handleStreamAttach(msg streamAttachMsg) {
	if msg.session == nil {
		return
	}
	if msg.gen != m.streamGen {
		msg.session.Cancel()
		return
	}
	m.recordSessionMapping(msg.req, msg.session)
	ls := m.attachStreamSession(msg.session)
	if ls == nil {
		return
	}

	// The pane owns the session until the response carrying it arrives, which
	// is what puts a stream on screen before its request has finished.
	id := msg.session.ID()
	target, _ := paneRunTarget(msg.pane).resolve(m.responseSplit)
	m.pane(target).streamID = id
	m.linkStreamSnapshots(ls)

	if ls.kind == stream.KindWebSocket {
		if m.wsSenders == nil {
			m.wsSenders = make(map[string]*httpx.WebSocketSender)
		}
		if msg.sender != nil {
			m.wsSenders[id] = msg.sender
		}
		m.ensureWebSocketConsole(id, msg.session, msg.sender, msg.req, msg.baseDir)
	}
	m.showStreamTab(id)
}

func (m *Model) runStreamSession(session *stream.Session, listener stream.Listener) {
	if session == nil || listener.C == nil {
		return
	}
	defer listener.Cancel()

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
		m.emitStreamMsg(streamEventMsg{
			sessionID: session.ID(),
			events:    cloneEventSlice(batch),
			dropped:   listener.Dropped(),
		})
		batch = batch[:0]
	}

	// The last batch can be empty, so the final count goes on the completion
	// message too.
	finish := func() {
		flush()
		state, err := session.State()
		m.emitStreamMsg(streamStateMsg{sessionID: session.ID(), state: state, err: err})
		m.emitStreamMsg(streamCompleteMsg{sessionID: session.ID(), dropped: listener.Dropped()})
	}

	for {
		if timer == nil {
			evt, ok := <-listener.C
			if !ok {
				finish()
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
				finish()
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

// liveSession returns the tracked session for id. Entries are created only in
// attachStreamSession, so an unknown id is a stale stream from a workspace
// that has been left.
func (m *Model) liveSession(id string) *liveSession {
	if id == "" {
		return nil
	}
	return m.liveSessions[id]
}

// paneStream resolves the session a pane is showing. A pane owns its session
// directly from the moment it attaches until the response carrying it arrives;
// from then on the snapshot is the owner, which is what lets a pinned pane keep
// a finished transcript while a new run streams into the other pane.
func (m *Model) paneStream(id responsePaneID) (string, *liveSession) {
	pane := m.pane(id)
	if pane == nil {
		return "", nil
	}
	if pane.streamID != "" {
		return pane.streamID, m.liveSession(pane.streamID)
	}
	if pane.snapshot == nil {
		return "", nil
	}
	sid := pane.snapshot.streamID
	if ls := pane.snapshot.stream; ls != nil {
		return sid, ls
	}
	return sid, m.liveSession(sid)
}

func (m *Model) streamIDForPane(id responsePaneID) string {
	sid, _ := m.paneStream(id)
	return sid
}

func (m *Model) streamForPane(id responsePaneID) *liveSession {
	_, ls := m.paneStream(id)
	return ls
}

// activeStreamID is the session the console shortcuts act on. Focus in the
// response pane picks the pane's stream, otherwise it is the one the request
// under the cursor opened.
func (m *Model) activeStreamID() string {
	if m.focus == focusResponse {
		if id := m.streamIDForPane(m.responsePaneFocus); id != "" {
			return id
		}
	}
	return m.sessionIDForRequest(m.currentRequest)
}

// bindSnapshotStream gives a snapshot the session its run opened. The session
// may not have attached yet, so the id is recorded either way and
// linkStreamSnapshots fills in the rest once the attachment lands.
func (m *Model) bindSnapshotStream(snap *responseSnapshot, id string) {
	if snap == nil {
		return
	}
	snap.streamID = id
	if ls := m.liveSession(id); ls != nil {
		m.linkSnapshot(snap, ls)
		m.releaseLiveStream(ls)
	}
}

// linkStreamSnapshots runs the same binding the other way: a session that has
// just attached reaches every snapshot already waiting for it.
func (m *Model) linkStreamSnapshots(ls *liveSession) {
	if ls == nil {
		return
	}
	m.linkSnapshot(m.responseLatest, ls)
	m.linkSnapshot(m.responsePrevious, ls)
	m.linkSnapshot(m.render.snapshot, ls)
	for i := range m.responsePanes {
		m.linkSnapshot(m.responsePanes[i].snapshot, ls)
	}
	m.releaseLiveStream(ls)
}

func (m *Model) linkSnapshot(snap *responseSnapshot, ls *liveSession) {
	if snap == nil || snap.streamID != ls.id {
		return
	}
	snap.stream = ls
	ls.bound = true
}

// releaseLiveStream drops a finished session from the registry once a snapshot
// holds it. Both conditions are needed, in either order: see liveSession.
func (m *Model) releaseLiveStream(ls *liveSession) {
	if ls == nil || !ls.bound || !ls.done {
		return
	}
	delete(m.liveSessions, ls.id)
}

func (m *Model) panesForStream(id string) []responsePaneID {
	if id == "" {
		return nil
	}
	var ids []responsePaneID
	for _, paneID := range m.visiblePaneIDs() {
		if m.streamIDForPane(paneID) == id {
			ids = append(ids, paneID)
		}
	}
	return ids
}

// responseTargetForStream keeps a stream's later responses in the pane that
// already owns it, so a transcript and its response never split up.
func (m *Model) responseTargetForStream(id string) responsePaneID {
	if ids := m.panesForStream(id); len(ids) > 0 {
		return ids[0]
	}
	return m.responseTargetPane()
}

// responseTargetForMsg picks where a response lands: the pane holding its
// stream, else the pane the run was launched from, else the live pane. Focus
// moving mid-run must not drag the response along with it.
func (m *Model) responseTargetForMsg(msg responseMsg) responsePaneID {
	if ids := m.panesForStream(msg.streamID); len(ids) > 0 {
		return ids[0]
	}
	if id, ok := msg.target.resolve(m.responseSplit); ok {
		return id
	}
	return m.responseTargetPane()
}

func (m *Model) handleStreamEvents(msg streamEventMsg) {
	if len(msg.events) == 0 {
		return
	}
	ls := m.liveSession(msg.sessionID)
	if ls == nil {
		return
	}
	ls.lost.listener = msg.dropped
	ls.append(msg.events)
	if ls.state == stream.StateConnecting {
		ls.state = stream.StateOpen
	}
	if m.sending {
		m.setStatusMessage(
			statusMsg{text: "Streaming response (receiving events)", level: statusInfo},
		)
	}
	m.refreshStreamPanes()
}

func (m *Model) handleStreamState(msg streamStateMsg) {
	ls := m.liveSession(msg.sessionID)
	if ls == nil {
		return
	}
	ls.setState(msg.state, msg.err)
	level := statusInfo
	if ls.failed() {
		level = statusWarn
		for _, id := range m.panesForStream(msg.sessionID) {
			m.pane(id).stopStreamTail()
		}
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
	ls := m.liveSession(msg.sessionID)
	if ls == nil {
		return
	}
	ls.lost.listener = msg.dropped
	if ls.state != stream.StateFailed {
		ls.state = stream.StateClosed
	}
	ls.done = true
	filterID := m.streamIDForPane(m.responsePaneFocus)
	if ls.kind == stream.KindWebSocket {
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
	if filterID == msg.sessionID {
		m.streamFilterActive = false
		m.streamFilterInput.SetValue("")
		m.streamFilterInput.Blur()
	}
	status := statusMsg{text: fmt.Sprintf("Stream %s completed", msg.sessionID), level: statusSuccess}
	if ls.failed() {
		detail := streamStateString(ls.state, ls.err)
		if ls.err != nil {
			detail = ls.err.Error()
		}
		status.text = fmt.Sprintf("Stream %s failed: %s", msg.sessionID, detail)
		status.level = statusWarn
	}
	m.setStatusMessage(status)
	m.releaseLiveStream(ls)
	m.refreshStreamPanes()
}

// showStreamTab brings every pane that owns the session onto its transcript.
func (m *Model) showStreamTab(sessionID string) {
	if m.liveSession(sessionID) == nil {
		return
	}
	for _, id := range m.panesForStream(sessionID) {
		pane := m.pane(id)
		pane.setActiveTab(responseTabStream)
		pane.tail = true
		if m.focus == focusResponse && id == m.responsePaneFocus {
			m.setLivePane(id)
		}
	}
	m.refreshStreamPanes()
}

const streamFilterPromptStatus = "Filter stream (Enter to apply, Esc to cancel)"

func (m *Model) closeStreamFilterPrompt() {
	m.streamFilterActive = false
	m.streamFilterInput.SetValue("")
	m.streamFilterInput.Blur()
	m.clearStatusMessages(streamFilterPromptStatus)
}

func (m *Model) handleStreamKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.focus != focusResponse {
		if m.streamFilterActive {
			m.closeStreamFilterPrompt()
		}
		return nil, false
	}
	pane := m.pane(m.responsePaneFocus)
	if pane == nil || pane.activeTab != responseTabStream {
		if m.streamFilterActive {
			m.closeStreamFilterPrompt()
		}
		return nil, false
	}
	sessionID := m.streamIDForPane(m.responsePaneFocus)
	ls := m.streamForPane(m.responsePaneFocus)
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
			m.closeStreamFilterPrompt()
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
	// Terminals that send NUL for ctrl+space surface it as ctrl+@.
	case "ctrl+space", "ctrl+@":
		if ls == nil {
			return nil, false
		}
		ls.setPaused(!ls.paused)
		// Pausing holds the transcript still. It says nothing about which pane
		// the next response belongs in, so followLatest stays untouched.
		pane.tail = !ls.paused
		text := "Stream resumed"
		if ls.paused {
			text = "Stream paused"
		}
		m.setStatusMessage(statusMsg{text: text, level: statusInfo})
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
			statusMsg{text: streamFilterPromptStatus, level: statusInfo},
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
		m.jumpToStreamBookmark(pane, ls, false)
		return nil, true
	case "ctrl+down":
		m.jumpToStreamBookmark(pane, ls, true)
		return nil, true
	}

	return nil, false
}

// jumpToStreamBookmark pauses on the next bookmark in either direction. The
// pause is what freezes the transcript at that point, so tailing has to stop.
func (m *Model) jumpToStreamBookmark(pane *responsePaneState, ls *liveSession, forward bool) {
	if ls == nil || len(ls.bookmarks) == 0 {
		m.setStatusMessage(statusMsg{text: "No bookmarks", level: statusInfo})
		return
	}
	bm := ls.nextBookmark(forward)
	if bm == nil {
		return
	}
	ls.setPaused(true)
	if bm.Index >= 0 && bm.Index <= len(ls.events) {
		ls.pausedIndex = bm.Index
	}
	m.setStatusMessage(
		statusMsg{text: fmt.Sprintf("Bookmark: %s", bookmarkLabel(*bm)), level: statusInfo},
	)
	pane.tail = false
	m.refreshStreamPanes()
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

func (m *Model) streamContentForPane(id responsePaneID) string {
	if m.streamIDForPane(id) == "" {
		return "No stream available\n"
	}
	ls := m.streamForPane(id)
	if ls == nil {
		return "Waiting for stream session\n"
	}
	return m.formatStreamContent(ls)
}

func (m *Model) consoleForPane(id responsePaneID) *websocketConsole {
	sessionID := m.streamIDForPane(id)
	if sessionID == "" {
		return nil
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
	if lost := ls.lost.total(); lost > 0 {
		builder.WriteString(th.StreamError.Render(
			fmt.Sprintf("Transcript incomplete: %d events dropped", lost),
		))
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
	if evt.Kind == stream.KindGRPC {
		return m.renderGRPCEvent(evt)
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

func (m *Model) renderGRPCEvent(evt *stream.Event) string {
	if evt == nil {
		return ""
	}
	th := m.theme
	timestamp := th.StreamTimestamp.Render(evt.Timestamp.Format("15:04:05.000"))
	symbol, dirStyle := m.streamDirectionStyle(evt.Direction)
	direction := dirStyle.Render(symbol)
	parts := []string{timestamp, direction}

	if evt.Direction == stream.DirNA {
		summary := grpcSummaryLine(evt.Metadata)
		if summary != "" {
			style := th.StreamSummary
			status := grpcMetaVal(evt.Metadata, grpcx.MetaStatus)
			if status != "" && !strings.EqualFold(status, "OK") {
				style = th.StreamError
			}
			parts = append(parts, style.Render(summary))
		}
		return strings.Join(filterEmpty(parts), " ")
	}

	if method := grpcMetaVal(evt.Metadata, grpcx.MetaMethod); method != "" {
		parts = append(parts, th.StreamSummary.Render(method))
	}

	label := grpcLabel(evt.Metadata)
	if label == "" {
		label = "message"
	}
	nameStyled := th.StreamEventName.Render(label)
	payload := strings.TrimSpace(string(evt.Payload))
	if payload == "" {
		parts = append(parts, nameStyled, th.StreamData.Render("<empty>"))
		return strings.Join(filterEmpty(parts), " ")
	}
	if formatted, ok := formatJSONForStream(evt.Payload); ok {
		block := nameStyled + "\n" + th.StreamData.Render(
			indentMultiline(formatted, streamJSONIndent),
		)
		parts = append(parts, block)
		return strings.Join(filterEmpty(parts), " ")
	}
	quoted := fmt.Sprintf("\"%s\"", truncatePreview(payload))
	parts = append(parts, nameStyled, th.StreamData.Render(quoted))
	return strings.Join(filterEmpty(parts), " ")
}

func grpcMetaVal(md map[string]string, key string) string {
	if md == nil {
		return ""
	}
	return strings.TrimSpace(md[key])
}

func grpcLabel(md map[string]string) string {
	typ := grpcMetaVal(md, grpcx.MetaMsgType)
	idx := grpcMetaVal(md, grpcx.MetaMsgIndex)
	if typ == "" {
		return idx
	}
	if idx == "" {
		return typ
	}
	return typ + "#" + idx
}

func grpcSummaryLine(md map[string]string) string {
	method := grpcMetaVal(md, grpcx.MetaMethod)
	status := grpcMetaVal(md, grpcx.MetaStatus)
	reason := grpcMetaVal(md, grpcx.MetaReason)
	summary := "summary"
	if method != "" {
		summary += " " + method
	}
	if status != "" {
		summary += " status=" + status
	}
	if reason != "" {
		summary += " reason=" + truncatePreview(reason)
	}
	return summary
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
	case stream.KindGRPC:
		return strings.Contains(strings.ToLower(string(evt.Payload)), filter)
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
		m.refreshStreamPane(id)
	}
}

func (m *Model) refreshStreamPane(id responsePaneID) {
	pane := m.pane(id)
	if pane == nil {
		return
	}
	if pane.wrapCache == nil {
		pane.wrapCache = make(map[responseTab]cachedWrap)
	}
	if pane.activeTab != responseTabStream {
		pane.wrapCache[responseTabStream] = cachedWrap{}
		return
	}
	sessionID := m.streamIDForPane(id)
	content := m.streamContentForPane(id)
	if content == "" {
		content = "<stream idle>\n"
	}
	width := pane.viewport.Width
	// The filter prompt belongs to the pane being typed in, not to every pane
	// showing the same session.
	if m.streamFilterActive && sessionID != "" && id == m.responsePaneFocus {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += m.streamFilterInput.View() + "\n"
	}
	wrapped := wrapStructuredContent(content, width)
	pane.wrapCache[responseTabStream] = cachedWrap{
		width:   width,
		content: wrapped,
		valid:   true,
	}
	decorated := m.applyResponseContentStyles(responseTabStream, wrapped)
	if console := m.consoleForPane(id); console != nil {
		if !strings.HasSuffix(decorated, "\n") {
			decorated += "\n"
		}
		if !strings.HasSuffix(decorated, "\n\n") {
			decorated += "\n"
		}
		decorated += console.view(width, m.theme)
	}
	pane.viewport.SetContent(decorated)
	if pane.tail {
		pane.viewport.GotoBottom()
	} else {
		pane.restoreScrollForActiveTab()
	}
	pane.setCurrPosition()
}
