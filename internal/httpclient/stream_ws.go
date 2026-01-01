package httpclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	wsMetaType        = "resterm.ws.type"
	wsMetaStep        = "resterm.ws.step"
	wsMetaClosedBy    = "resterm.ws.closed.by"
	wsMetaCloseCode   = "resterm.ws.close.code"
	wsMetaCloseReason = "resterm.ws.close.reason"
)

const defaultWebSocketSendQueue = 32

const (
	wsOpcodeText   = 0x1
	wsOpcodeBinary = 0x2
	wsOpcodeClose  = 0x8
	wsOpcodePing   = 0x9
	wsOpcodePong   = 0xA

	websocketControlMaxPayload = 125
)

type WebSocketEvent struct {
	Step      string    `json:"step,omitempty"`
	Direction string    `json:"direction"`
	Type      string    `json:"type"`
	Size      int       `json:"size"`
	Text      string    `json:"text,omitempty"`
	Base64    string    `json:"base64,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Code      int       `json:"code,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

type WebSocketSummary struct {
	SentCount     int           `json:"sentCount"`
	ReceivedCount int           `json:"receivedCount"`
	Duration      time.Duration `json:"duration"`
	ClosedBy      string        `json:"closedBy"`
	CloseCode     int           `json:"closeCode,omitempty"`
	CloseReason   string        `json:"closeReason,omitempty"`
}

type WebSocketTranscript struct {
	Events  []WebSocketEvent `json:"events"`
	Summary WebSocketSummary `json:"summary"`
}

type WebSocketHandle struct {
	Session *stream.Session
	Meta    StreamMeta
	Sender  *WebSocketSender
}

type wsOutboundKind int

const (
	wsOutboundMessage wsOutboundKind = iota
	wsOutboundClose
	wsOutboundPing
	wsOutboundPong
)

type wsOutbound struct {
	ctx      context.Context
	kind     wsOutboundKind
	msgType  websocket.MessageType
	payload  []byte
	code     websocket.StatusCode
	reason   string
	metadata map[string]string
	result   chan error
}

func (c *Client) StartWebSocket(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*WebSocketHandle, *Response, error) {
	if req == nil || req.WebSocket == nil {
		return nil, nil, errdef.New(errdef.CodeHTTP, "websocket metadata missing")
	}

	wsOpts := req.WebSocket.Options
	handshakeCtx, handshakeCancel := ctxWithTimeout(ctx, wsOpts.HandshakeTimeout)
	defer handshakeCancel()

	httpReq, effectiveOpts, err := c.prepareHTTPRequest(handshakeCtx, req, resolver, opts)
	if err != nil {
		handshakeCancel()
		return nil, nil, err
	}

	factory := c.resolveHTTPFactory()
	if factory == nil {
		handshakeCancel()
		return nil, nil, errdef.New(errdef.CodeHTTP, "http client factory unavailable")
	}
	client, err := factory(effectiveOpts)
	if err != nil {
		handshakeCancel()
		return nil, nil, err
	}
	client.Timeout = 0

	dialOpts := wsDialOptions(httpReq, wsOpts, client)

	dial := c.wsDial
	if dial == nil {
		dial = websocket.Dial
	}

	start := time.Now()
	conn, resp, err := dial(handshakeCtx, httpReq.URL.String(), dialOpts)
	if err != nil {
		handshakeCancel()
		if resp != nil {
			fallback, convErr := buildWebSocketFallback(resp, req, start)
			if convErr != nil {
				return nil, nil, convErr
			}
			return nil, fallback, nil
		}
		return nil, nil, errdef.Wrap(errdef.CodeHTTP, err, "dial websocket")
	}
	// Swap contexts now - handshake timeout shouldn't kill the active connection.
	// The new context lets the session run until explicitly closed or parent cancels.
	handshakeCancel()
	sessionCtx, sessionCancel := context.WithCancel(ctx)

	meta := buildStreamMeta(
		req,
		httpReq,
		resp,
		effectiveOpts.BaseDir,
		metaDefaults{
			status: "101 Switching Protocols",
			code:   http.StatusSwitchingProtocols,
			proto:  "HTTP/1.1",
		},
	)

	session := stream.NewSession(sessionCtx, stream.KindWebSocket, stream.Config{})
	session.MarkOpen()

	runtime := &wsRuntime{
		conn:    conn,
		session: session,
		writeCh: make(chan wsOutbound, defaultWebSocketSendQueue),
		cancel:  sessionCancel,
		pulse:   make(chan struct{}, 1),
	}
	runtime.touchActivity()

	if wsOpts.MaxMessageBytes > 0 {
		conn.SetReadLimit(wsOpts.MaxMessageBytes)
	}

	if wsOpts.IdleTimeout > 0 {
		go runtime.idleWatch(wsOpts.IdleTimeout)
	}

	go runtime.readLoop()
	go runtime.writeLoop()

	sender := &WebSocketSender{runtime: runtime}
	return &WebSocketHandle{Session: session, Meta: meta, Sender: sender}, nil, nil
}

func wsDialOptions(
	req *http.Request,
	wsOpts restfile.WebSocketOptions,
	client *http.Client,
) *websocket.DialOptions {
	var hdr http.Header
	if req != nil {
		hdr = cloneHdr(req.Header)
	}
	opts := &websocket.DialOptions{
		HTTPHeader:   hdr,
		Subprotocols: append([]string(nil), wsOpts.Subprotocols...),
		HTTPClient:   client,
	}
	if wsOpts.CompressionSet {
		if wsOpts.Compression {
			opts.CompressionMode = websocket.CompressionNoContextTakeover
		} else {
			opts.CompressionMode = websocket.CompressionDisabled
		}
	}
	return opts
}

func (c *Client) ExecuteWebSocket(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*Response, error) {
	handle, fallback, err := c.StartWebSocket(ctx, req, resolver, opts)
	if err != nil {
		return nil, err
	}
	if fallback != nil {
		return fallback, nil
	}

	return c.CompleteWebSocket(ctx, handle, req, opts)
}

func (c *Client) CompleteWebSocket(
	ctx context.Context,
	handle *WebSocketHandle,
	req *restfile.Request,
	opts Options,
) (*Response, error) {
	if handle == nil || handle.Session == nil || handle.Sender == nil {
		return nil, errdef.New(errdef.CodeHTTP, "websocket session not available")
	}

	session := handle.Session
	listener := session.Subscribe()
	defer listener.Cancel()

	acc := newWSAccumulator()
	for _, evt := range listener.Snapshot.Events {
		acc.consume(evt)
	}

	eventsDone := make(chan struct{})
	go func() {
		for evt := range listener.C {
			acc.consume(evt)
		}
		close(eventsDone)
	}()

	sender := handle.Sender

	baseDir := handle.Meta.BaseDir
	if baseDir == "" {
		baseDir = opts.BaseDir
	}
	closedByScript, err := c.runWSSteps(session, sender, req, baseDir, opts)
	if err != nil {
		return nil, err
	}

	if !closedByScript {
		_ = sender.Close(
			session.Context(),
			websocket.StatusNormalClosure,
			"resterm closed",
			map[string]string{wsMetaType: "close", wsMetaStep: "auto-close"},
		)
	}

	select {
	case <-session.Done():
	case <-ctx.Done():
		session.Cancel()
		<-session.Done()
	}

	<-eventsDone

	state, stateErr := session.State()
	stats := session.StatsSnapshot()
	if !stats.EndedAt.IsZero() {
		acc.summary.Duration = stats.EndedAt.Sub(stats.StartedAt)
	} else {
		acc.summary.Duration = time.Since(handle.Meta.ConnectedAt)
	}
	applyWebSocketSummaryDefaults(&acc.summary, state, stateErr)

	transcript := WebSocketTranscript{Events: acc.events, Summary: acc.summary}
	body, err := json.MarshalIndent(transcript, "", "  ")
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "encode websocket transcript")
	}

	headers := cloneHdr(handle.Meta.Headers)
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set(streamHeaderType, "websocket")
	headers.Set(streamHeaderSummary, fmt.Sprintf(
		"sent=%d recv=%d closed=%s",
		transcript.Summary.SentCount,
		transcript.Summary.ReceivedCount,
		transcript.Summary.ClosedBy))

	meta := handle.Meta
	if meta.Status == "" {
		meta.Status = "101 Switching Protocols"
	}
	if meta.StatusCode == 0 {
		meta.StatusCode = http.StatusSwitchingProtocols
	}
	if meta.Proto == "" {
		meta.Proto = "HTTP/1.1"
	}
	return streamResp(meta, headers, body, acc.summary.Duration), nil
}

func wsRecvWindow(opts restfile.WebSocketOptions) time.Duration {
	win := 250 * time.Millisecond
	if opts.IdleTimeout <= 0 {
		return win
	}
	half := opts.IdleTimeout / 2
	if half > 0 && half < win {
		return half
	}
	return win
}

func (c *Client) runWSSteps(
	session *stream.Session,
	sender *WebSocketSender,
	req *restfile.Request,
	baseDir string,
	opts Options,
) (bool, error) {
	if req == nil || req.WebSocket == nil {
		return false, errdef.New(errdef.CodeHTTP, "websocket request missing")
	}

	wsReq := req.WebSocket
	ctx := session.Context()
	recvWindow := wsRecvWindow(wsReq.Options)
	fallbacks, allowRaw := resolveFileLookup(baseDir, opts)
	closedByScript := false

	for idx, step := range wsReq.Steps {
		sender.touch()

		if err := ensureSessionAlive(session); err != nil {
			return false, err
		}

		label := fmt.Sprintf("%d:%s", idx+1, string(step.Type))
		meta := map[string]string{wsMetaStep: label}
		switch step.Type {
		case restfile.WebSocketStepSendText:
			meta[wsMetaType] = "text"
			if err := sender.SendText(ctx, step.Value, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepSendJSON:
			payload := strings.TrimSpace(step.Value)
			if payload == "" {
				payload = "{}"
			}
			meta[wsMetaType] = "json"
			if err := sender.SendJSON(ctx, payload, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepSendBase64:
			meta[wsMetaType] = "binary"
			if err := sender.SendBase64(ctx, step.Value, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepSendFile:
			data, _, readErr := c.readFileWithFallback(
				step.File,
				baseDir,
				fallbacks,
				allowRaw,
				"websocket payload file",
			)
			if readErr != nil {
				session.Cancel()
				return false, readErr
			}
			meta[wsMetaType] = "binary"
			if err := sender.SendBinary(ctx, data, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepPing:
			meta[wsMetaType] = "ping"
			if err := sender.Ping(ctx, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepPong:
			if err := sender.Pong(ctx, step.Value, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepWait:
			if err := waitForDuration(ctx, step.Duration); err != nil {
				session.Cancel()
				return false, err
			}
		case restfile.WebSocketStepClose:
			meta[wsMetaType] = "close"
			code := websocket.StatusNormalClosure
			if step.Code != 0 {
				code = websocket.StatusCode(step.Code)
			}
			if err := sender.Close(ctx, code, step.Reason, meta); err != nil {
				session.Cancel()
				return false, err
			}
			closedByScript = true
		}
	}

	return closedByScript, nil
}

func buildWebSocketFallback(
	httpResp *http.Response,
	req *restfile.Request,
	started time.Time,
) (*Response, error) {
	if httpResp == nil {
		return nil, errdef.New(errdef.CodeHTTP, "websocket handshake response unavailable")
	}

	var body []byte
	if httpResp.Body != nil {
		data, err := io.ReadAll(httpResp.Body)
		closeErr := httpResp.Body.Close()
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "read websocket handshake body")
		}
		if closeErr != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, closeErr, "close websocket handshake body")
		}
		body = data
	}

	return respFromHTTP(httpResp.Request, httpResp, req, body, time.Since(started)), nil
}

func waitForWindow(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	_ = waitForDuration(ctx, d)
}

func waitForDuration(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func ensureSessionAlive(session *stream.Session) error {
	if session == nil {
		return errdef.New(errdef.CodeHTTP, "websocket session missing")
	}
	if err := session.Context().Err(); err != nil {
		if sessErr := session.Err(); sessErr != nil {
			return sessErr
		}
		return errdef.Wrap(errdef.CodeHTTP, err, "websocket session closed")
	}
	return nil
}

type wsRuntime struct {
	conn    *websocket.Conn
	session *stream.Session
	writeCh chan wsOutbound
	cancel  context.CancelFunc
	pulse   chan struct{}
	once    sync.Once
}

func (rt *wsRuntime) readLoop() {
	session := rt.session
	ctx := session.Context()
	defer rt.shutdown()

	for {
		msgType, data, err := rt.conn.Read(ctx)
		if err != nil {
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				meta := map[string]string{
					wsMetaType:        "close",
					wsMetaClosedBy:    "server",
					wsMetaCloseCode:   strconv.Itoa(int(ce.Code)),
					wsMetaCloseReason: ce.Reason,
				}
				session.Publish(&stream.Event{
					Kind:      stream.KindWebSocket,
					Direction: stream.DirReceive,
					Timestamp: time.Now(),
					Metadata:  meta,
					WS: stream.WSMetadata{
						Opcode: wsOpcodeClose,
						Code:   ce.Code,
						Reason: ce.Reason,
					},
				})
				session.Close(nil)
				return
			}
			if ctx.Err() != nil {
				session.Close(ctx.Err())
			} else {
				session.Close(errdef.Wrap(errdef.CodeHTTP, err, "read websocket message"))
			}
			return
		}

		rt.touchActivity()

		payload := append([]byte(nil), data...)
		metadata := map[string]string{}
		opcode := wsOpcodeBinary
		if msgType == websocket.MessageText {
			opcode = wsOpcodeText
		}

		typ := opcodeToType(opcode)
		metadata[wsMetaType] = typ

		session.Publish(&stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirReceive,
			Timestamp: time.Now(),
			Metadata:  metadata,
			Payload:   payload,
			WS: stream.WSMetadata{
				Opcode: opcode,
			},
		})
	}
}

func (rt *wsRuntime) idleWatch(limit time.Duration) {
	if limit <= 0 {
		return
	}
	timer := time.NewTimer(limit)
	defer timer.Stop()

	for {
		select {
		case <-rt.session.Context().Done():
			return
		case <-timer.C:
			meta := map[string]string{
				wsMetaClosedBy:    "timeout",
				wsMetaCloseReason: fmt.Sprintf("idle timeout after %s", limit),
			}
			rt.session.Publish(&stream.Event{
				Kind:      stream.KindWebSocket,
				Direction: stream.DirNA,
				Timestamp: time.Now(),
				Metadata:  meta,
			})
			rt.session.Close(nil)
			return
		case <-rt.pulse:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(limit)
		}
	}
}

func (rt *wsRuntime) touchActivity() {
	select {
	case rt.pulse <- struct{}{}:
	default:
	}
}

func (rt *wsRuntime) writeLoop() {
	session := rt.session
	ctx := session.Context()
	defer rt.shutdown()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-rt.writeCh:
			if !ok {
				return
			}
			if err := rt.performWrite(msg); err != nil {
				if msg.result != nil {
					msg.result <- err
				}
				session.Close(err)
				return
			}
			if msg.result != nil {
				msg.result <- nil
			}
			if msg.kind == wsOutboundClose {
				return
			}
		}
	}
}

func (rt *wsRuntime) performWrite(msg wsOutbound) error {
	session := rt.session
	ctx := msg.ctx
	if ctx == nil {
		ctx = session.Context()
	}

	switch msg.kind {
	case wsOutboundMessage:
		opcode := wsOpcodeBinary
		if msg.msgType == websocket.MessageText {
			opcode = wsOpcodeText
		}
		if err := rt.conn.Write(ctx, msg.msgType, msg.payload); err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "send websocket frame")
		}
		rt.touchActivity()

		payload := append([]byte(nil), msg.payload...)
		metadata := cloneMetadata(msg.metadata)
		if metadata == nil {
			metadata = map[string]string{}
		}
		if _, ok := metadata[wsMetaType]; !ok {
			metadata[wsMetaType] = opcodeToType(opcode)
		}

		session.Publish(&stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirSend,
			Timestamp: time.Now(),
			Metadata:  metadata,
			Payload:   payload,
			WS: stream.WSMetadata{
				Opcode: opcode,
			},
		})
		return nil
	case wsOutboundPing:
		if err := rt.conn.Ping(ctx); err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "send websocket ping")
		}
		rt.touchActivity()

		metadata := cloneMetadata(msg.metadata)
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata[wsMetaType] = "ping"
		session.Publish(&stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirSend,
			Timestamp: time.Now(),
			Metadata:  metadata,
			WS: stream.WSMetadata{
				Opcode: wsOpcodePing,
			},
		})
		return nil
	case wsOutboundPong:
		payload := append([]byte(nil), msg.payload...)
		if len(payload) > websocketControlMaxPayload {
			return errdef.New(
				errdef.CodeHTTP,
				"websocket pong payload exceeds %d bytes",
				websocketControlMaxPayload,
			)
		}
		if err := wsWriteControl(rt.conn, ctx, wsOpcodePong, payload); err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "send websocket pong")
		}
		rt.touchActivity()

		metadata := cloneMetadata(msg.metadata)
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata[wsMetaType] = "pong"
		session.Publish(&stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirSend,
			Timestamp: time.Now(),
			Metadata:  metadata,
			Payload:   payload,
			WS: stream.WSMetadata{
				Opcode: wsOpcodePong,
			},
		})
		return nil
	case wsOutboundClose:
		session.MarkClosing()
		if err := rt.conn.Close(msg.code, msg.reason); err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "close websocket")
		}
		rt.touchActivity()

		metadata := cloneMetadata(msg.metadata)
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata[wsMetaType] = "close"
		metadata[wsMetaClosedBy] = "client"
		metadata[wsMetaCloseCode] = strconv.Itoa(int(msg.code))
		if msg.reason != "" {
			metadata[wsMetaCloseReason] = msg.reason
		}
		session.Publish(&stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirSend,
			Timestamp: time.Now(),
			Metadata:  metadata,
			WS: stream.WSMetadata{
				Opcode: wsOpcodeClose,
				Code:   msg.code,
				Reason: msg.reason,
			},
		})
		return nil
	default:
		return nil
	}
}

func (rt *wsRuntime) shutdown() {
	rt.once.Do(func() {
		close(rt.writeCh)
		if rt.cancel != nil {
			rt.cancel()
		}
		if err := rt.conn.Close(websocket.StatusNormalClosure, ""); err != nil &&
			!errors.Is(err, net.ErrClosed) && !errors.Is(err, context.Canceled) {
			if rt.session != nil {
				rt.session.Close(errdef.Wrap(errdef.CodeHTTP, err, "close websocket connection"))
			}
		}
	})
}

type WebSocketSender struct {
	runtime *wsRuntime
}

func (s *WebSocketSender) touch() {
	if s == nil || s.runtime == nil {
		return
	}
	s.runtime.touchActivity()
}

// Multiple contexts racing - per-message timeout, session lifetime, and write completion.
// Nested selects give priority to results already available before checking timeouts.
func (s *WebSocketSender) enqueue(msg wsOutbound) (err error) {
	if msg.ctx == nil {
		msg.ctx = s.runtime.session.Context()
	}

	defer func() {
		if r := recover(); r != nil {
			err = errdef.New(errdef.CodeHTTP, "websocket session closed")
			if msg.result != nil {
				msg.result <- err
			}
		}
	}()

	select {
	case <-s.runtime.session.Context().Done():
		return errdef.New(errdef.CodeHTTP, "websocket session closed")
	default:
	}

	select {
	case s.runtime.writeCh <- msg:
		if msg.result != nil {
			for {
				select {
				case err = <-msg.result:
					return err
				case <-msg.ctx.Done():
					select {
					case err = <-msg.result:
						return err
					default:
						if msg.kind == wsOutboundClose {
							return nil
						}
						return msg.ctx.Err()
					}
				case <-s.runtime.session.Context().Done():
					select {
					case err = <-msg.result:
						return err
					default:
						if msg.kind == wsOutboundClose {
							return nil
						}
						return errdef.New(errdef.CodeHTTP, "websocket session closed")
					}
				}
			}
		}
		return nil
	case <-msg.ctx.Done():
		if msg.result != nil {
			select {
			case err = <-msg.result:
				return err
			default:
				if msg.kind == wsOutboundClose {
					return nil
				}
			}
		}
		return msg.ctx.Err()
	case <-s.runtime.session.Context().Done():
		return errdef.New(errdef.CodeHTTP, "websocket session closed")
	}
}

func (s *WebSocketSender) SendText(ctx context.Context, text string, meta map[string]string) error {
	payload := []byte(text)
	m := cloneMetadata(meta)
	if m == nil {
		m = map[string]string{}
	}
	m[wsMetaType] = "text"
	msg := wsOutbound{
		ctx:      ctx,
		kind:     wsOutboundMessage,
		msgType:  websocket.MessageText,
		payload:  payload,
		metadata: m,
		result:   make(chan error, 1),
	}
	return s.enqueue(msg)
}

func (s *WebSocketSender) SendJSON(
	ctx context.Context,
	jsonPayload string,
	meta map[string]string,
) error {
	if !json.Valid([]byte(jsonPayload)) {
		return errdef.New(errdef.CodeHTTP, "invalid json payload for websocket send")
	}
	m := cloneMetadata(meta)
	if m == nil {
		m = map[string]string{}
	}
	m[wsMetaType] = "json"
	msg := wsOutbound{
		ctx:      ctx,
		kind:     wsOutboundMessage,
		msgType:  websocket.MessageText,
		payload:  []byte(jsonPayload),
		metadata: m,
		result:   make(chan error, 1),
	}
	return s.enqueue(msg)
}

func (s *WebSocketSender) SendBinary(
	ctx context.Context,
	data []byte,
	meta map[string]string,
) error {
	payload := append([]byte(nil), data...)
	m := cloneMetadata(meta)
	if m == nil {
		m = map[string]string{}
	}
	m[wsMetaType] = "binary"
	msg := wsOutbound{
		ctx:      ctx,
		kind:     wsOutboundMessage,
		msgType:  websocket.MessageBinary,
		payload:  payload,
		metadata: m,
		result:   make(chan error, 1),
	}
	return s.enqueue(msg)
}

func (s *WebSocketSender) SendBase64(
	ctx context.Context,
	data string,
	meta map[string]string,
) error {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return errdef.Wrap(errdef.CodeHTTP, err, "decode base64 payload")
	}
	return s.SendBinary(ctx, decoded, meta)
}

func (s *WebSocketSender) Ping(ctx context.Context, meta map[string]string) error {
	msg := wsOutbound{
		ctx:      ctx,
		kind:     wsOutboundPing,
		metadata: cloneMetadata(meta),
		result:   make(chan error, 1),
	}
	return s.enqueue(msg)
}

func (s *WebSocketSender) Pong(ctx context.Context, payload string, meta map[string]string) error {
	data := []byte(payload)
	if len(data) > websocketControlMaxPayload {
		return errdef.New(
			errdef.CodeHTTP,
			"websocket pong payload exceeds %d bytes",
			websocketControlMaxPayload,
		)
	}
	m := cloneMetadata(meta)
	if m == nil {
		m = map[string]string{}
	}
	m[wsMetaType] = "pong"
	msg := wsOutbound{
		ctx:      ctx,
		kind:     wsOutboundPong,
		payload:  append([]byte(nil), data...),
		metadata: m,
		result:   make(chan error, 1),
	}
	return s.enqueue(msg)
}

func (s *WebSocketSender) Close(
	ctx context.Context,
	code websocket.StatusCode,
	reason string,
	meta map[string]string,
) error {
	msg := wsOutbound{
		ctx:      ctx,
		kind:     wsOutboundClose,
		code:     code,
		reason:   reason,
		metadata: cloneMetadata(meta),
		result:   make(chan error, 1),
	}
	return s.enqueue(msg)
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func opcodeToType(op int) string {
	switch op {
	case wsOpcodeText:
		return "text"
	case wsOpcodeBinary:
		return "binary"
	case wsOpcodePing:
		return "ping"
	case wsOpcodePong:
		return "pong"
	case wsOpcodeClose:
		return "close"
	default:
		return "unknown"
	}
}

type wsAccumulator struct {
	events  []WebSocketEvent
	summary WebSocketSummary
}

func newWSAccumulator() *wsAccumulator {
	return &wsAccumulator{
		events:  make([]WebSocketEvent, 0, 16),
		summary: WebSocketSummary{},
	}
}

func (a *wsAccumulator) consume(evt *stream.Event) {
	if evt == nil {
		return
	}
	meta := evt.Metadata
	typ := ""
	if meta != nil {
		typ = meta[wsMetaType]
	}
	switch evt.Direction {
	case stream.DirSend, stream.DirReceive:
		if typ == "" {
			typ = opcodeToType(evt.WS.Opcode)
		}
		jsonEvt := WebSocketEvent{
			Direction: directionToString(evt.Direction),
			Type:      typ,
			Timestamp: evt.Timestamp,
			Size:      len(evt.Payload),
		}
		if meta != nil {
			if step, ok := meta[wsMetaStep]; ok {
				jsonEvt.Step = step
			}
		}
		switch typ {
		case "text", "json", "pong", "ping":
			jsonEvt.Text = string(evt.Payload)
		case "binary":
			jsonEvt.Base64 = base64.StdEncoding.EncodeToString(evt.Payload)
		case "close":
			if meta != nil {
				if codeStr, ok := meta[wsMetaCloseCode]; ok {
					if code, err := strconv.Atoi(codeStr); err == nil {
						jsonEvt.Code = code
					}
				}
				if reason, ok := meta[wsMetaCloseReason]; ok {
					jsonEvt.Reason = reason
				}
			}
			if evt.WS.Code != 0 && jsonEvt.Code == 0 {
				jsonEvt.Code = int(evt.WS.Code)
			}
			if evt.WS.Reason != "" && jsonEvt.Reason == "" {
				jsonEvt.Reason = evt.WS.Reason
			}
		}
		a.events = append(a.events, jsonEvt)
		if evt.Direction == stream.DirSend {
			a.summary.SentCount++
		} else {
			a.summary.ReceivedCount++
		}
		if typ == "close" {
			if meta != nil {
				if by, ok := meta[wsMetaClosedBy]; ok {
					a.summary.ClosedBy = by
				}
				if reason, ok := meta[wsMetaCloseReason]; ok && reason != "" {
					a.summary.CloseReason = reason
				}
				if codeStr, ok := meta[wsMetaCloseCode]; ok {
					if code, err := strconv.Atoi(codeStr); err == nil {
						a.summary.CloseCode = code
					}
				}
			}
			if jsonEvt.Code != 0 {
				a.summary.CloseCode = jsonEvt.Code
			}
			if jsonEvt.Reason != "" {
				a.summary.CloseReason = jsonEvt.Reason
			}
		}
	case stream.DirNA:
		if meta != nil {
			if by, ok := meta[wsMetaClosedBy]; ok {
				a.summary.ClosedBy = by
			}
			if codeStr, ok := meta[wsMetaCloseCode]; ok {
				if code, err := strconv.Atoi(codeStr); err == nil {
					a.summary.CloseCode = code
				}
			}
			if reason, ok := meta[wsMetaCloseReason]; ok {
				a.summary.CloseReason = reason
			}
		}
	}
}

func directionToString(dir stream.Direction) string {
	switch dir {
	case stream.DirSend:
		return "send"
	case stream.DirReceive:
		return "receive"
	default:
		return "info"
	}
}

func applyWebSocketSummaryDefaults(sum *WebSocketSummary, state stream.State, stateErr error) {
	if sum == nil {
		return
	}
	if sum.ClosedBy == "" {
		if state == stream.StateFailed || stateErr != nil {
			sum.ClosedBy = "error"
			if sum.CloseReason == "" && stateErr != nil {
				sum.CloseReason = stateErr.Error()
			}
		} else {
			sum.ClosedBy = "client"
		}
	}
	if sum.CloseReason == "" && stateErr != nil && sum.ClosedBy == "error" {
		sum.CloseReason = stateErr.Error()
	}
}
