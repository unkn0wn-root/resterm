package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// GraphQL over WebSocket speaks graphql-transport-ws, the protocol of the
// graphql-ws library. Each socket carries one operation.
const (
	graphQLWSID   = "1"
	graphQLWSStep = "graphql"
	graphQLWSInit = `{"type":"connection_init"}`
	graphQLWSPong = `{"type":"pong"}`
)

type graphQLWSMessage struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// GraphQLSubscribeMessage builds the subscribe message that starts gql over
// graphql-transport-ws.
func (c *Client) GraphQLSubscribeMessage(
	gql *restfile.GraphQLBody,
	resolver *vars.Resolver,
	opts Options,
) (string, error) {
	op, err := c.prepareGraphQLOperation(gql, resolver, opts)
	if err != nil {
		return "", err
	}
	payload, err := buildGraphQLPayload(op)
	if err != nil {
		return "", err
	}
	msg, err := json.Marshal(graphQLWSMessage{ID: graphQLWSID, Type: "subscribe", Payload: payload})
	if err != nil {
		return "", diag.WrapAs(diag.ClassProtocol, err, "encode graphql subscribe message")
	}
	return string(msg), nil
}

type graphQLWS struct {
	sender     *WebSocketSender
	subscribe  string
	subscribed chan struct{}
	// acked and over are read and set on the read loop only. over is set once
	// the operation has ended.
	acked bool
	over  bool

	// mu guards the replies the read loop owes the server until run sends them.
	mu            sync.Mutex
	owedPongs     int
	owedSubscribe bool
	wake          chan struct{}
}

// receive acts on a message once the read loop has recorded it, with the type
// and error that check returned. Once the operation ends it sends resterm's
// close, and the read loop records the rest until the server answers.
func (g *graphQLWS) receive(ctx context.Context, t string, err error) {
	var code stream.WSCloseCode
	var reason string
	switch {
	case err != nil:
		// The spec asks for close code 4400 on a message it does not allow.
		code, reason = 4400, "invalid message"
	case t == "complete":
		code, reason = stream.WSCloseNormal, "operation complete"
	case t == "error":
		code, reason = stream.WSCloseNormal, "operation failed"
	case t == "ping":
		g.mu.Lock()
		g.owedPongs++
		g.mu.Unlock()
		g.notify()
		return
	case t == "connection_ack" && !g.acked:
		g.acked = true
		g.mu.Lock()
		g.owedSubscribe = true
		g.mu.Unlock()
		g.notify()
		return
	default:
		return
	}
	g.over = true
	_ = g.sender.Close(ctx, code, reason, graphQLWSStep)
}

// pending fails a session that evt ends before connection_ack, because the
// operation never started.
func (g *graphQLWS) pending(evt *stream.Event) error {
	switch {
	case g.acked:
		return nil
	case evt.WS.ClosedBy == stream.WSClosedByTimeout:
		return diag.New(diag.ClassTimeout, "server sent no connection_ack before the idle timeout")
	default:
		return diag.Newf(
			diag.ClassProtocol,
			"server closed the socket with %d %q before connection_ack",
			evt.WS.Code,
			evt.WS.Reason,
		)
	}
}

// notify wakes the goroutine that start runs without blocking the read loop.
func (g *graphQLWS) notify() {
	select {
	case g.wake <- struct{}{}:
	default:
	}
}

// start sends connection_init and returns once subscribe is sent or the
// session ended. The channel it returns closes when its goroutine stops.
func (g *graphQLWS) start(session *stream.Session) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx := session.Context()
		if err := g.run(ctx); err != nil && ctx.Err() == nil {
			g.sender.fail(err)
		}
	}()
	select {
	case <-g.subscribed:
	case <-done:
	}
	return done
}

// run sends connection_init, then subscribe and the pongs the read loop owes
// the server each time it wakes.
func (g *graphQLWS) run(ctx context.Context) error {
	if err := g.sender.SendJSON(ctx, graphQLWSInit, graphQLWSStep); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-g.wake:
		}
		g.mu.Lock()
		subscribe, pongs := g.owedSubscribe, g.owedPongs
		g.owedSubscribe, g.owedPongs = false, 0
		g.mu.Unlock()
		if subscribe {
			if err := g.sender.SendJSON(ctx, g.subscribe, graphQLWSStep); err != nil {
				return err
			}
			close(g.subscribed)
		}
		for range pongs {
			if err := g.sender.SendJSON(ctx, graphQLWSPong, graphQLWSStep); err != nil {
				return err
			}
		}
	}
}

// check judges a server message the way the graphql-ws reference validator
// does, before the read loop records it. It returns the message type for
// receive, or the error when the message breaks the protocol. A message for
// another operation has no type. Field names match exactly, as in JSON.
func (g *graphQLWS) check(typ stream.WSType, data []byte) (string, error) {
	// The spec lets a client ignore what arrives once the operation is over.
	if g.over {
		return "", nil
	}
	if typ == stream.WSBinary {
		return "", diag.New(diag.ClassProtocol, "server sent a binary graphql-transport-ws message")
	}
	var msg map[string]json.RawMessage
	err := json.Unmarshal(data, &msg)
	var syntax *json.SyntaxError
	var t string
	switch {
	case errors.As(err, &syntax):
		return "", diag.New(diag.ClassProtocol, "server sent a graphql-transport-ws message that is not valid JSON")
	case err != nil || msg == nil:
		return "", diag.New(diag.ClassProtocol, "server sent a graphql-transport-ws message that is not a JSON object")
	case json.Unmarshal(msg["type"], &t) != nil || t == "":
		return "", diag.New(diag.ClassProtocol, "server sent a graphql-transport-ws message without a type")
	}
	// A RawMessage holds the value without the space around it.
	payload := msg["payload"]
	switch t {
	case "connection_ack", "ping", "pong":
		if payload != nil && payload[0] != '{' && string(payload) != "null" {
			return "", diag.Newf(
				diag.ClassProtocol,
				"server sent %s with a payload that is neither an object nor null",
				t,
			)
		}
	case "next", "error", "complete":
		var id string
		if json.Unmarshal(msg["id"], &id) != nil || id == "" {
			return "", diag.Newf(diag.ClassProtocol, "server sent %s without a non-empty string id", t)
		}
		if t == "next" && (payload == nil || payload[0] != '{') {
			return "", diag.New(diag.ClassProtocol, "server sent next without an object payload")
		}
		if t == "error" {
			var errs []map[string]json.RawMessage
			if json.Unmarshal(payload, &errs) != nil || len(errs) == 0 ||
				slices.ContainsFunc(errs, func(e map[string]json.RawMessage) bool { return e["message"] == nil }) {
				return "", diag.New(diag.ClassProtocol, "server sent error without a list of GraphQL errors")
			}
		}
		if !g.acked {
			return "", diag.Newf(diag.ClassProtocol, "server sent %s before connection_ack", t)
		}
		if id != graphQLWSID {
			// The spec lets a client ignore messages for operations it does not know.
			return "", nil
		}
	case "connection_init", "subscribe":
		return "", diag.Newf(diag.ClassProtocol, "server sent %s, a message only a client sends", t)
	default:
		return "", diag.New(diag.ClassProtocol, "server sent a graphql-transport-ws message of an unknown type")
	}
	return t, nil
}
