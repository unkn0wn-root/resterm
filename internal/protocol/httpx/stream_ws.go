package httpx

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"nhooyr.io/websocket"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/k8s"
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

// Values used for WebSocketSummary.ClosedBy.
const (
	wsClosedByServer   = "server"
	wsClosedByClient   = "client"
	wsClosedByTimeout  = "timeout"
	wsClosedByCanceled = "canceled"
	wsClosedByError    = "error"
)

const defaultWebSocketSendQueue = 32

const webSocketSwitchingProtocolsStatus = "101 Switching Protocols"

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
	Dropped       int64         `json:"dropped,omitempty"`
}

type WebSocketTranscript struct {
	Events  []WebSocketEvent `json:"events"`
	Summary WebSocketSummary `json:"summary"`
}

// Err reports the failure that ended the session. A close either side asked for
// and a timeout are normal endings and return nil.
func (s WebSocketSummary) Err() error {
	switch s.ClosedBy {
	case wsClosedByCanceled:
		return diag.New(
			diag.ClassCanceled,
			cmp.Or(s.CloseReason, "websocket stream canceled"),
		)
	case wsClosedByError:
		return diag.New(
			diag.ClassProtocol,
			cmp.Or(s.CloseReason, "websocket stream failed"),
		)
	default:
		return nil
	}
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
		return nil, nil, diag.New(diag.ClassProtocol, "websocket metadata missing")
	}

	effective := applyRequestSettings(opts, req.Settings)
	if verErr := checkWebSocketHTTPVersion(effective.HTTPVersion); verErr != nil {
		return nil, nil, verErr
	}

	wsOpts := req.WebSocket.Options
	handshakeCtx, handshakeCancel := ctxWithTimeout(ctx, wsOpts.HandshakeTimeout)
	defer handshakeCancel()

	httpReq, effectiveOpts, err := c.prepareHTTPRequestWithOpts(
		handshakeCtx,
		req,
		resolver,
		effective,
	)
	if err != nil {
		handshakeCancel()
		return nil, nil, err
	}

	client, err := c.streamClient(effectiveOpts)
	if err != nil {
		handshakeCancel()
		return nil, nil, err
	}

	dialOpts := wsDialOptions(httpReq, wsOpts, client)

	dial := c.wsDial
	if dial == nil {
		dial = websocket.Dial
	}

	var k8sDiag *k8s.RequestDiag
	if effectiveOpts.K8s != nil && effectiveOpts.K8s.Active() {
		boundCtx, reqDiag := k8s.BindRequestContext(handshakeCtx)
		handshakeCtx = boundCtx
		httpReq = httpReq.WithContext(boundCtx)
		k8sDiag = reqDiag
	}

	start := time.Now()
	conn, resp, err := dial(handshakeCtx, httpReq.URL.String(), dialOpts)
	if err != nil {
		handshakeCancel()
		if resp != nil {
			fallback, convErr := buildWebSocketFallback(resp, req, start, effectiveOpts)
			if convErr != nil {
				return nil, nil, convErr
			}
			return nil, fallback, nil
		}
		if k8sDiag != nil {
			err = k8s.AnnotateRequestError(err, start, k8sDiag)
		}
		return nil, nil, diag.WrapAs(diag.ClassProtocol, err, "dial websocket")
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
			status: webSocketSwitchingProtocolsStatus,
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

	conn.SetReadLimit(webSocketReadLimit(wsOpts.MaxMessageBytes, effectiveOpts.WSMaxMessageBytes))

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
		hdr = req.Header.Clone()
	}
	opts := &websocket.DialOptions{
		HTTPHeader:   hdr,
		Subprotocols: slices.Clone(wsOpts.Subprotocols),
		HTTPClient:   client,
	}
	if on, ok := wsOpts.Compression.Get(); ok {
		if on {
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
		return nil, diag.New(diag.ClassProtocol, "websocket session not available")
	}

	session := handle.Session
	listener := session.Subscribe()
	defer listener.Cancel()

	acc := newWSAccumulator()
	lost := listener.Snapshot.Evicted
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
	// The listener's count is only final once it has drained.
	lost += listener.Dropped()
	acc.summary.Dropped += int64(lost)

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
		return nil, diag.WrapAs(diag.ClassProtocol, err, "encode websocket transcript")
	}

	headers := handle.Meta.Headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("Content-Type", streamContentTypeJSON)
	headers.Set(StreamHeaderType, "websocket")
	headers.Set(StreamHeaderSummary, webSocketSummaryLine(transcript.Summary))

	meta := handle.Meta
	if meta.Status == "" {
		meta.Status = webSocketSwitchingProtocolsStatus
	}
	if meta.StatusCode == 0 {
		meta.StatusCode = http.StatusSwitchingProtocols
	}
	if meta.Proto == "" {
		meta.Proto = "HTTP/1.1"
	}
	return streamResp(meta, headers, body, acc.summary.Duration), nil
}

func webSocketSummaryLine(sum WebSocketSummary) string {
	line := fmt.Sprintf(
		"sent=%d recv=%d closed=%s",
		sum.SentCount,
		sum.ReceivedCount,
		sum.ClosedBy,
	)
	if sum.Dropped > 0 {
		line += fmt.Sprintf(" dropped=%d", sum.Dropped)
	}
	return line
}

func buildWebSocketFallback(
	httpResp *http.Response,
	req *restfile.Request,
	started time.Time,
	opts Options,
) (*Response, error) {
	if httpResp == nil {
		return nil, diag.New(diag.ClassProtocol, "websocket handshake response unavailable")
	}

	var body []byte
	if httpResp.Body != nil {
		data, err := opts.readBody(httpResp.Body)
		closeErr := httpResp.Body.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, diag.WrapAs(diag.ClassProtocol, closeErr, "close websocket handshake body")
		}
		body = data
	}

	return respFromHTTP(httpResp.Request, httpResp, req, body, time.Since(started)), nil
}
