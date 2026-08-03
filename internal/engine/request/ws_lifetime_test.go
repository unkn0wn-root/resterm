package request

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nhooyr.io/websocket"

	"github.com/unkn0wn-root/resterm/internal/engine"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func startWebSocketServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		<-r.Context().Done()
		if err := conn.Close(websocket.StatusNormalClosure, "bye"); err != nil {
			t.Logf("close websocket: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The handover of the session to the console and the cancellation of the request
// race. Cancelling from inside the dialer pins that race to the losing side so
// the outcome is not timing dependent.
func TestInteractiveWebSocketCancelDuringHandoverDoesNotAttach(t *testing.T) {
	srv := startWebSocketServer(t)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	dial := func(
		ctx context.Context,
		url string,
		opts *websocket.DialOptions,
	) (*websocket.Conn, *http.Response, error) {
		conn, resp, err := websocket.Dial(ctx, url, opts)
		if err == nil {
			cancel()
		}
		return conn, resp, err
	}

	eng := New(engine.Config{
		FilePath: "/tmp/example.http",
		Client:   httpx.NewClientWithOptions(httpx.WithWebSocketDialer(dial)),
	}, rtrun.New(rtrun.Config{}))

	req := &restfile.Request{
		Method:    http.MethodGet,
		URL:       strings.Replace(srv.URL, "http", "ws", 1) + "/chat",
		WebSocket: &restfile.WebSocketRequest{},
	}

	attached := 0
	res, err := eng.ExecuteWith(nil, req, testEnv("dev"), ExecOptions{
		Ctx: parent,
		AttachWS: func(*httpx.WebSocketHandle, *restfile.Request) {
			attached++
		},
	})
	if err != nil {
		t.Fatalf("ExecuteWith: %v", err)
	}
	if attached != 0 {
		t.Fatalf("attached %d canceled sessions, want none", attached)
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("result error = %v, want context.Canceled", res.Err)
	}
}

// The whole point of detaching is that the console outlives the request that
// opened it, so the successful path must not follow the request context.
func TestInteractiveWebSocketOutlivesTheRequest(t *testing.T) {
	srv := startWebSocketServer(t)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := New(engine.Config{
		FilePath: "/tmp/example.http",
		Client:   httpx.NewClientWithOptions(),
	}, rtrun.New(rtrun.Config{}))

	req := &restfile.Request{
		Method:    http.MethodGet,
		URL:       strings.Replace(srv.URL, "http", "ws", 1) + "/chat",
		WebSocket: &restfile.WebSocketRequest{},
	}

	var handle *httpx.WebSocketHandle
	res, err := eng.ExecuteWith(nil, req, testEnv("dev"), ExecOptions{
		Ctx: parent,
		AttachWS: func(h *httpx.WebSocketHandle, _ *restfile.Request) {
			handle = h
		},
	})
	if err != nil {
		t.Fatalf("ExecuteWith: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("result error = %v, want none", res.Err)
	}
	if handle == nil {
		t.Fatal("no websocket handle was attached")
	}
	if err := handle.Session.Context().Err(); err != nil {
		t.Fatalf("session context ended with the request: %v", err)
	}

	cancel()
	if err := handle.Session.Context().Err(); err != nil {
		t.Fatalf("session context followed the canceled request: %v", err)
	}
}
