package httpclient

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"nhooyr.io/websocket"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/util"
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
		return diag.New(diag.ClassProtocol, "invalid json payload for websocket send")
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
		return diag.WrapAs(diag.ClassProtocol, err, "decode base64 payload")
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
		return diag.Newf(
			diag.ClassProtocol,
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
	return util.CloneMap(in)
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
