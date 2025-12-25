package ui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	bubbletextarea "github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"nhooyr.io/websocket"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const (
	consoleHistoryLimit = 128
	wsMetaType          = "resterm.ws.type"
	wsMetaStep          = "resterm.ws.step"
	wsMetaCloseCode     = "resterm.ws.close.code"
	wsMetaCloseReason   = "resterm.ws.close.reason"
)

const websocketConsoleSendTimeout = 5 * time.Second

const (
	consoleInputSingleLineHeight = 1
	consoleInputMultiLineHeight  = 5
)

type websocketConsoleMode int

const (
	consoleModeText websocketConsoleMode = iota
	consoleModeJSON
	consoleModeBase64
	consoleModeFile
)

func (m websocketConsoleMode) String() string {
	switch m {
	case consoleModeText:
		return "text"
	case consoleModeJSON:
		return "json"
	case consoleModeBase64:
		return "base64"
	case consoleModeFile:
		return "file"
	default:
		return "text"
	}
}

type consoleHistoryEntry struct {
	Mode    websocketConsoleMode
	Payload string
	Time    time.Time
}

type websocketConsole struct {
	sessionID  string
	sender     *httpclient.WebSocketSender
	session    *stream.Session
	baseDir    string
	mode       websocketConsoleMode
	input      bubbletextarea.Model
	history    []consoleHistoryEntry
	historyIdx int
	status     string
	active     bool
}

func newWebsocketConsole(
	sessionID string,
	session *stream.Session,
	sender *httpclient.WebSocketSender,
	baseDir string,
) *websocketConsole {
	input := bubbletextarea.New()
	input.SetHeight(consoleInputMultiLineHeight)
	input.SetWidth(defaultConsoleWidth())
	input.Placeholder = "Type message"
	input.ShowLineNumbers = false
	input.Prompt = "> "
	input.CharLimit = 0
	input.MaxHeight = 0
	input.MaxWidth = 0
	input.SetCursor(0)
	input.Focus()

	console := &websocketConsole{
		sessionID:  sessionID,
		sender:     sender,
		session:    session,
		baseDir:    baseDir,
		mode:       consoleModeText,
		input:      input,
		historyIdx: -1,
		active:     true,
	}
	console.updateInputForMode()
	return console
}

func (wc *websocketConsole) focus() {
	wc.active = true
	wc.input.Focus()
}

func (wc *websocketConsole) blur() {
	wc.active = false
	wc.input.Blur()
}

func (wc *websocketConsole) cycleMode() {
	wc.mode++
	if wc.mode > consoleModeFile {
		wc.mode = consoleModeText
	}
	wc.updateInputForMode()
}

func (wc *websocketConsole) sendContext() (context.Context, context.CancelFunc) {
	base := context.Background()
	if wc.session != nil {
		base = wc.session.Context()
	}
	return context.WithTimeout(base, websocketConsoleSendTimeout)
}

func (wc *websocketConsole) setStatus(format string, args ...interface{}) {
	if len(args) == 0 {
		wc.status = format
	} else {
		wc.status = fmt.Sprintf(format, args...)
	}
}

func (wc *websocketConsole) clearStatus() {
	wc.status = ""
}

func (wc *websocketConsole) prependHistory(entry consoleHistoryEntry) {
	if entry.Payload == "" {
		return
	}
	wc.history = append([]consoleHistoryEntry{entry}, wc.history...)
	if len(wc.history) > consoleHistoryLimit {
		wc.history = wc.history[:consoleHistoryLimit]
	}
	wc.historyIdx = -1
}

func (wc *websocketConsole) historyPrev() bool {
	if len(wc.history) == 0 {
		return false
	}
	if wc.historyIdx+1 >= len(wc.history) {
		wc.historyIdx = len(wc.history) - 1
	} else {
		wc.historyIdx++
	}
	wc.applyHistory()
	return true
}

func (wc *websocketConsole) historyNext() bool {
	if wc.historyIdx <= 0 {
		wc.historyIdx = -1
		wc.input.SetValue("")
		return true
	}
	wc.historyIdx--
	wc.applyHistory()
	return true
}

func (wc *websocketConsole) applyHistory() {
	if wc.historyIdx < 0 || wc.historyIdx >= len(wc.history) {
		wc.input.SetValue("")
		wc.input.SetCursor(0)
		return
	}
	entry := wc.history[wc.historyIdx]
	wc.mode = entry.Mode
	wc.updateInputForMode()
	wc.input.SetValue(entry.Payload)
	wc.input.CursorEnd()
}

func (wc *websocketConsole) updateInputForMode() {
	switch wc.mode {
	case consoleModeFile:
		wc.input.SetHeight(consoleInputSingleLineHeight)
	default:
		wc.input.SetHeight(consoleInputMultiLineHeight)
	}
}

func (wc *websocketConsole) view(width int, th theme.Theme) string {
	effectiveWidth := effectiveConsoleWidth(width)
	wc.applyInputStyles(th)
	if wc.active {
		wc.input.Focus()
	} else {
		wc.input.Blur()
	}
	wc.input.SetWidth(effectiveWidth)

	title := th.StreamConsoleTitle.Render("WebSocket Console")
	modeLabel := th.StreamSummary.Render("Mode:")
	modeValue := th.StreamConsoleMode.Render(strings.ToUpper(wc.mode.String()))
	help := th.StreamSummary.Render("(Ctrl+S send, Enter newline, F2 cycle, Ctrl+I toggle)")

	var builder strings.Builder
	builder.WriteString(title)
	builder.WriteByte('\n')
	builder.WriteString(
		lipgloss.JoinHorizontal(lipgloss.Left, modeLabel, " ", modeValue, " ", help),
	)
	builder.WriteByte('\n')
	if wc.status != "" {
		builder.WriteString(th.StreamConsoleStatus.Render(wc.status))
		builder.WriteByte('\n')
	}
	builder.WriteString(wc.input.View())
	return builder.String()
}

func (wc *websocketConsole) applyInputStyles(th theme.Theme) {
	focused := bubbletextarea.Style{
		Base:             th.StreamConsoleInputFocused,
		CursorLine:       th.StreamConsoleInputFocused,
		CursorLineNumber: th.StreamConsoleInputFocused,
		EndOfBuffer:      th.StreamConsoleInputFocused,
		LineNumber:       th.StreamConsoleInputFocused,
		Placeholder:      th.StreamConsoleInputFocused,
		Prompt:           th.StreamConsolePrompt,
		Text:             th.StreamConsoleInputFocused,
	}
	blurred := bubbletextarea.Style{
		Base:             th.StreamConsoleInput,
		CursorLine:       th.StreamConsoleInput,
		CursorLineNumber: th.StreamConsoleInput,
		EndOfBuffer:      th.StreamConsoleInput,
		LineNumber:       th.StreamConsoleInput,
		Placeholder:      th.StreamConsoleInput,
		Prompt:           th.StreamConsolePrompt,
		Text:             th.StreamConsoleInput,
	}
	wc.input.FocusedStyle = focused
	wc.input.BlurredStyle = blurred
}

func defaultConsoleWidth() int {
	return 60
}

func effectiveConsoleWidth(paneWidth int) int {
	if paneWidth <= 0 {
		return defaultConsoleWidth()
	}
	usable := paneWidth - 2
	if usable < 20 {
		return 20
	}
	return usable
}

func (wc *websocketConsole) payload() (func() error, string, string, error) {
	if wc.sender == nil {
		return nil, "", "", fmt.Errorf("websocket sender unavailable")
	}
	meta := map[string]string{wsMetaStep: "interactive"}
	switch wc.mode {
	case consoleModeText:
		value := wc.input.Value()
		meta[wsMetaType] = "text"
		send := func() error {
			ctx, cancel := wc.sendContext()
			defer cancel()
			return wc.sender.SendText(ctx, value, meta)
		}
		return send, fmt.Sprintf("Sent text (%d bytes)", len([]byte(value))), value, nil
	case consoleModeJSON:
		value := wc.input.Value()
		if !json.Valid([]byte(value)) {
			return nil, "", "", fmt.Errorf("invalid JSON payload")
		}
		meta[wsMetaType] = "json"
		send := func() error {
			ctx, cancel := wc.sendContext()
			defer cancel()
			return wc.sender.SendJSON(ctx, value, meta)
		}
		return send, fmt.Sprintf("Sent json (%d bytes)", len([]byte(value))), value, nil
	case consoleModeBase64:
		trimmed := strings.TrimSpace(wc.input.Value())
		if trimmed == "" {
			return nil, "", "", fmt.Errorf("base64 payload empty")
		}
		encoded := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, trimmed)
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, "", "", fmt.Errorf("invalid base64: %w", err)
		}
		payload := append([]byte(nil), decoded...)
		meta[wsMetaType] = "binary"
		send := func() error {
			ctx, cancel := wc.sendContext()
			defer cancel()
			return wc.sender.SendBinary(ctx, payload, meta)
		}
		return send, fmt.Sprintf("Sent binary (%d bytes)", len(decoded)), trimmed, nil
	case consoleModeFile:
		rawPath := strings.TrimSpace(wc.input.Value())
		if rawPath == "" {
			return nil, "", "", fmt.Errorf("file path empty")
		}
		resolved := rawPath
		baseDir := strings.TrimSpace(wc.baseDir)
		if !filepath.IsAbs(resolved) && baseDir != "" {
			resolved = filepath.Join(baseDir, rawPath)
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return nil, "", "", fmt.Errorf("read file: %w", err)
		}
		payload := append([]byte(nil), data...)
		meta[wsMetaType] = "binary"
		send := func() error {
			ctx, cancel := wc.sendContext()
			defer cancel()
			return wc.sender.SendBinary(ctx, payload, meta)
		}
		return send, fmt.Sprintf(
			"Sent file %s (%d bytes)",
			filepath.Base(resolved),
			len(data),
		), rawPath, nil
	default:
		value := wc.input.Value()
		meta[wsMetaType] = "text"
		send := func() error {
			ctx, cancel := wc.sendContext()
			defer cancel()
			return wc.sender.SendText(ctx, value, meta)
		}
		return send, fmt.Sprintf("Sent text (%d bytes)", len([]byte(value))), value, nil
	}
}

func (m *Model) ensureWebSocketConsole(
	sessionID string,
	session *stream.Session,
	sender *httpclient.WebSocketSender,
	req *restfile.Request,
	baseDir string,
) {
	if sessionID == "" || sender == nil {
		return
	}
	if baseDir == "" {
		baseDir = m.sessionBaseDir(req)
	}
	if session == nil && m.sessionHandles != nil {
		session = m.sessionHandles[sessionID]
	}
	if m.wsConsole != nil && m.wsConsole.sessionID == sessionID {
		m.wsConsole.sender = sender
		if session != nil {
			m.wsConsole.session = session
		}
		if baseDir != "" {
			m.wsConsole.baseDir = baseDir
		}
		return
	}
	m.wsConsole = newWebsocketConsole(sessionID, session, sender, baseDir)
}

func (m *Model) handleWebSocketConsoleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	key := msg.String()
	sessionID := m.sessionIDForRequest(m.currentRequest)
	if key == "ctrl+i" || key == "ctrl+alt+i" || key == "alt+ctrl+i" {
		return m.toggleWebSocketConsole(), true
	}

	console := m.wsConsole
	if console == nil || console.sessionID != sessionID {
		return nil, false
	}

	pane := m.pane(responsePanePrimary)
	if pane == nil || pane.activeTab != responseTabStream {
		return nil, false
	}

	if !console.active {
		if key == "f2" {
			console.cycleMode()
			console.clearStatus()
			m.refreshStreamPanes()
			return nil, true
		}
		if key == "enter" {
			console.focus()
			m.refreshStreamPanes()
			return nil, true
		}
		return nil, false
	}

	switch key {
	case "esc":
		console.blur()
		m.refreshStreamPanes()
		return nil, true
	case "f2":
		console.cycleMode()
		console.clearStatus()
		m.refreshStreamPanes()
		return nil, true
	case "ctrl+p":
		console.clearStatus()
		return m.sendConsolePing(), true
	case "ctrl+w":
		console.clearStatus()
		return m.sendConsoleClose(), true
	case "ctrl+l":
		console.clearStatus()
		return m.clearStreamBufferCmd(), true
	case "up":
		if console.historyPrev() {
			console.clearStatus()
			m.refreshStreamPanes()
			return nil, true
		}
	case "down":
		if console.historyNext() {
			console.clearStatus()
			m.refreshStreamPanes()
			return nil, true
		}
	case "ctrl+s", "ctrl+shift+s":
		return m.sendConsolePayload(), true
	case "ctrl+enter", "ctrl+j", "ctrl+m":
		return m.sendConsolePayload(), true
	}

	updated, cmd := console.input.Update(msg)
	console.input = updated
	m.refreshStreamPanes()
	return cmd, true
}

func (m *Model) toggleWebSocketConsole() tea.Cmd {
	var cmds []tea.Cmd
	sessionID := m.sessionIDForRequest(m.currentRequest)
	if sessionID == "" {
		m.setStatusMessage(statusMsg{text: "No active websocket stream", level: statusWarn})
		return nil
	}
	sender := m.wsSenders[sessionID]
	if sender == nil {
		m.setStatusMessage(
			statusMsg{text: "Websocket session not ready for console", level: statusWarn},
		)
		return nil
	}
	baseDir := m.sessionBaseDir(m.currentRequest)
	session := (*stream.Session)(nil)
	if m.sessionHandles != nil {
		session = m.sessionHandles[sessionID]
	}
	if m.wsConsole == nil || m.wsConsole.sessionID != sessionID {
		m.ensureWebSocketConsole(sessionID, session, sender, m.currentRequest, baseDir)
		if cmd := m.focusStreamPane(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.setStatusMessage(
			statusMsg{text: "Websocket console ready (F2 to cycle mode)", level: statusInfo},
		)
	} else {
		if m.wsConsole.active {
			m.wsConsole.blur()
			m.setStatusMessage(statusMsg{text: "Websocket console hidden", level: statusInfo})
		} else {
			m.wsConsole.focus()
			if cmd := m.focusStreamPane(); cmd != nil {
				cmds = append(cmds, cmd)
			}
			m.setStatusMessage(statusMsg{text: "Websocket console active", level: statusInfo})
		}
	}
	m.refreshStreamPanes()
	return batchCommands(cmds...)
}

func (m *Model) focusStreamPane() tea.Cmd {
	if pane := m.pane(responsePanePrimary); pane != nil {
		pane.setActiveTab(responseTabStream)
	}
	m.responsePaneFocus = responsePanePrimary
	if m.focus != focusResponse {
		return m.setFocus(focusResponse)
	}
	m.setLivePane(m.responsePaneFocus)
	return nil
}

func (m *Model) sendConsolePayload() tea.Cmd {
	if m.wsConsole == nil {
		return nil
	}
	mode := m.wsConsole.mode
	sendFunc, status, payloadText, err := m.wsConsole.payload()
	if err != nil {
		m.wsConsole.setStatus("%s", err.Error())
		m.refreshStreamPanes()
		return nil
	}
	m.wsConsole.setStatus("Sending...")
	m.refreshStreamPanes()
	return func() tea.Msg {
		err := sendFunc()
		return wsConsoleResultMsg{err: err, payload: payloadText, mode: mode, status: status}
	}
}

func (m *Model) sendConsolePing() tea.Cmd {
	if m.wsConsole == nil || m.wsConsole.sender == nil {
		return nil
	}
	console := m.wsConsole
	console.setStatus("Sending ping...")
	m.refreshStreamPanes()
	mode := console.mode
	return func() tea.Msg {
		ctx, cancel := console.sendContext()
		defer cancel()
		err := console.sender.Ping(
			ctx,
			map[string]string{wsMetaType: "ping", wsMetaStep: "interactive"},
		)
		return wsConsoleResultMsg{err: err, status: "Ping sent", mode: mode}
	}
}

func (m *Model) sendConsoleClose() tea.Cmd {
	if m.wsConsole == nil || m.wsConsole.sender == nil {
		return nil
	}
	console := m.wsConsole
	console.setStatus("Closing session...")
	m.refreshStreamPanes()
	mode := console.mode
	return func() tea.Msg {
		ctx, cancel := console.sendContext()
		defer cancel()
		err := console.sender.Close(
			ctx,
			websocket.StatusNormalClosure,
			"interactive close",
			map[string]string{wsMetaType: "close", wsMetaStep: "interactive"},
		)
		return wsConsoleResultMsg{err: err, status: "Close frame sent", mode: mode}
	}
}

func (m *Model) clearStreamBufferCmd() tea.Cmd {
	sessionID := m.sessionIDForRequest(m.currentRequest)
	if sessionID != "" {
		if ls := m.ensureLiveSession(sessionID); ls != nil {
			ls.events = nil
			ls.filter = ""
			ls.paused = false
			ls.pausedIndex = -1
			ls.bookmarks = nil
			ls.bookmarkIdx = -1
		}
		if m.wsConsole != nil && m.wsConsole.sessionID == sessionID {
			m.wsConsole.history = nil
			m.wsConsole.historyIdx = -1
			m.wsConsole.clearStatus()
		}
	}
	m.streamFilterActive = false
	m.streamFilterInput.SetValue("")
	m.streamFilterInput.Blur()
	m.refreshStreamPanes()
	return func() tea.Msg {
		return statusMsg{text: "Stream buffer cleared", level: statusInfo}
	}
}

func (m *Model) handleConsoleResult(msg wsConsoleResultMsg) {
	if m.wsConsole == nil {
		return
	}
	if msg.err != nil {
		m.wsConsole.setStatus("%s", msg.err.Error())
		m.refreshStreamPanes()
		return
	}
	mode := msg.mode
	if mode < consoleModeText || mode > consoleModeFile {
		mode = m.wsConsole.mode
	}
	if msg.payload != "" {
		m.wsConsole.prependHistory(
			consoleHistoryEntry{Mode: mode, Payload: msg.payload, Time: time.Now()},
		)
	}
	m.wsConsole.input.SetValue("")
	m.wsConsole.input.SetCursor(0)
	if msg.status != "" {
		m.wsConsole.setStatus("%s", msg.status)
	} else {
		m.wsConsole.clearStatus()
	}
	m.refreshStreamPanes()
}

func (m *Model) sessionBaseDir(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if req.Settings != nil {
		if base, ok := req.Settings["baseDir"]; ok && strings.TrimSpace(base) != "" {
			return strings.TrimSpace(base)
		}
	}
	if m.cfg.HTTPOptions.BaseDir != "" {
		return m.cfg.HTTPOptions.BaseDir
	}
	if strings.TrimSpace(req.URL) != "" && strings.HasPrefix(strings.ToLower(req.URL), "file://") {
		return ""
	}
	if m.currentFile != "" {
		return filepath.Dir(m.currentFile)
	}
	return ""
}
