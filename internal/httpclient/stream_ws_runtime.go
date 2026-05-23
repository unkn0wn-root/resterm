package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

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
				session.Close(diag.WrapAs(diag.ClassProtocol, err, "read websocket message"))
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
			return diag.WrapAs(diag.ClassProtocol, err, "send websocket frame")
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
			return diag.WrapAs(diag.ClassProtocol, err, "send websocket ping")
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
			return diag.Newf(
				diag.ClassProtocol,
				"websocket pong payload exceeds %d bytes",
				websocketControlMaxPayload,
			)
		}
		if err := wsWriteControl(rt.conn, ctx, wsOpcodePong, payload); err != nil {
			return diag.WrapAs(diag.ClassProtocol, err, "send websocket pong")
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
			return diag.WrapAs(diag.ClassProtocol, err, "close websocket")
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
				rt.session.Close(
					diag.WrapAs(diag.ClassProtocol, err, "close websocket connection"),
				)
			}
		}
	})
}
