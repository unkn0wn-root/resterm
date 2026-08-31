package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

type echoStore struct {
	messages []string
}

func (s *echoStore) add(msg string) {
	s.messages = append(s.messages, msg)
}

func startEchoWebSocketServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	store := &echoStore{}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if err != nil {
				return
			}
			// A hijacked connection outlives Server.Close, so the handler must not touch t.
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "bye")
			}()

			ctx := r.Context()
			for {
				typ, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				switch typ {
				case websocket.MessageText, websocket.MessageBinary:
					store.add(string(data))
					if err := conn.Write(ctx, typ, data); err != nil {
						return
					}
				}
			}
		}),
	)
	srv.Listener = ln
	srv.Start()

	cleanup := func() {
		srv.Close()
	}
	return srv, cleanup
}

func startSilentWebSocketServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if err != nil {
				return
			}
			// A hijacked connection outlives Server.Close, so the handler must not touch t.
			defer func() {
				_ = conn.Close(websocket.StatusNormalClosure, "bye")
			}()
			<-r.Context().Done()
		}),
	)
	srv.Listener = ln
	srv.Start()

	cleanup := func() {
		srv.Close()
	}
	return srv, cleanup
}

func TestExecuteWebSocketChat(t *testing.T) {
	server, cleanup := startEchoWebSocketServer(t)
	defer cleanup()

	wsURL := strings.Replace(server.URL, "http", "ws", 1) + "/ws/chat"
	client := NewClient(nil)

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    wsURL,
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{
				IdleTimeout: 2 * time.Second,
			},
			Steps: []restfile.WebSocketStep{
				{Type: restfile.WebSocketStepSendText, Value: "Hello from resterm!"},
				{Type: restfile.WebSocketStepPong, Value: "client-pong"},
				{Type: restfile.WebSocketStepWait, Duration: 200 * time.Millisecond},
				{Type: restfile.WebSocketStepClose, Code: 1000, Reason: "normal closure"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.ExecuteWebSocket(ctx, req, nil, Options{})
	if err != nil {
		t.Fatalf("ExecuteWebSocket returned error: %v", err)
	}

	if resp == nil {
		t.Fatalf("expected response, got nil")
	}

	if got := resp.Headers.Get("X-Resterm-Stream-Type"); got != "websocket" {
		t.Fatalf("expected websocket stream header, got %q", got)
	}

	var transcript WebSocketTranscript
	if err := json.Unmarshal(resp.Body, &transcript); err != nil {
		t.Fatalf("failed to decode transcript: %v", err)
	}
	if err := transcript.Summary.Err(); err != nil {
		t.Fatalf("scripted close ended as a stream failure: %v", err)
	}
	if transcript.Summary.ClosedBy != wsClosedByClient {
		t.Fatalf("scripted close ended by %q, want %q", transcript.Summary.ClosedBy, wsClosedByClient)
	}

	foundPong := false
	for _, evt := range transcript.Events {
		if evt.Direction == "send" && evt.Type == "pong" && evt.Text == "client-pong" {
			foundPong = true
			break
		}
	}
	if !foundPong {
		t.Fatalf("expected pong event in transcript: %+v", transcript.Events)
	}
}

func TestWebSocketShutdownDoesNotRepeatAStartedClose(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	session := stream.NewSession(ctx, stream.KindWebSocket, stream.Config{})
	session.MarkOpen()
	session.MarkClosing()

	runtime := &wsRuntime{
		session: session,
		writeCh: make(chan wsOutbound),
		cancel:  cancel,
	}
	runtime.closeStarted.Store(true)
	runtime.shutdown()

	state, err := session.State()
	if state != stream.StateClosing || err != nil {
		t.Fatalf("shutdown changed a closing session to state %v with error %v", state, err)
	}
}

// Both sides send a close frame. Only the first one says who ended the session.
func TestAccumulatorKeepsTheFirstCloseFrame(t *testing.T) {
	acc := newWSAccumulator()
	acc.consume(&stream.Event{
		Kind:      stream.KindWebSocket,
		Direction: stream.DirSend,
		Metadata: map[string]string{
			wsMetaType:        "close",
			wsMetaClosedBy:    wsClosedByClient,
			wsMetaCloseCode:   "1000",
			wsMetaCloseReason: "resterm closed",
		},
	})
	acc.consume(&stream.Event{
		Kind:      stream.KindWebSocket,
		Direction: stream.DirReceive,
		Metadata: map[string]string{
			wsMetaType:        "close",
			wsMetaClosedBy:    wsClosedByServer,
			wsMetaCloseCode:   "1001",
			wsMetaCloseReason: "server going away",
		},
	})

	if acc.summary.ClosedBy != wsClosedByClient {
		t.Fatalf("closedBy = %q, want the side that closed first", acc.summary.ClosedBy)
	}
	if acc.summary.CloseCode != 1000 || acc.summary.CloseReason != "resterm closed" {
		t.Fatalf("close = %d %q, want the first frame kept",
			acc.summary.CloseCode, acc.summary.CloseReason)
	}
}

func TestWebSocketAutoCloseIsAttributedToTheClient(t *testing.T) {
	server, cleanup := startEchoWebSocketServer(t)
	defer cleanup()

	req := &restfile.Request{
		Method:    http.MethodGet,
		URL:       strings.Replace(server.URL, "http", "ws", 1) + "/ws",
		WebSocket: &restfile.WebSocketRequest{},
	}
	resp, err := NewClient(nil).ExecuteWebSocket(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("ExecuteWebSocket: %v", err)
	}

	var transcript WebSocketTranscript
	if err := json.Unmarshal(resp.Body, &transcript); err != nil {
		t.Fatalf("decode transcript: %v", err)
	}
	if transcript.Summary.ClosedBy != wsClosedByClient {
		t.Fatalf("closedBy = %q, want %q", transcript.Summary.ClosedBy, wsClosedByClient)
	}
	if err := transcript.Summary.Err(); err != nil {
		t.Fatalf("an auto close ended as a stream failure: %v", err)
	}
}

// The server has to win the close for this to mean anything, so the session is
// never completed. Completing it would send a close of our own first.
func TestWebSocketServerCloseIsAttributedToTheServer(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if err != nil {
				return
			}
			_ = conn.Close(websocket.StatusGoingAway, "server going away")
		}),
	)
	server.Listener = ln
	server.Start()
	defer server.Close()

	req := &restfile.Request{
		Method:    http.MethodGet,
		URL:       strings.Replace(server.URL, "http", "ws", 1) + "/ws",
		WebSocket: &restfile.WebSocketRequest{},
	}
	handle, fallback, err := NewClient(nil).StartWebSocket(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("StartWebSocket: %v", err)
	}
	if fallback != nil {
		t.Fatalf("expected a live websocket handle, got a fallback response")
	}
	t.Cleanup(handle.Session.Cancel)

	select {
	case <-handle.Session.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not end after the server closed it")
	}

	acc := newWSAccumulator()
	for _, evt := range handle.Session.EventsSnapshot() {
		acc.consume(evt)
	}
	state, stateErr := handle.Session.State()
	applyWebSocketSummaryDefaults(&acc.summary, state, stateErr)

	if acc.summary.ClosedBy != wsClosedByServer {
		t.Fatalf("closedBy = %q, want %q", acc.summary.ClosedBy, wsClosedByServer)
	}
	if acc.summary.CloseReason != "server going away" {
		t.Fatalf("closeReason = %q, want the reason the server sent", acc.summary.CloseReason)
	}
}

func TestWebSocketServerCloseDuringOutboundPublication(t *testing.T) {
	closeNow := make(chan struct{}, 1)
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if err != nil {
				return
			}
			<-closeNow
			_ = conn.Close(websocket.StatusGoingAway, "server going away")
		}),
	)
	server.Listener = ln
	server.Start()
	defer server.Close()
	defer func() {
		select {
		case closeNow <- struct{}{}:
		default:
		}
	}()

	conn, _, err := websocket.Dial(
		t.Context(),
		strings.Replace(server.URL, "http", "ws", 1),
		nil,
	)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}

	session := stream.NewSession(t.Context(), stream.KindWebSocket, stream.Config{})
	session.MarkOpen()
	runtime := &wsRuntime{
		conn:    conn,
		session: session,
		writeCh: make(chan wsOutbound),
		cancel:  session.Cancel,
	}

	if !runtime.beginOutbound() {
		t.Fatal("begin outbound publication")
	}
	closeNow <- struct{}{}
	runtime.readLoop()
	runtime.finishOutbound(nil, false)

	state, stateErr := session.State()
	if stateErr != nil {
		t.Fatalf("normal server close became a session failure: %v", stateErr)
	}
	if state != stream.StateClosed {
		t.Fatalf("session state = %v, want %v", state, stream.StateClosed)
	}
}

func TestExecuteWebSocketPingPayloadTranscript(t *testing.T) {
	server, cleanup := startEchoWebSocketServer(t)
	defer cleanup()

	wsURL := strings.Replace(server.URL, "http", "ws", 1) + "/ws/ping"
	client := NewClient(nil)

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    wsURL,
		WebSocket: &restfile.WebSocketRequest{
			Steps: []restfile.WebSocketStep{
				{Type: restfile.WebSocketStepPing, Value: "heartbeat"},
				{Type: restfile.WebSocketStepClose, Code: 1000, Reason: "done"},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.ExecuteWebSocket(ctx, req, nil, Options{})
	if err != nil {
		t.Fatalf("ExecuteWebSocket returned error: %v", err)
	}

	transcript, err := DecodeWebSocketTranscript(resp.Body)
	if err != nil {
		t.Fatalf("failed to decode transcript: %v", err)
	}
	for _, evt := range transcript.Events {
		if evt.Direction == "send" && evt.Type == "ping" && evt.Text == "heartbeat" {
			return
		}
	}
	t.Fatalf("expected ping payload in transcript: %+v", transcript.Events)
}

func TestStartWebSocketUsesHTTPFactory(t *testing.T) {
	called := false
	client := NewClientWithOptions(
		WithHTTPFactory(func(Options) (*http.Client, error) {
			called = true
			return &http.Client{}, nil
		}),
		WithWebSocketDialer(
			func(ctx context.Context, url string, opts *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
				return nil, nil, fmt.Errorf("dial boom")
			},
		),
	)

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.com/ws",
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{},
		},
	}

	_, _, err := client.StartWebSocket(context.Background(), req, nil, Options{})
	if err == nil {
		t.Fatalf("expected dial error")
	}
	if !called {
		t.Fatalf("expected custom HTTP factory to be used")
	}
}

func TestStartWebSocketBindsK8sRequestDiag(t *testing.T) {
	client := NewClientWithOptions(
		WithWebSocketDialer(
			func(ctx context.Context, url string, opts *websocket.DialOptions) (*websocket.Conn, *http.Response, error) {
				if diag := k8s.RequestDiagFromContext(ctx); diag == nil {
					t.Fatal("expected websocket handshake context to include k8s request diag")
				}
				return nil, nil, errors.New("dial boom")
			},
		),
	)

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.com/ws",
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{},
		},
	}
	opts := Options{
		K8s: &k8s.Plan{
			Manager: &k8s.Manager{},
			Config:  &k8s.Config{},
		},
	}

	_, _, err := client.StartWebSocket(context.Background(), req, nil, opts)
	if err == nil {
		t.Fatalf("expected dial error")
	}
}

func TestApplyWebSocketSummaryDefaults(t *testing.T) {
	sum := WebSocketSummary{}
	applyWebSocketSummaryDefaults(&sum, stream.StateFailed, errors.New("boom"))
	if sum.ClosedBy != "error" {
		t.Fatalf("expected closedBy to be error, got %q", sum.ClosedBy)
	}
	if sum.CloseReason != "boom" {
		t.Fatalf("expected close reason to propagate error, got %q", sum.CloseReason)
	}

	sumExisting := WebSocketSummary{ClosedBy: "server"}
	applyWebSocketSummaryDefaults(&sumExisting, stream.StateFailed, errors.New("ignored"))
	if sumExisting.ClosedBy != "server" {
		t.Fatalf("expected existing closedBy to remain, got %q", sumExisting.ClosedBy)
	}

	sumClient := WebSocketSummary{}
	applyWebSocketSummaryDefaults(&sumClient, stream.StateClosed, nil)
	if sumClient.ClosedBy != "client" {
		t.Fatalf("expected default closedBy to client, got %q", sumClient.ClosedBy)
	}

	sumCanceled := WebSocketSummary{}
	applyWebSocketSummaryDefaults(&sumCanceled, stream.StateFailed, context.Canceled)
	if sumCanceled.ClosedBy != wsClosedByCanceled {
		t.Fatalf("closedBy = %q, want a canceled run named as one", sumCanceled.ClosedBy)
	}

	sumTimeout := WebSocketSummary{}
	applyWebSocketSummaryDefaults(&sumTimeout, stream.StateFailed, context.DeadlineExceeded)
	if sumTimeout.ClosedBy != wsClosedByTimeout {
		t.Fatalf("closedBy = %q, want the elapsed duration named as a timeout", sumTimeout.ClosedBy)
	}
}

func TestWebSocketSummaryErr(t *testing.T) {
	tests := []struct {
		name    string
		summary WebSocketSummary
		class   diag.Class
	}{
		{name: "server closed", summary: WebSocketSummary{ClosedBy: wsClosedByServer}},
		{name: "client closed", summary: WebSocketSummary{ClosedBy: wsClosedByClient}},
		{name: "timed out", summary: WebSocketSummary{ClosedBy: wsClosedByTimeout}},
		{
			name:    "read failed",
			summary: WebSocketSummary{ClosedBy: wsClosedByError, CloseReason: "read: boom"},
			class:   diag.ClassProtocol,
		},
		{
			name: "canceled",
			summary: WebSocketSummary{
				ClosedBy:    wsClosedByCanceled,
				CloseReason: "context canceled",
			},
			class: diag.ClassCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.summary.Err()
			if tt.class == "" {
				if err != nil {
					t.Fatalf("Err() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Err() = nil, want a %s failure", tt.class)
			}
			if got := diag.Classes(err); !slices.Contains(got, tt.class) {
				t.Fatalf("Err() classes = %v, want %s", got, tt.class)
			}
			if err.Error() != tt.summary.CloseReason {
				t.Fatalf("Err() = %q, want %q", err, tt.summary.CloseReason)
			}
		})
	}
}

func TestWebSocketIdleTimeout(t *testing.T) {
	server, cleanup := startSilentWebSocketServer(t)
	defer cleanup()

	wsURL := strings.Replace(server.URL, "http", "ws", 1) + "/ws/idle"
	client := NewClient(nil)

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    wsURL,
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{
				IdleTimeout: 150 * time.Millisecond,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	handle, fallback, err := client.StartWebSocket(ctx, req, nil, Options{})
	if err != nil {
		t.Fatalf("StartWebSocket returned error: %v", err)
	}
	if fallback != nil {
		t.Fatalf("expected live websocket handle, got fallback response")
	}

	session := handle.Session
	select {
	case <-session.Done():
	case <-time.After(750 * time.Millisecond):
		t.Fatal("websocket session did not close after idle timeout")
	}

	acc := newWSAccumulator()
	for _, evt := range session.EventsSnapshot() {
		acc.consume(evt)
	}
	state, stateErr := session.State()
	applyWebSocketSummaryDefaults(&acc.summary, state, stateErr)

	if acc.summary.ClosedBy != "timeout" {
		t.Fatalf("expected closedBy to be timeout, got %q", acc.summary.ClosedBy)
	}
	if reason := acc.summary.CloseReason; !strings.Contains(reason, "idle timeout") {
		t.Fatalf("expected idle timeout reason, got %q", reason)
	}
}

func TestStartWebSocketInteractive(t *testing.T) {
	server, cleanup := startEchoWebSocketServer(t)
	defer cleanup()

	wsURL := strings.Replace(server.URL, "http", "ws", 1) + "/ws/chat"
	client := NewClient(nil)

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    wsURL,
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handle, fallback, err := client.StartWebSocket(ctx, req, nil, Options{})
	if err != nil {
		t.Fatalf("StartWebSocket returned error: %v", err)
	}
	if fallback != nil {
		t.Fatalf("expected live session, received fallback response")
	}

	session := handle.Session
	listener := session.Subscribe()
	defer listener.Cancel()

	message := "hello resterm"
	pongPayload := "ack"

	if err := handle.Sender.SendText(
		session.Context(),
		message,
		map[string]string{wsMetaType: "text"},
	); err != nil {
		t.Fatalf("SendText failed: %v", err)
	}

	if err := handle.Sender.Pong(
		session.Context(),
		pongPayload,
		map[string]string{wsMetaStep: "interactive"},
	); err != nil {
		t.Fatalf("Pong failed: %v", err)
	}

	receivedSend := false
	receivedEcho := false
	receivedPong := false

	deadline := time.After(2 * time.Second)

loop:
	for !receivedSend || !receivedEcho || !receivedPong {
		select {
		case evt, ok := <-listener.C:
			if !ok {
				break loop
			}
			if evt.Direction == stream.DirSend && string(evt.Payload) == message {
				receivedSend = true
			}
			if evt.Direction == stream.DirReceive && string(evt.Payload) == message {
				receivedEcho = true
			}
			if evt.Direction == stream.DirSend && evt.Metadata != nil {
				if evt.Metadata[wsMetaType] == "pong" && string(evt.Payload) == pongPayload {
					receivedPong = true
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for websocket events")
		}
	}

	if err := handle.Sender.Close(
		session.Context(),
		websocket.StatusNormalClosure,
		"done",
		map[string]string{wsMetaType: "close"},
	); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case <-session.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session did not terminate after close")
	}

	if !receivedSend || !receivedEcho || !receivedPong {
		t.Fatalf(
			"expected to observe send, receive and pong events, got send=%v receive=%v pong=%v",
			receivedSend,
			receivedEcho,
			receivedPong,
		)
	}
}

func TestStartWebSocketHandshakeFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Denied", "true")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("handshake rejected"))
	}))
	defer srv.Close()

	wsURL := strings.Replace(srv.URL, "http", "ws", 1)
	client := NewClient(nil)

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    wsURL,
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	handle, fallback, err := client.StartWebSocket(ctx, req, nil, Options{})
	if err != nil {
		t.Fatalf("StartWebSocket returned error: %v", err)
	}
	if handle != nil {
		t.Fatalf("expected no handle on handshake failure")
	}
	if fallback == nil {
		t.Fatalf("expected fallback response on handshake failure")
	}
	if fallback.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected status code %d", fallback.StatusCode)
	}
	if string(fallback.Body) != "handshake rejected" {
		t.Fatalf("unexpected fallback body %q", fallback.Body)
	}
	if got := fallback.Headers.Get("X-Denied"); got != "true" {
		t.Fatalf("expected X-Denied header, got %q", got)
	}
}

func TestStartWebSocketHandshakeTimeoutScope(t *testing.T) {
	srv, cleanup := startEchoWebSocketServer(t)
	defer cleanup()

	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/ws/chat"
	client := NewClient(nil)

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    wsURL,
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{
				HandshakeTimeout: 100 * time.Millisecond,
				IdleTimeout:      2 * time.Second,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	handle, fallback, err := client.StartWebSocket(ctx, req, nil, Options{})
	if err != nil {
		t.Fatalf("StartWebSocket returned error: %v", err)
	}
	if fallback != nil {
		t.Fatalf("expected live websocket handle, got fallback response")
	}

	session := handle.Session
	listener := session.Subscribe()
	defer listener.Cancel()

	// Wait longer than the handshake timeout; the session context should remain active.
	select {
	case <-time.After(250 * time.Millisecond):
	case <-session.Done():
		t.Fatal("session terminated before post-handshake timeout elapsed")
	}

	message := "post-timeout ping"
	if err := handle.Sender.SendText(
		session.Context(),
		message,
		map[string]string{wsMetaType: "text"},
	); err != nil {
		t.Fatalf("SendText after handshake timeout failed: %v", err)
	}

	deadline := time.After(time.Second)
	receivedEcho := false

loop:
	for !receivedEcho {
		select {
		case evt, ok := <-listener.C:
			if !ok {
				break loop
			}
			if evt.Direction == stream.DirReceive && string(evt.Payload) == message {
				receivedEcho = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for echo after handshake timeout window")
		}
	}

	if err := handle.Sender.Close(
		session.Context(),
		websocket.StatusNormalClosure,
		"done",
		map[string]string{wsMetaType: "close"},
	); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	select {
	case <-session.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session did not terminate after close")
	}
}

func startClosingWebSocketServer(t *testing.T) *httptest.Server {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if err != nil {
				return
			}
			typ, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if err := conn.Write(r.Context(), typ, data); err != nil {
				return
			}
			_ = conn.Close(websocket.StatusGoingAway, "server going away")
		}),
	)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func wsTranscript(t *testing.T, resp *Response) WebSocketTranscript {
	t.Helper()
	if resp == nil {
		t.Fatal("no response to read a transcript from")
	}
	transcript, err := DecodeWebSocketTranscript(resp.Body)
	if err != nil {
		t.Fatalf("decode transcript: %v", err)
	}
	return *transcript
}

func wsSent(transcript WebSocketTranscript, text string) bool {
	return slices.ContainsFunc(transcript.Events, func(evt WebSocketEvent) bool {
		return evt.Direction == "send" && evt.Text == text
	})
}

func TestCompleteWebSocketKeepsTheTranscriptWhenTheServerClosesMidScript(t *testing.T) {
	server := startClosingWebSocketServer(t)

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    strings.Replace(server.URL, "http", "ws", 1) + "/ws",
		WebSocket: &restfile.WebSocketRequest{
			Steps: []restfile.WebSocketStep{
				{Type: restfile.WebSocketStepSendText, Value: "hello"},
				{Type: restfile.WebSocketStepWait, Duration: 5 * time.Second},
				{Type: restfile.WebSocketStepSendText, Value: "never sent"},
			},
		},
	}

	resp, err := NewClient(nil).ExecuteWebSocket(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("a server closing mid-script failed the request: %v", err)
	}

	transcript := wsTranscript(t, resp)
	if err := transcript.Summary.Err(); err != nil {
		t.Fatalf("a normal server close ended as a stream failure: %v", err)
	}
	if transcript.Summary.ClosedBy != wsClosedByServer {
		t.Fatalf("closedBy = %q, want %q", transcript.Summary.ClosedBy, wsClosedByServer)
	}
	if !wsSent(transcript, "hello") {
		t.Fatalf("the transcript lost the frames sent before the close: %+v", transcript.Events)
	}
}

func TestCompleteWebSocketReportsAStepFailureInTheSummary(t *testing.T) {
	server, cleanup := startEchoWebSocketServer(t)
	defer cleanup()

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    strings.Replace(server.URL, "http", "ws", 1) + "/ws",
		WebSocket: &restfile.WebSocketRequest{
			Steps: []restfile.WebSocketStep{
				{Type: restfile.WebSocketStepSendText, Value: "hello"},
				{Type: restfile.WebSocketStepSendFile, File: "no-such-payload.bin"},
			},
		},
	}

	resp, err := NewClient(nil).ExecuteWebSocket(t.Context(), req, nil, Options{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("a failed step returned before the transcript was built: %v", err)
	}

	transcript := wsTranscript(t, resp)
	if transcript.Summary.ClosedBy != wsClosedByError {
		t.Fatalf("closedBy = %q, want %q", transcript.Summary.ClosedBy, wsClosedByError)
	}
	if !strings.Contains(transcript.Summary.CloseReason, "step 2:send_file") ||
		!strings.Contains(transcript.Summary.CloseReason, "websocket payload file") {
		t.Fatalf("closeReason = %q, want the step and why it failed", transcript.Summary.CloseReason)
	}
	// A payload Resterm could not read is a filesystem failure, not a protocol one.
	if transcript.Summary.ErrorClass != diag.ClassFilesystem {
		t.Fatalf("errorClass = %q, want %q", transcript.Summary.ErrorClass, diag.ClassFilesystem)
	}
	err = transcript.Summary.Err()
	if err == nil {
		t.Fatal("Err() = nil, want the failure to fail the request")
	}
	if !slices.Contains(diag.Classes(err), diag.ClassFilesystem) {
		t.Fatalf("Err() classes = %v, want %s", diag.Classes(err), diag.ClassFilesystem)
	}
	if !wsSent(transcript, "hello") {
		t.Fatalf("the transcript lost the frames sent before the failure: %+v", transcript.Events)
	}
}

func TestCompleteWebSocketNamesACancelledRunAndKeepsTheTranscript(t *testing.T) {
	server, cleanup := startEchoWebSocketServer(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    strings.Replace(server.URL, "http", "ws", 1) + "/ws",
		WebSocket: &restfile.WebSocketRequest{
			Steps: []restfile.WebSocketStep{
				{Type: restfile.WebSocketStepSendText, Value: "hello"},
				{Type: restfile.WebSocketStepWait, Duration: 5 * time.Second},
			},
		},
	}

	time.AfterFunc(300*time.Millisecond, cancel)
	resp, err := NewClient(nil).ExecuteWebSocket(ctx, req, nil, Options{})
	if err != nil {
		t.Fatalf("a cancelled run returned before the transcript was built: %v", err)
	}

	transcript := wsTranscript(t, resp)
	if transcript.Summary.ClosedBy != wsClosedByCanceled {
		t.Fatalf("closedBy = %q, want %q", transcript.Summary.ClosedBy, wsClosedByCanceled)
	}
	if !slices.Contains(diag.Classes(transcript.Summary.Err()), diag.ClassCanceled) {
		t.Fatalf("Err() = %v, want a cancelled run", transcript.Summary.Err())
	}
	if !wsSent(transcript, "hello") {
		t.Fatalf("the transcript lost the frames sent before the cancel: %+v", transcript.Events)
	}
}

func TestWebSocketStepFailureKeepsItsClass(t *testing.T) {
	server, cleanup := startEchoWebSocketServer(t)
	defer cleanup()

	tests := []struct {
		name string
		step restfile.WebSocketStep
		want diag.Class
	}{
		{
			name: "unreadable payload file",
			step: restfile.WebSocketStep{
				Type: restfile.WebSocketStepSendFile,
				File: "no-such-payload.bin",
			},
			want: diag.ClassFilesystem,
		},
		{
			name: "payload that is not base64",
			step: restfile.WebSocketStep{
				Type:  restfile.WebSocketStepSendBase64,
				Value: "not base64 at all",
			},
			want: diag.ClassProtocol,
		},
		{
			name: "payload that is not json",
			step: restfile.WebSocketStep{
				Type:  restfile.WebSocketStepSendJSON,
				Value: "{oops",
			},
			want: diag.ClassProtocol,
		},
		{
			name: "control frame over its payload limit",
			step: restfile.WebSocketStep{
				Type:  restfile.WebSocketStepPing,
				Value: strings.Repeat("A", websocketControlMaxPayload+1),
			},
			want: diag.ClassProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{
				Method: http.MethodGet,
				URL:    strings.Replace(server.URL, "http", "ws", 1) + "/ws",
				WebSocket: &restfile.WebSocketRequest{
					Steps: []restfile.WebSocketStep{tt.step},
				},
			}

			resp, err := NewClient(nil).ExecuteWebSocket(
				t.Context(),
				req,
				nil,
				Options{BaseDir: t.TempDir()},
			)
			if err != nil {
				t.Fatalf("a failed step returned before the transcript was built: %v", err)
			}

			sum := wsTranscript(t, resp).Summary
			if sum.ClosedBy != wsClosedByError {
				t.Fatalf("closedBy = %q, want %q", sum.ClosedBy, wsClosedByError)
			}
			if sum.ErrorClass != tt.want {
				t.Fatalf("errorClass = %q, want %q", sum.ErrorClass, tt.want)
			}
			if got := diag.Classes(sum.Err()); !slices.Contains(got, tt.want) {
				t.Fatalf("Err() classes = %v, want %s", got, tt.want)
			}
		})
	}
}

// A transcript another build wrote can name a class this one does not have.
func TestStreamSummaryErrFallsBackToProtocol(t *testing.T) {
	tests := []struct {
		name  string
		class diag.Class
		want  diag.Class
	}{
		{name: "no class", want: diag.ClassProtocol},
		{name: "unknown class", class: "wormhole", want: diag.ClassProtocol},
		{name: "unclassified", class: diag.ClassUnknown, want: diag.ClassProtocol},
		{name: "a class this build knows", class: diag.ClassNetwork, want: diag.ClassNetwork},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := WebSocketSummary{ClosedBy: wsClosedByError, ErrorClass: tt.class}
			if got := diag.ClassOf(ws.Err()); got != tt.want {
				t.Fatalf("websocket Err() class = %q, want %q", got, tt.want)
			}
			sse := SSESummary{Reason: sseReasonErr, ErrorClass: tt.class}
			if got := diag.ClassOf(sse.Err()); got != tt.want {
				t.Fatalf("sse Err() class = %q, want %q", got, tt.want)
			}
		})
	}
}
