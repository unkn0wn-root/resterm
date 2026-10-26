package httpx

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	gqlAck      = `{"type":"connection_ack"}`
	gqlPing     = `{"type":"ping"}`
	gqlNext     = `{"id":"1","type":"next","payload":{"data":{"n":1}}}`
	gqlComplete = `{"id":"1","type":"complete"}`
)

func gqlServer(
	t *testing.T,
	protocols []string,
	serve func(context.Context, *websocket.Conn),
) (string, <-chan *http.Request) {
	t.Helper()
	seen := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Clone(context.Background())
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: protocols})
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		ctx := r.Context()
		serve(ctx, conn)
		_ = gqlDrain(ctx, conn)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), seen
}

func gqlDrain(ctx context.Context, conn *websocket.Conn) error {
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return err
		}
	}
}

func gqlRead(ctx context.Context, conn *websocket.Conn) string {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return ""
	}
	return string(data)
}

func gqlWrite(ctx context.Context, conn *websocket.Conn, msgs ...string) {
	for _, msg := range msgs {
		if err := conn.Write(ctx, websocket.MessageText, []byte(msg)); err != nil {
			return
		}
	}
}

func gqlOpen(ctx context.Context, conn *websocket.Conn) {
	gqlRead(ctx, conn)
	gqlWrite(ctx, conn, gqlAck)
	gqlRead(ctx, conn)
}

func gqlRequest(url string, ws restfile.WebSocketRequest) *restfile.Request {
	return &restfile.Request{
		Method:    http.MethodGet,
		URL:       url,
		WebSocket: &ws,
		Body:      restfile.BodySource{GraphQL: &restfile.GraphQLBody{Query: "subscription { n }"}},
	}
}

func gqlRun(t *testing.T, req *restfile.Request) WebSocketTranscript {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	resp, err := NewClient(nil).ExecuteWebSocket(ctx, req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("ExecuteWebSocket: %v", err)
	}
	return wsTranscript(t, resp)
}

func gqlFrames(tr WebSocketTranscript) []string {
	var out []string
	for _, evt := range tr.Events {
		frame := evt.Direction + " "
		switch evt.Type {
		case stream.WSText, stream.WSJSON:
			var msg struct{ Type string }
			_ = json.Unmarshal([]byte(evt.Text), &msg)
			frame += cmp.Or(msg.Type, evt.Text)
		case stream.WSClose:
			frame += fmt.Sprintf("close %d %s", evt.Code, evt.Reason)
		default:
			frame += string(evt.Type)
		}
		out = append(out, frame)
	}
	if n := len(out); n >= 2 && strings.HasPrefix(out[n-2], "send close ") &&
		out[n-1] == "receive "+strings.TrimPrefix(out[n-2], "send ") {
		out = out[:n-1]
	}
	return out
}

func TestGraphQLWebSocketRunsTheOperation(t *testing.T) {
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		gqlOpen(ctx, conn)
		gqlWrite(ctx, conn, gqlPing)
		gqlRead(ctx, conn)
		gqlWrite(ctx, conn, gqlNext, gqlNext, gqlNext, gqlComplete)
	})

	tr := gqlRun(t, gqlRequest(url, restfile.WebSocketRequest{}))
	want := []string{
		"send connection_init",
		"receive connection_ack",
		"send subscribe",
		"receive ping",
		"send pong",
		"receive next",
		"receive next",
		"receive next",
		"receive complete",
		"send close 1000 operation complete",
	}
	if got := gqlFrames(tr); !slices.Equal(got, want) {
		t.Fatalf("frames = %q, want %q", got, want)
	}
	for _, evt := range tr.Events {
		if evt.Direction == "send" && evt.Step != "graphql" {
			t.Fatalf("sent %s with step %q, want graphql", evt.Text, evt.Step)
		}
	}
	sum := tr.Summary
	if sum.ClosedBy != stream.WSClosedByClient || sum.CloseCode != 1000 || sum.CloseReason != "operation complete" {
		t.Fatalf(
			"summary = %s %d %q, want client 1000 operation complete",
			sum.ClosedBy,
			sum.CloseCode,
			sum.CloseReason,
		)
	}
	if err := sum.Err(); err != nil {
		t.Fatalf("Err() = %v, want a normal ending", err)
	}
}

func TestGraphQLWebSocketKeepsUpWithAFlood(t *testing.T) {
	procs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(procs) })
	closed := make(chan error, 1)
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		gqlOpen(ctx, conn)
		for range 1000 {
			gqlWrite(ctx, conn, gqlNext)
		}
		gqlWrite(ctx, conn, gqlComplete)
		closed <- gqlDrain(ctx, conn)
	})
	ws := restfile.WebSocketRequest{Options: restfile.WebSocketOptions{IdleTimeout: 2 * time.Second}}
	sum := gqlRun(t, gqlRequest(url, ws)).Summary
	var ce websocket.CloseError
	if err := <-closed; !errors.As(err, &ce) || ce.Code != websocket.StatusNormalClosure ||
		ce.Reason != "operation complete" {
		t.Fatalf(
			"server read %v, summary %s %q, want close 1000 operation complete",
			err,
			sum.ClosedBy,
			sum.CloseReason,
		)
	}
}

func TestGraphQLWebSocketEndings(t *testing.T) {
	for _, tt := range []struct {
		name   string
		serve  func(context.Context, *websocket.Conn)
		frames []string
		by     stream.WSClosedBy
		code   int
		reason string
	}{
		{
			name: "pong before the ack",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, gqlPing)
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, gqlAck)
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, gqlComplete)
			},
			frames: []string{
				"send connection_init",
				"receive ping",
				"send pong",
				"receive connection_ack",
				"send subscribe",
				"receive complete",
				"send close 1000 operation complete",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation complete",
		},
		{
			name: "error ends the operation",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, `{"id":"1","type":"error","payload":[{"message":"no"}]}`)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive error",
				"send close 1000 operation failed",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation failed",
		},
		{
			name: "other ids are ignored",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(
					ctx,
					conn,
					`{"id":"2","type":"next","payload":{}}`,
					`{"id":"2","type":"error","payload":[{"message":"no"}]}`,
					`{"id":"2","type":"complete"}`,
					`{"type":"pong"}`,
					gqlNext,
					gqlComplete,
				)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive next",
				"receive error",
				"receive complete",
				"receive pong",
				"receive next",
				"receive complete",
				"send close 1000 operation complete",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation complete",
		},
		{
			name: "a second ack is ignored",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, gqlAck, gqlComplete)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive connection_ack",
				"receive complete",
				"send close 1000 operation complete",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation complete",
		},
		{
			name: "the server closes after the ack",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				_ = conn.Close(websocket.StatusNormalClosure, "bye")
			},
			frames: []string{"send connection_init", "receive connection_ack", "send subscribe", "receive close 1000 bye"},
			by:     stream.WSClosedByServer,
			code:   1000,
			reason: "bye",
		},
		{
			name: "the server refuses after the ack",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				_ = conn.Close(4403, "Forbidden")
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive close 4403 Forbidden",
			},
			by:     stream.WSClosedByServer,
			code:   4403,
			reason: "Forbidden",
		},
		{
			name: "the server fails after the ack",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				_ = conn.Close(4500, "boom")
			},
			frames: []string{"send connection_init", "receive connection_ack", "send subscribe", "receive close 4500 boom"},
			by:     stream.WSClosedByServer,
			code:   4500,
			reason: "boom",
		},
		{
			name: "an ack with a null payload",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, `{"type":"connection_ack","payload":null}`)
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, gqlComplete)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive complete",
				"send close 1000 operation complete",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation complete",
		},
		{
			name: "an ack with an object payload",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, `{"type":"connection_ack","payload":{"a":1}}`)
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, gqlComplete)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive complete",
				"send close 1000 operation complete",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation complete",
		},
		{
			name: "a ping with an object payload",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, `{"type":"ping","payload":{"n":1}}`)
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, gqlComplete)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive ping",
				"send pong",
				"receive complete",
				"send close 1000 operation complete",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation complete",
		},
		{
			name: "a ping with a null payload",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, `{"type":"ping","payload":null}`)
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, gqlComplete)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive ping",
				"send pong",
				"receive complete",
				"send close 1000 operation complete",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation complete",
		},
		{
			name: "next with an empty object",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, `{"id":"1","type":"next","payload":{}}`, gqlComplete)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive next",
				"receive complete",
				"send close 1000 operation complete",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation complete",
		},
		{
			name: "an error whose message is null",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, `{"id":"1","type":"error","payload":[{"message":null}]}`)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive error",
				"send close 1000 operation failed",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation failed",
		},
		{
			name: "complete with a payload",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, `{"id":"1","type":"complete","payload":42}`)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"receive complete",
				"send close 1000 operation complete",
			},
			by:     stream.WSClosedByClient,
			code:   1000,
			reason: "operation complete",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, _ := gqlServer(t, []string{"graphql-transport-ws"}, tt.serve)
			tr := gqlRun(t, gqlRequest(url, restfile.WebSocketRequest{}))
			if got := gqlFrames(tr); !slices.Equal(got, tt.frames) {
				t.Fatalf("frames = %q, want %q", got, tt.frames)
			}
			sum := tr.Summary
			if sum.ClosedBy != tt.by || sum.CloseCode != tt.code || sum.CloseReason != tt.reason {
				t.Fatalf(
					"summary = %s %d %q, want %s %d %q",
					sum.ClosedBy,
					sum.CloseCode,
					sum.CloseReason,
					tt.by,
					tt.code,
					tt.reason,
				)
			}
			if err := sum.Err(); err != nil {
				t.Fatalf("Err() = %v, want a normal ending", err)
			}
		})
	}
}

func TestGraphQLWebSocketMatchesFieldNamesExactly(t *testing.T) {
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		gqlOpen(ctx, conn)
		gqlWrite(ctx, conn, `{"id":"1","type":"next","payload":{"data":{}},"Type":"complete"}`)
	})
	ws := restfile.WebSocketRequest{Options: restfile.WebSocketOptions{IdleTimeout: 300 * time.Millisecond}}
	sum := gqlRun(t, gqlRequest(url, ws)).Summary
	if sum.ClosedBy != stream.WSClosedByTimeout || sum.ReceivedCount != 2 || sum.Err() != nil {
		t.Fatalf("summary = %+v, want the message read as next and the subscription left open", sum)
	}
}

func TestGraphQLWebSocketFailsOnProtocolViolations(t *testing.T) {
	const (
		binary     = "server sent a binary graphql-transport-ws message"
		notJSON    = "server sent a graphql-transport-ws message that is not valid JSON"
		notObject  = "server sent a graphql-transport-ws message that is not a JSON object"
		noType     = "server sent a graphql-transport-ws message without a type"
		unknown    = "server sent a graphql-transport-ws message of an unknown type"
		ack        = "server sent connection_ack with a payload that is neither an object nor null"
		pong       = "server sent pong with a payload that is neither an object nor null"
		next       = "server sent next without an object payload"
		nextID     = "server sent next without a non-empty string id"
		errs       = "server sent error without a list of GraphQL errors"
		completeID = "server sent complete without a non-empty string id"
		initMsg    = "server sent connection_init, a message only a client sends"
	)
	for _, tt := range []struct {
		name      string
		beforeAck bool
		binary    bool
		msg       string
		want      string
	}{
		{name: "invalid json", beforeAck: true, msg: "hello", want: notJSON},
		{
			name: "nesting deeper than the decoder allows",
			msg:  strings.Repeat("[", 10001) + strings.Repeat("]", 10001),
			want: notJSON,
		},
		{name: "null", msg: "null", want: notObject},
		{name: "not an object", beforeAck: true, msg: "[]", want: notObject},
		{name: "a number", msg: "42", want: notObject},
		{name: "a string", msg: `"next"`, want: notObject},
		{name: "data after the object", msg: `{"type":"pong"} x`, want: notJSON},
		{name: "no type", beforeAck: true, msg: `{}`, want: noType},
		{name: "an empty type", msg: `{"type":""}`, want: noType},
		{name: "a type that is not a string", beforeAck: true, msg: `{"type":5}`, want: noType},
		{name: "a null type", msg: `{"type":null}`, want: noType},
		{name: "field names in another case", msg: `{"TYPE":"complete","ID":"1"}`, want: noType},
		{name: "binary frame", beforeAck: true, binary: true, msg: gqlAck, want: binary},
		{name: "binary next", binary: true, msg: `{"id":"1","type":"next","payload":{}}`, want: binary},
		{name: "unknown type", msg: `{"type":"hello"}`, want: unknown},
		{
			name:      "an ack with a number payload",
			beforeAck: true,
			msg:       `{"type":"connection_ack","payload":42}`,
			want:      ack,
		},
		{
			name:      "an ack with an empty list payload",
			beforeAck: true,
			msg:       `{"type":"connection_ack","payload":[]}`,
			want:      ack,
		},
		{
			name:      "an ack with a list payload",
			beforeAck: true,
			msg:       `{"type":"connection_ack","payload":[1]}`,
			want:      ack,
		},
		{
			name:      "an ack with a string payload",
			beforeAck: true,
			msg:       `{"type":"connection_ack","payload":"x"}`,
			want:      ack,
		},
		{name: "a second ack with a number payload", msg: `{"type":"connection_ack","payload":42}`, want: ack},
		{
			name: "a ping with a number payload",
			msg:  `{"type":"ping","payload":42}`,
			want: "server sent ping with a payload that is neither an object nor null",
		},
		{name: "a pong with a number payload", msg: `{"type":"pong","payload":42}`, want: pong},
		{
			name:      "a pong with a number payload before the ack",
			beforeAck: true,
			msg:       `{"type":"pong","payload":42}`,
			want:      pong,
		},
		{name: "next with a number payload", msg: `{"id":"1","type":"next","payload":42}`, want: next},
		{name: "next with a null payload", msg: `{"id":"1","type":"next","payload":null}`, want: next},
		{name: "next with a list payload", msg: `{"id":"1","type":"next","payload":[]}`, want: next},
		{name: "next without a payload", msg: `{"id":"1","type":"next"}`, want: next},
		{name: "next without an id", msg: `{"type":"next","payload":{}}`, want: nextID},
		{name: "next with an empty id", msg: `{"id":"","type":"next","payload":{}}`, want: nextID},
		{name: "next with a number id", msg: `{"id":1,"type":"next","payload":{}}`, want: nextID},
		{name: "next with a null id", msg: `{"id":null,"type":"next","payload":{}}`, want: nextID},
		{name: "next for another id with a number payload", msg: `{"id":"2","type":"next","payload":42}`, want: next},
		{name: "next before the ack", beforeAck: true, msg: gqlNext, want: "server sent next before connection_ack"},
		{name: "error with an object payload", msg: `{"id":"1","type":"error","payload":{"message":"no"}}`, want: errs},
		{name: "error with an empty list", msg: `{"id":"1","type":"error","payload":[]}`, want: errs},
		{name: "error without a message", msg: `{"id":"1","type":"error","payload":[{}]}`, want: errs},
		{name: "error with a number in the list", msg: `{"id":"1","type":"error","payload":[42]}`, want: errs},
		{name: "error with null in the list", msg: `{"id":"1","type":"error","payload":[null]}`, want: errs},
		{name: "error without a payload", msg: `{"id":"1","type":"error"}`, want: errs},
		{
			name: "error without an id",
			msg:  `{"type":"error","payload":[{"message":"no"}]}`,
			want: "server sent error without a non-empty string id",
		},
		{name: "error for another id with an empty list", msg: `{"id":"2","type":"error","payload":[]}`, want: errs},
		{
			name: "error with a message key in another case",
			msg:  `{"id":"1","type":"error","payload":[{"Message":"no"}]}`,
			want: errs,
		},
		{
			name:      "error before the ack",
			beforeAck: true,
			msg:       `{"id":"1","type":"error","payload":[{"message":"no"}]}`,
			want:      "server sent error before connection_ack",
		},
		{name: "complete without an id", msg: `{"type":"complete"}`, want: completeID},
		{name: "complete with an empty id", msg: `{"id":"","type":"complete"}`, want: completeID},
		{name: "complete with a number id", msg: `{"id":1,"type":"complete"}`, want: completeID},
		{
			name:      "complete before the ack",
			beforeAck: true,
			msg:       gqlComplete,
			want:      "server sent complete before connection_ack",
		},
		{name: "connection_init from the server", msg: `{"type":"connection_init"}`, want: initMsg},
		{name: "connection_init before the ack", beforeAck: true, msg: `{"type":"connection_init"}`, want: initMsg},
		{
			name:      "subscribe from the server",
			beforeAck: true,
			msg:       `{"id":"1","type":"subscribe","payload":{}}`,
			want:      "server sent subscribe, a message only a client sends",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			typ := websocket.MessageText
			if tt.binary {
				typ = websocket.MessageBinary
			}
			closed := make(chan error, 1)
			url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
				if tt.beforeAck {
					gqlRead(ctx, conn)
				} else {
					gqlOpen(ctx, conn)
				}
				_ = conn.Write(ctx, typ, []byte(tt.msg))
				gqlWrite(ctx, conn, gqlNext, gqlComplete)
				closed <- gqlDrain(ctx, conn)
			})
			sum := gqlRun(t, gqlRequest(url, restfile.WebSocketRequest{})).Summary
			if err := <-closed; websocket.CloseStatus(err) != 4400 {
				t.Fatalf("server read %v, want close code 4400", err)
			}
			if sum.ClosedBy != stream.WSClosedByError || sum.ErrorClass != diag.ClassProtocol ||
				sum.CloseCode != 4400 || sum.CloseReason != tt.want {
				t.Fatalf(
					"summary = %s %s %d %q, want error protocol 4400 %q",
					sum.ClosedBy,
					sum.ErrorClass,
					sum.CloseCode,
					sum.CloseReason,
					tt.want,
				)
			}
			if !slices.Contains(diag.Classes(sum.Err()), diag.ClassProtocol) {
				t.Fatalf("Err() = %v, want a protocol failure", sum.Err())
			}
		})
	}
}

func TestGraphQLWebSocketViolationOutlivesTheServerClose(t *testing.T) {
	procs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(procs) })
	for _, tt := range []struct {
		name  string
		idle  time.Duration
		end   func(context.Context, *websocket.Conn)
		codes []int
	}{
		{
			name: "the server closes",
			end: func(_ context.Context, conn *websocket.Conn) {
				_ = conn.Close(websocket.StatusNormalClosure, "peer done")
			},
			codes: []int{4400},
		},
		{
			name:  "the connection drops",
			end:   func(_ context.Context, conn *websocket.Conn) { _ = conn.CloseNow() },
			codes: []int{4400, 0},
		},
		{
			name:  "the server goes quiet",
			idle:  500 * time.Millisecond,
			end:   func(ctx context.Context, _ *websocket.Conn) { <-ctx.Done() },
			codes: []int{4400},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, "not-json")
				tt.end(ctx, conn)
			})
			ws := restfile.WebSocketRequest{Options: restfile.WebSocketOptions{IdleTimeout: tt.idle}}
			sum := gqlRun(t, gqlRequest(url, ws)).Summary
			if sum.ClosedBy != stream.WSClosedByError || sum.ErrorClass != diag.ClassProtocol ||
				!slices.Contains(tt.codes, sum.CloseCode) ||
				sum.CloseReason != "server sent a graphql-transport-ws message that is not valid JSON" {
				t.Fatalf("summary = %s %s %d %q, want the violation and close code %v",
					sum.ClosedBy, sum.ErrorClass, sum.CloseCode, sum.CloseReason, tt.codes)
			}
		})
	}
}

func TestGraphQLWebSocketViolationOutlivesRestermsClose(t *testing.T) {
	release := make(chan struct{})
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		gqlOpen(ctx, conn)
		<-release
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	steps := []restfile.WebSocketStep{{Type: restfile.WebSocketStepWait, Duration: time.Hour}}
	req := gqlRequest(url, restfile.WebSocketRequest{Steps: steps})
	client := NewClient(nil)
	handle, _, err := client.StartWebSocket(ctx, req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("StartWebSocket: %v", err)
	}
	result := make(chan *Response, 1)
	go func() {
		resp, _ := client.CompleteWebSocket(ctx, handle, req, Options{})
		result <- resp
	}()
	<-handle.graphql.subscribed
	rt := handle.Sender.runtime
	go func() { _ = handle.Sender.Close(ctx, stream.WSCloseNormal, "done", "1:close") }()
	for !rt.closeStarted.Load() {
		runtime.Gosched()
	}
	bad := diag.New(diag.ClassProtocol, "server sent a graphql-transport-ws message that is not a JSON object")
	rt.receive(stream.WSText, []byte("not-json"), bad)
	close(release)
	sum := wsTranscript(t, <-result).Summary
	if sum.ClosedBy != stream.WSClosedByError || sum.ErrorClass != diag.ClassProtocol ||
		sum.CloseReason != bad.Error() {
		t.Fatalf("summary = %s %s %q, want the violation", sum.ClosedBy, sum.ErrorClass, sum.CloseReason)
	}
}

func TestGraphQLWebSocketCompleteBeforeTheServerClose(t *testing.T) {
	procs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(procs) })
	for _, tt := range []struct {
		name   string
		msg    string
		frame  string
		reason string
	}{
		{name: "complete", msg: gqlComplete, frame: "receive complete", reason: "operation complete"},
		{
			name:   "error",
			msg:    `{"id":"1","type":"error","payload":[{"message":"no"}]}`,
			frame:  "receive error",
			reason: "operation failed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, tt.msg, gqlNext)
				_ = conn.Close(websocket.StatusNormalClosure, "peer done")
			})
			tr := gqlRun(t, gqlRequest(url, restfile.WebSocketRequest{}))
			want := []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				tt.frame,
				"send close 1000 " + tt.reason,
				"receive next",
				"receive close 1000 peer done",
			}
			if got := gqlFrames(tr); !slices.Equal(got, want) {
				t.Fatalf("frames = %q, want %q", got, want)
			}
			sum := tr.Summary
			if sum.ClosedBy != stream.WSClosedByClient || sum.CloseCode != 1000 || sum.CloseReason != tt.reason {
				t.Fatalf(
					"summary = %s %d %q, want client 1000 %q",
					sum.ClosedBy,
					sum.CloseCode,
					sum.CloseReason,
					tt.reason,
				)
			}
		})
	}
}

func TestGraphQLWebSocketFailsBeforeTheAck(t *testing.T) {
	for _, tt := range []struct {
		name   string
		code   websocket.StatusCode
		reason string
		idle   time.Duration
		class  diag.Class
		want   string
	}{
		{
			name:   "the server closes normally",
			code:   websocket.StatusNormalClosure,
			reason: "bye",
			class:  diag.ClassProtocol,
			want:   `server closed the socket with 1000 "bye" before connection_ack`,
		},
		{
			name:   "the server refuses the connection",
			code:   4403,
			reason: "Forbidden",
			class:  diag.ClassProtocol,
			want:   `server closed the socket with 4403 "Forbidden" before connection_ack`,
		},
		{
			name:   "the server fails",
			code:   4500,
			reason: "boom",
			class:  diag.ClassProtocol,
			want:   `server closed the socket with 4500 "boom" before connection_ack`,
		},
		{
			name:  "idle passes",
			idle:  300 * time.Millisecond,
			class: diag.ClassTimeout,
			want:  "server sent no connection_ack before the idle timeout",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
				gqlRead(ctx, conn)
				if tt.code != 0 {
					_ = conn.Close(tt.code, tt.reason)
				}
			})
			ws := restfile.WebSocketRequest{Options: restfile.WebSocketOptions{IdleTimeout: tt.idle}}
			sum := gqlRun(t, gqlRequest(url, ws)).Summary
			if sum.ClosedBy != stream.WSClosedByError || sum.ErrorClass != tt.class ||
				sum.CloseCode != int(tt.code) || sum.CloseReason != tt.want {
				t.Fatalf(
					"summary = %s %s %d %q, want error %s %d %q",
					sum.ClosedBy,
					sum.ErrorClass,
					sum.CloseCode,
					sum.CloseReason,
					tt.class,
					tt.code,
					tt.want,
				)
			}
			if !slices.Contains(diag.Classes(sum.Err()), tt.class) {
				t.Fatalf("Err() = %v, want a %s failure", sum.Err(), tt.class)
			}
		})
	}
}

func TestGraphQLWebSocketStepsRunAfterSubscribe(t *testing.T) {
	steps := []restfile.WebSocketStep{{Type: restfile.WebSocketStepSendText, Value: "step"}}
	for _, tt := range []struct {
		name   string
		serve  func(context.Context, *websocket.Conn)
		frames []string
	}{
		{
			name: "the operation ends first",
			serve: func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlRead(ctx, conn)
				gqlWrite(ctx, conn, gqlComplete)
			},
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"send step",
				"receive complete",
				"send close 1000 operation complete",
			},
		},
		{
			name:  "the steps end first",
			serve: gqlOpen,
			frames: []string{
				"send connection_init",
				"receive connection_ack",
				"send subscribe",
				"send step",
				"send close 1000 resterm closed",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, _ := gqlServer(t, []string{"graphql-transport-ws"}, tt.serve)
			tr := gqlRun(t, gqlRequest(url, restfile.WebSocketRequest{Steps: steps}))
			if got := gqlFrames(tr); !slices.Equal(got, tt.frames) {
				t.Fatalf("frames = %q, want %q", got, tt.frames)
			}
			if tr.Summary.ClosedBy != stream.WSClosedByClient {
				t.Fatalf("closedBy = %s, want client", tr.Summary.ClosedBy)
			}
		})
	}
}

func TestGraphQLWebSocketWithoutStepsWaitsForTheOperation(t *testing.T) {
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		gqlOpen(ctx, conn)
		time.Sleep(300 * time.Millisecond)
		gqlWrite(ctx, conn, gqlNext, gqlComplete)
	})
	tr := gqlRun(t, gqlRequest(url, restfile.WebSocketRequest{}))
	if tr.Summary.CloseReason != "operation complete" || tr.Summary.ReceivedCount != 4 {
		t.Fatalf("summary = %+v, want the operation to complete after its next", tr.Summary)
	}
}

func TestGraphQLWebSocketIdleEndsAQuietSubscription(t *testing.T) {
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, gqlOpen)
	ws := restfile.WebSocketRequest{Options: restfile.WebSocketOptions{IdleTimeout: 300 * time.Millisecond}}
	sum := gqlRun(t, gqlRequest(url, ws)).Summary
	if sum.ClosedBy != stream.WSClosedByTimeout || sum.Err() != nil {
		t.Fatalf("summary = %s err %v, want a normal idle timeout", sum.ClosedBy, sum.Err())
	}
}

func TestGraphQLWebSocketSubprotocol(t *testing.T) {
	complete := func(ctx context.Context, conn *websocket.Conn) {
		gqlOpen(ctx, conn)
		gqlWrite(ctx, conn, gqlComplete)
	}
	for _, tt := range []struct {
		name    string
		gql     bool
		offer   []string
		accept  []string
		offered string
		err     string
	}{
		{name: "offered by default", gql: true, accept: []string{"graphql-transport-ws"}, offered: "graphql-transport-ws"},
		{
			name:    "named explicitly",
			gql:     true,
			offer:   []string{"graphql-transport-ws"},
			accept:  []string{"graphql-transport-ws"},
			offered: "graphql-transport-ws",
		},
		{
			name:    "server accepts none",
			gql:     true,
			offered: "graphql-transport-ws",
			err:     "the server did not accept the graphql-transport-ws subprotocol",
		},
		{name: "plain websocket offers none", accept: []string{"graphql-transport-ws"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, seen := gqlServer(t, tt.accept, complete)
			req := gqlRequest(
				url,
				restfile.WebSocketRequest{Options: restfile.WebSocketOptions{Subprotocols: tt.offer}},
			)
			if !tt.gql {
				req.Body.GraphQL = nil
			}
			handle, _, err := NewClient(nil).StartWebSocket(t.Context(), req, vars.NewResolver(), Options{})
			if tt.err != "" {
				if err == nil || err.Error() != tt.err {
					t.Fatalf("err = %v, want %q", err, tt.err)
				}
			} else {
				if err != nil {
					t.Fatalf("StartWebSocket: %v", err)
				}
				handle.Session.Cancel()
				<-handle.Session.Done()
			}
			var offered string
			select {
			case r := <-seen:
				offered = r.Header.Get("Sec-WebSocket-Protocol")
			default:
				if tt.offered != "" {
					t.Fatal("the handshake never reached the server")
				}
			}
			if offered != tt.offered {
				t.Fatalf("offered %q, want %q", offered, tt.offered)
			}
		})
	}
}

func TestGraphQLWebSocketKeepsTheOperationOutOfTheHandshakeRequest(t *testing.T) {
	gql := &restfile.GraphQLBody{Query: "subscription { n }", Variables: `{"a": 1}`, OperationName: "N"}
	for _, tt := range []struct {
		name   string
		method string
		url    string
		ws     bool
		target string
		body   string
		ct     string
	}{
		{
			name:   "websocket get",
			method: http.MethodGet,
			url:    "ws://example.com/graphql?x=1",
			ws:     true,
			target: "ws://example.com/graphql?x=1",
		},
		{
			name:   "websocket post",
			method: http.MethodPost,
			url:    "ws://example.com/graphql?x=1",
			ws:     true,
			target: "ws://example.com/graphql?x=1",
		},
		{
			name:   "http get",
			method: http.MethodGet,
			url:    "https://example.com/graphql?x=1",
			target: "https://example.com/graphql?operationName=N&query=subscription+%7B+n+%7D&variables=%7B%22a%22%3A1%7D&x=1",
		},
		{
			name:   "http post",
			method: http.MethodPost,
			url:    "https://example.com/graphql?x=1",
			target: "https://example.com/graphql?x=1",
			body:   `{"operationName":"N","query":"subscription { n }","variables":{"a":1}}`,
			ct:     "application/json",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{Method: tt.method, URL: tt.url, Body: restfile.BodySource{GraphQL: gql}}
			if tt.ws {
				req.WebSocket = &restfile.WebSocketRequest{}
			}
			httpReq, _, body, err := NewClient(nil).BuildHTTPRequest(t.Context(), req, vars.NewResolver(), Options{})
			if err != nil {
				t.Fatalf("BuildHTTPRequest: %v", err)
			}
			got := fmt.Sprintf("%s body=%s ct=%q", httpReq.URL, body, httpReq.Header.Get("Content-Type"))
			if want := fmt.Sprintf("%s body=%s ct=%q", tt.target, tt.body, tt.ct); got != want {
				t.Fatalf("request = %s, want %s", got, want)
			}
		})
	}
}

func TestGraphQLWebSocketDialsTheURLAsWritten(t *testing.T) {
	url, seen := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		gqlOpen(ctx, conn)
		gqlWrite(ctx, conn, gqlComplete)
	})
	req := gqlRequest(url+"/graphql?x=1", restfile.WebSocketRequest{})
	gqlRun(t, req)
	if r := <-seen; r.URL.RequestURI() != "/graphql?x=1" || req.URL != url+"/graphql?x=1" {
		t.Fatalf("handshake = %s, request URL = %s, want the URL as written", r.URL.RequestURI(), req.URL)
	}
}

func TestGraphQLWebSocketSubscribesWithThePOSTBody(t *testing.T) {
	files := mapFS{
		"q.graphql": []byte("subscription Count($from: Int!) { count(from: $from) }\n"),
		"v.json":    []byte(`{"from": {{from}}, "tag": "<x>"}`),
	}
	env := vars.NewMapProvider("env", map[string]string{"from": "3", "op": "Count"})
	gql := restfile.GraphQLBody{QueryFile: "q.graphql", VariablesFile: "v.json", OperationName: "{{op}}"}

	post := &restfile.Request{
		Method: http.MethodPost,
		URL:    "http://example.com",
		Body:   restfile.BodySource{GraphQL: &gql},
	}
	_, _, body, err := NewClient(files).BuildHTTPRequest(t.Context(), post, vars.NewResolver(env), Options{})
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}

	subscribe := make(chan string, 1)
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		gqlRead(ctx, conn)
		gqlWrite(ctx, conn, gqlAck)
		subscribe <- gqlRead(ctx, conn)
		gqlWrite(ctx, conn, gqlComplete)
	})
	req := gqlRequest(url, restfile.WebSocketRequest{})
	req.Body.GraphQL = &gql
	resp, err := NewClient(files).ExecuteWebSocket(t.Context(), req, vars.NewResolver(env), Options{})
	if err != nil {
		t.Fatalf("ExecuteWebSocket: %v", err)
	}
	if sum := wsTranscript(t, resp).Summary; sum.CloseReason != "operation complete" {
		t.Fatalf("summary = %+v, want a completed operation", sum)
	}
	var msg struct {
		ID      string
		Type    string
		Payload json.RawMessage
	}
	if err := json.Unmarshal([]byte(<-subscribe), &msg); err != nil {
		t.Fatalf("decode subscribe: %v", err)
	}
	if msg.ID != "1" || msg.Type != "subscribe" || string(msg.Payload) != string(body) {
		t.Fatalf("subscribe = %s %s %s, want 1 subscribe %s", msg.ID, msg.Type, msg.Payload, body)
	}
}

func TestGraphQLWebSocketExpansionFailsBeforeDialing(t *testing.T) {
	url, seen := gqlServer(t, []string{"graphql-transport-ws"}, gqlOpen)
	req := gqlRequest(url, restfile.WebSocketRequest{})
	req.Body.GraphQL.Query = "subscription { {{nope}} }"
	_, _, err := NewClient(nil).StartWebSocket(t.Context(), req, vars.NewResolver(), Options{})
	if err == nil || err.Error() != "expand graphql query: undefined variable: nope" {
		t.Fatalf("err = %v, want the expansion error", err)
	}
	select {
	case <-seen:
		t.Fatal("dialed before the operation was built")
	default:
	}
}

func TestGraphQLWebSocketEndsAfterAFailedAutoClose(t *testing.T) {
	release := make(chan struct{})
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		conn.SetReadLimit(-1)
		gqlOpen(ctx, conn)
		<-release
		gqlWrite(ctx, conn, gqlComplete)
		gqlRead(ctx, conn)
		_ = conn.Close(4001, "refused")
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	steps := []restfile.WebSocketStep{{Type: restfile.WebSocketStepWait, Duration: time.Hour}}
	req := gqlRequest(url, restfile.WebSocketRequest{Steps: steps})
	client := NewClient(nil)
	handle, _, err := client.StartWebSocket(ctx, req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("StartWebSocket: %v", err)
	}
	result := make(chan *Response, 1)
	go func() {
		resp, _ := client.CompleteWebSocket(ctx, handle, req, Options{})
		result <- resp
	}()
	<-handle.graphql.subscribed
	rt := handle.Sender.runtime
	go func() { _ = handle.Sender.SendBinary(ctx, make([]byte, 16<<20), "big") }()
	for {
		rt.mu.Lock()
		out := rt.out
		rt.mu.Unlock()
		if out > 0 {
			break
		}
		runtime.Gosched()
	}
	go func() { _ = handle.Sender.Close(ctx, stream.WSCloseNormal, "resterm closed", "auto-close") }()
	for !rt.closeStarted.Load() {
		runtime.Gosched()
	}
	close(release)
	sum := wsTranscript(t, <-result).Summary
	if ctx.Err() != nil || sum.Err() != nil {
		t.Fatalf("summary = %+v, ctx %v, want the run to end by itself without an error", sum, ctx.Err())
	}
}

func TestGraphQLWebSocketClosedByTheUserBeforeTheAck(t *testing.T) {
	sent := make(chan struct{})
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		gqlRead(ctx, conn)
		close(sent)
	})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	req := gqlRequest(url, restfile.WebSocketRequest{})
	client := NewClient(nil)
	handle, _, err := client.StartWebSocket(ctx, req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("StartWebSocket: %v", err)
	}
	result := make(chan *Response, 1)
	go func() {
		resp, _ := client.CompleteWebSocket(ctx, handle, req, Options{})
		result <- resp
	}()
	<-sent
	_ = handle.Sender.Close(ctx, stream.WSCloseNormal, "interactive close", "console")
	sum := wsTranscript(t, <-result).Summary
	if sum.ClosedBy != stream.WSClosedByClient || sum.Err() != nil {
		t.Fatalf("summary = %s %q, want the user's close without a failure", sum.ClosedBy, sum.CloseReason)
	}
}

func TestGraphQLWebSocketCompletesWhenThePeerDrops(t *testing.T) {
	url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
		gqlOpen(ctx, conn)
		gqlWrite(ctx, conn, gqlComplete)
		_ = conn.CloseNow()
	})
	sum := gqlRun(t, gqlRequest(url, restfile.WebSocketRequest{})).Summary
	if sum.ClosedBy != stream.WSClosedByClient || sum.Err() != nil {
		t.Fatalf("summary = %s %q, want the completed operation without a failure", sum.ClosedBy, sum.CloseReason)
	}
}

func TestGraphQLWebSocketViolationInTheTranscriptFails(t *testing.T) {
	bad := `{"id":"1","type":"next","payload":42,"pad":"` + strings.Repeat("a", 3<<20) + `"}`
	for _, tt := range []struct {
		name string
		end  func(context.Context, *WebSocketHandle)
	}{
		{
			name: "resterm closes",
			end: func(ctx context.Context, h *WebSocketHandle) {
				_ = h.Sender.Close(ctx, stream.WSCloseNormal, "resterm closed", "auto-close")
			},
		},
		{
			name: "idle passes",
			end: func(_ context.Context, h *WebSocketHandle) {
				evt := &stream.Event{
					Kind:      stream.KindWebSocket,
					Direction: stream.DirNA,
					Timestamp: time.Now(),
					WS:        stream.WSMetadata{ClosedBy: stream.WSClosedByTimeout, Reason: "idle timeout after 1ms"},
				}
				h.Sender.runtime.stop(evt, nil)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
				gqlOpen(ctx, conn)
				gqlWrite(ctx, conn, bad)
			})
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			req := gqlRequest(url, restfile.WebSocketRequest{
				Steps:   []restfile.WebSocketStep{{Type: restfile.WebSocketStepWait, Duration: time.Hour}},
				Options: restfile.WebSocketOptions{MaxMessageBytes: 16 << 20},
			})
			client := NewClient(nil)
			handle, _, err := client.StartWebSocket(ctx, req, vars.NewResolver(), Options{})
			if err != nil {
				t.Fatalf("StartWebSocket: %v", err)
			}
			listener := handle.Session.Subscribe()
			defer listener.Cancel()
			go func() {
				for evt := range listener.C {
					if len(evt.Payload) > 1<<20 {
						tt.end(ctx, handle)
						return
					}
				}
			}()
			resp, err := client.CompleteWebSocket(ctx, handle, req, Options{})
			if err != nil {
				t.Fatalf("CompleteWebSocket: %v", err)
			}
			tr := wsTranscript(t, resp)
			recorded := slices.ContainsFunc(tr.Events, func(e WebSocketEvent) bool { return e.Size > 1<<20 })
			sum := tr.Summary
			if !recorded || sum.ClosedBy != stream.WSClosedByError || sum.ErrorClass != diag.ClassProtocol ||
				sum.CloseReason != "server sent next without an object payload" {
				t.Fatalf(
					"recorded %v, summary = %s %s %q, want the recorded violation",
					recorded,
					sum.ClosedBy,
					sum.ErrorClass,
					sum.CloseReason,
				)
			}
		})
	}
}

func TestGraphQLWebSocketGivesUpOnAStuckClose(t *testing.T) {
	flood := `{"id":"1","type":"next","payload":{"data":{"v":"` + strings.Repeat("x", 16000) + `"}}}`
	for _, tt := range []struct {
		name string
		msg  string
		by   stream.WSClosedBy
	}{
		{name: "a violation", msg: "not-json", by: stream.WSClosedByError},
		{name: "complete", msg: gqlComplete, by: stream.WSClosedByClient},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
				conn.SetReadLimit(-1)
				gqlOpen(ctx, conn)
				_, r, err := conn.Reader(ctx)
				if err != nil {
					return
				}
				gqlWrite(ctx, conn, tt.msg)
				for range 4096 {
					if conn.Write(ctx, websocket.MessageText, []byte(flood)) != nil {
						return
					}
				}
				_, _ = io.Copy(io.Discard, r)
			})
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			steps := []restfile.WebSocketStep{
				{Type: restfile.WebSocketStepSendText, Value: strings.Repeat("y", 32<<20)},
			}
			req := gqlRequest(url, restfile.WebSocketRequest{Steps: steps})
			resp, err := NewClient(nil).ExecuteWebSocket(ctx, req, vars.NewResolver(), Options{})
			if err != nil {
				t.Fatalf("ExecuteWebSocket: %v", err)
			}
			sum := wsTranscript(t, resp).Summary
			if ctx.Err() != nil || sum.ClosedBy != tt.by {
				t.Fatalf("summary = %+v, ctx %v, want %s before the deadline", sum, ctx.Err(), tt.by)
			}
		})
	}
}

func gqlRawOpen(p *rawPeer) {
	p.read()
	p.write(0x1, []byte(gqlAck))
	p.read()
}

func gqlBadNext(size int) string {
	return `{"id":"1","type":"next","payload":42,"pad":"` + strings.Repeat("a", size) + `"}`
}

func TestGraphQLWebSocketOutlivesAPeerThatDoesNotAnswer(t *testing.T) {
	for _, tt := range []struct {
		name   string
		answer func(*rawPeer)
	}{
		{name: "the peer drops the connection", answer: func(*rawPeer) {}},
		{name: "the peer never answers", answer: func(p *rawPeer) { p.read() }},
		{name: "the peer answers another code", answer: func(p *rawPeer) { p.close(1001, "away"); p.read() }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			url := startRawPeer(t, "graphql-transport-ws", func(p *rawPeer) {
				gqlRawOpen(p)
				p.write(0x1, []byte(gqlComplete))
				p.readClose()
				tt.answer(p)
			})
			sum := gqlRun(t, gqlRequest(url, restfile.WebSocketRequest{})).Summary
			if sum.ClosedBy != stream.WSClosedByClient || sum.CloseCode != 1000 ||
				sum.CloseReason != "operation complete" || sum.Err() != nil {
				t.Fatalf("summary = %+v, want the completed operation and resterm's close 1000", sum)
			}
		})
	}
}

func TestGraphQLWebSocketCountsAFrameThatArrivesDuringItsClose(t *testing.T) {
	url := startRawPeer(t, "graphql-transport-ws", func(p *rawPeer) {
		gqlRawOpen(p)
		p.readClose()
		p.write(0x1, []byte(gqlBadNext(4<<20)))
		p.close(1000, "")
		p.read()
	})
	ws := restfile.WebSocketRequest{
		Steps:   []restfile.WebSocketStep{{Type: restfile.WebSocketStepClose, Code: 1000, Reason: "done"}},
		Options: restfile.WebSocketOptions{MaxMessageBytes: 8 << 20},
	}
	tr := gqlRun(t, gqlRequest(url, ws))
	recorded := slices.ContainsFunc(tr.Events, func(e WebSocketEvent) bool { return e.Size > 4<<20 })
	sum := tr.Summary
	if !recorded || sum.ClosedBy != stream.WSClosedByError || sum.ErrorClass != diag.ClassProtocol ||
		sum.CloseCode != 1000 || sum.CloseReason != "server sent next without an object payload" {
		t.Fatalf("frame recorded %v, summary = %+v, want the violation after resterm's close 1000", recorded, sum)
	}
}

func TestGraphQLWebSocketCountsAFrameReadBeforeRestermsClose(t *testing.T) {
	for _, size := range []int{1 << 10, 30 << 10, 3 << 20} {
		t.Run(fmt.Sprintf("%d bytes", size), func(t *testing.T) {
			release := make(chan struct{})
			url := startRawPeer(t, "graphql-transport-ws", func(p *rawPeer) {
				gqlRawOpen(p)
				<-release
				p.write(0x1, []byte(gqlBadNext(size)))
				p.readClose()
				p.close(1000, "")
				p.read()
			})
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			req := gqlRequest(
				url,
				restfile.WebSocketRequest{Options: restfile.WebSocketOptions{MaxMessageBytes: 8 << 20}},
			)
			client := NewClient(nil)
			handle, _, err := client.StartWebSocket(ctx, req, vars.NewResolver(), Options{})
			if err != nil {
				t.Fatalf("StartWebSocket: %v", err)
			}
			result := make(chan *Response, 1)
			go func() {
				resp, _ := client.CompleteWebSocket(ctx, handle, req, Options{})
				result <- resp
			}()
			<-handle.graphql.subscribed
			rt := handle.Sender.runtime
			select {
			case <-rt.pulse:
			default:
			}
			close(release)
			for len(rt.pulse) == 0 {
				runtime.Gosched()
			}
			_ = handle.Sender.Close(ctx, stream.WSCloseNormal, "done", "test")
			tr := wsTranscript(t, <-result)
			recorded := slices.ContainsFunc(tr.Events, func(e WebSocketEvent) bool { return e.Size > size })
			sum := tr.Summary
			if !recorded || sum.ClosedBy != stream.WSClosedByError || sum.ErrorClass != diag.ClassProtocol ||
				sum.CloseCode != 1000 && sum.CloseCode != 4400 {
				t.Fatalf("frame recorded %v, summary = %+v, want the violation and close 1000 or 4400", recorded, sum)
			}
		})
	}
}

func TestGraphQLWebSocketIdleAgreesWithTheAckItRead(t *testing.T) {
	const idle = 50 * time.Millisecond
	for round := range 3 {
		for d := 40 * time.Millisecond; d <= 60*time.Millisecond; d += time.Millisecond {
			url, _ := gqlServer(t, []string{"graphql-transport-ws"}, func(ctx context.Context, conn *websocket.Conn) {
				gqlRead(ctx, conn)
				time.Sleep(d)
				gqlWrite(ctx, conn, gqlAck)
			})
			ws := restfile.WebSocketRequest{Options: restfile.WebSocketOptions{IdleTimeout: idle}}
			tr := gqlRun(t, gqlRequest(url, ws))
			acked := slices.Contains(gqlFrames(tr), "receive connection_ack")
			noAck := tr.Summary.CloseReason == "server sent no connection_ack before the idle timeout"
			if acked == noAck {
				t.Fatalf("round %d, ack after %s: ack in the transcript %v, no-ack failure %v", round, d, acked, noAck)
			}
		}
	}
}

func TestGraphQLWebSocketFailsWhenTheListenerFallsBehind(t *testing.T) {
	url := startRawPeer(t, "graphql-transport-ws", func(p *rawPeer) {
		gqlRawOpen(p)
		p.readClose()
		for range 5000 {
			p.write(0x1, []byte(gqlNext))
		}
		p.write(0x1, []byte("not-json"))
		p.close(1000, "")
		p.read()
	})
	ws := restfile.WebSocketRequest{
		Steps: []restfile.WebSocketStep{{Type: restfile.WebSocketStepClose, Code: 1000, Reason: "done"}},
	}
	sum := gqlRun(t, gqlRequest(url, ws)).Summary
	if sum.ClosedBy != stream.WSClosedByError || sum.ErrorClass != diag.ClassProtocol {
		t.Fatalf("summary = %+v, want the violation after 5000 messages", sum)
	}
}

func TestGraphQLWebSocketWaitsForTheAnswerFromWhenItsCloseIsSent(t *testing.T) {
	sent := make(chan time.Time, 1)
	url := startRawPeer(t, "graphql-transport-ws", func(p *rawPeer) {
		gqlRawOpen(p)
		_, _ = p.r.Peek(1)
		p.write(0x1, []byte(gqlComplete))
		completed := time.Now()
		sent <- completed
		time.Sleep(time.Second)
		p.read()
		p.readClose()
		time.Sleep(time.Until(completed.Add(5500 * time.Millisecond)))
		p.close(1000, "late")
		p.read()
	})
	steps := []restfile.WebSocketStep{{Type: restfile.WebSocketStepSendText, Value: strings.Repeat("y", 32<<20)}}
	tr := gqlRun(t, gqlRequest(url, restfile.WebSocketRequest{Steps: steps}))
	took := time.Since(<-sent)
	answered := slices.ContainsFunc(tr.Events, func(e WebSocketEvent) bool {
		return e.Direction == "receive" && e.Type == stream.WSClose && e.Reason == "late"
	})
	sum := tr.Summary
	if !answered || sum.ClosedBy != stream.WSClosedByClient || sum.CloseCode != 1000 || sum.Err() != nil ||
		took < 5*time.Second || took > 6*time.Second {
		t.Fatalf(
			"answer recorded %v, summary = %+v after %s, want the late answer after about 5.5s",
			answered,
			sum,
			took,
		)
	}
}

func TestGraphQLWebSocketCancelAndDeadlineDuringItsClose(t *testing.T) {
	for _, tt := range []struct {
		name  string
		start func(context.Context, <-chan struct{}) (context.Context, context.CancelFunc)
		by    stream.WSClosedBy
		class diag.Class
	}{
		{
			name: "cancel",
			start: func(ctx context.Context, closing <-chan struct{}) (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(ctx)
				go func() {
					<-closing
					cancel()
				}()
				return ctx, cancel
			},
			by:    stream.WSClosedByCanceled,
			class: diag.ClassCanceled,
		},
		{
			name: "deadline",
			start: func(ctx context.Context, _ <-chan struct{}) (context.Context, context.CancelFunc) {
				return context.WithTimeout(ctx, time.Second)
			},
			by:    stream.WSClosedByTimeout,
			class: diag.ClassTimeout,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			closing := make(chan struct{})
			url := startRawPeer(t, "graphql-transport-ws", func(p *rawPeer) {
				gqlRawOpen(p)
				p.write(0x1, []byte(gqlComplete))
				p.readClose()
				close(closing)
				p.read()
			})
			ctx, cancel := tt.start(t.Context(), closing)
			defer cancel()
			resp, err := NewClient(
				nil,
			).ExecuteWebSocket(ctx, gqlRequest(url, restfile.WebSocketRequest{}), vars.NewResolver(), Options{})
			if err != nil {
				t.Fatalf("ExecuteWebSocket: %v", err)
			}
			sum := wsTranscript(t, resp).Summary
			if sum.ClosedBy != tt.by || !slices.Contains(diag.Classes(sum.Err()), tt.class) {
				t.Fatalf("summary = %+v, want %s with class %s", sum, tt.by, tt.class)
			}
		})
	}
}
