package httpx

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

type WebSocketSender struct {
	runtime *wsRuntime
}

func (s *WebSocketSender) touch() {
	if s == nil || s.runtime == nil {
		return
	}
	s.runtime.touchActivity()
}

// fail ends the session with err and stops publishing, so a close frame from
// the peer cannot replace the real reason the run stopped.
func (s *WebSocketSender) fail(err error) {
	if s == nil || s.runtime == nil {
		return
	}
	s.runtime.closeTerminalNow(nil, err)
}

// Multiple contexts race here: per-message timeout, session lifetime, and write
// completion. Nested selects give priority to results that are already available.
func (s *WebSocketSender) enqueue(msg wsOutbound) (err error) {
	if msg.ctx == nil {
		msg.ctx = s.runtime.session.Context()
	}

	defer func() {
		if r := recover(); r != nil {
			err = diag.New(diag.ClassProtocol, "websocket session closed")
			if msg.result != nil {
				msg.result <- err
			}
		}
	}()

	select {
	case <-s.runtime.session.Context().Done():
		return diag.New(diag.ClassProtocol, "websocket session closed")
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
						return diag.New(diag.ClassProtocol, "websocket session closed")
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
		return diag.New(diag.ClassProtocol, "websocket session closed")
	}
}

func (s *WebSocketSender) SendText(ctx context.Context, text, step string) error {
	return s.send(ctx, stream.WSText, []byte(text), step)
}

func (s *WebSocketSender) SendJSON(ctx context.Context, jsonPayload, step string) error {
	if !json.Valid([]byte(jsonPayload)) {
		return diag.New(diag.ClassProtocol, "invalid json payload for websocket send")
	}
	return s.send(ctx, stream.WSJSON, []byte(jsonPayload), step)
}

func (s *WebSocketSender) SendBinary(ctx context.Context, data []byte, step string) error {
	return s.send(ctx, stream.WSBinary, append([]byte(nil), data...), step)
}

func (s *WebSocketSender) SendBase64(ctx context.Context, data, step string) error {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return diag.WrapAs(diag.ClassProtocol, err, "decode base64 payload")
	}
	return s.SendBinary(ctx, decoded, step)
}

func (s *WebSocketSender) send(ctx context.Context, typ stream.WSType, payload []byte, step string) error {
	return s.enqueue(wsOutbound{
		ctx:     ctx,
		kind:    wsOutboundMessage,
		typ:     typ,
		payload: payload,
		step:    step,
		result:  make(chan error, 1),
	})
}

func (s *WebSocketSender) Ping(ctx context.Context, payload, step string) error {
	return s.control(ctx, wsOutboundPing, stream.WSPing, payload, step)
}

func (s *WebSocketSender) Pong(ctx context.Context, payload, step string) error {
	return s.control(ctx, wsOutboundPong, stream.WSPong, payload, step)
}

func (s *WebSocketSender) control(
	ctx context.Context,
	kind wsOutboundKind,
	typ stream.WSType,
	payload, step string,
) error {
	if len(payload) > websocketControlMaxPayload {
		return diag.Newf(
			diag.ClassProtocol,
			"websocket %s payload exceeds %d bytes",
			typ,
			websocketControlMaxPayload,
		)
	}
	return s.enqueue(wsOutbound{
		ctx:     ctx,
		kind:    kind,
		typ:     typ,
		payload: []byte(payload),
		step:    step,
		result:  make(chan error, 1),
	})
}

func (s *WebSocketSender) Close(ctx context.Context, code stream.WSCloseCode, reason, step string) error {
	return s.enqueue(wsOutbound{
		ctx:    ctx,
		kind:   wsOutboundClose,
		typ:    stream.WSClose,
		code:   code,
		reason: reason,
		step:   step,
		result: make(chan error, 1),
	})
}
