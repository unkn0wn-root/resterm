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
	// Keep transcript publication ordered around writes without holding this
	// mutex during network I/O. Data frames received while an outbound frame is
	// in flight are replayed after the send event is recorded.
	mu     sync.Mutex
	out    bool
	q      []*stream.Event
	end    bool
	endErr error
	done   bool
	once   sync.Once
}

func (rt *wsRuntime) publishReceive(evt *stream.Event) {
	if evt == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.done {
		return
	}
	if rt.out {
		rt.q = append(rt.q, evt)
		return
	}
	rt.session.Publish(evt)
}

func (rt *wsRuntime) publishTerminal(evt *stream.Event, err error) {
	if evt == nil && err == nil {
		return
	}
	close := false
	rt.mu.Lock()
	if rt.done {
		rt.mu.Unlock()
		return
	}
	if rt.out {
		if evt != nil {
			rt.q = append(rt.q, evt)
		}
		rt.end = true
		rt.endErr = err
		rt.mu.Unlock()
		return
	}
	if evt != nil {
		rt.session.Publish(evt)
	}
	rt.done = true
	close = true
	rt.mu.Unlock()
	if close {
		rt.session.Close(err)
	}
}

func (rt *wsRuntime) closeTerminalNow(evt *stream.Event, err error) {
	close := false
	rt.mu.Lock()
	if !rt.done {
		for _, ev := range rt.q {
			rt.session.Publish(ev)
		}
		rt.q = nil
		rt.end = false
		rt.endErr = nil
		rt.out = false
		if evt != nil {
			rt.session.Publish(evt)
		}
		rt.done = true
		close = true
	}
	rt.mu.Unlock()
	if close {
		rt.session.Close(err)
	}
}

func (rt *wsRuntime) beginOutbound() bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.done {
		return false
	}
	rt.out = true
	return true
}

func (rt *wsRuntime) finishOutbound(evt *stream.Event, publish bool) {
	close := false
	var err error

	rt.mu.Lock()
	if !rt.done {
		rt.out = false
		if publish && evt != nil {
			rt.session.Publish(evt)
		}
		for _, ev := range rt.q {
			rt.session.Publish(ev)
		}
		if rt.end {
			close = true
			err = rt.endErr
			rt.done = true
		}
	}
	rt.out = false
	rt.q = nil
	rt.end = false
	rt.endErr = nil
	rt.mu.Unlock()
	if close {
		rt.session.Close(err)
	}
}

func (rt *wsRuntime) writeAndPublish(write func() error, evt *stream.Event) error {
	if !rt.beginOutbound() {
		return diag.New(diag.ClassProtocol, "websocket session closed")
	}
	if err := write(); err != nil {
		rt.finishOutbound(nil, false)
		return err
	}
	rt.touchActivity()
	rt.finishOutbound(evt, true)
	return nil
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
				rt.publishTerminal(&stream.Event{
					Kind:      stream.KindWebSocket,
					Direction: stream.DirReceive,
					Timestamp: time.Now(),
					Metadata:  meta,
					WS: stream.WSMetadata{
						Opcode: wsOpcodeClose,
						Code:   ce.Code,
						Reason: ce.Reason,
					},
				}, nil)
				return
			}
			if ctx.Err() != nil {
				rt.closeTerminalNow(nil, ctx.Err())
			} else {
				rt.closeTerminalNow(
					nil,
					diag.WrapAs(diag.ClassProtocol, err, "read websocket message"),
				)
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

		rt.publishReceive(&stream.Event{
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
			rt.closeTerminalNow(&stream.Event{
				Kind:      stream.KindWebSocket,
				Direction: stream.DirNA,
				Timestamp: time.Now(),
				Metadata:  meta,
			}, nil)
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

		payload := append([]byte(nil), msg.payload...)
		metadata := cloneMetadata(msg.metadata)
		if metadata == nil {
			metadata = map[string]string{}
		}
		if _, ok := metadata[wsMetaType]; !ok {
			metadata[wsMetaType] = opcodeToType(opcode)
		}

		evt := &stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirSend,
			Timestamp: time.Now(),
			Metadata:  metadata,
			Payload:   payload,
			WS: stream.WSMetadata{
				Opcode: opcode,
			},
		}
		if err := rt.writeAndPublish(func() error {
			return rt.conn.Write(ctx, msg.msgType, msg.payload)
		}, evt); err != nil {
			return diag.WrapAs(diag.ClassProtocol, err, "send websocket frame")
		}
		return nil
	case wsOutboundPing:
		payload := append([]byte(nil), msg.payload...)
		if len(payload) > websocketControlMaxPayload {
			return diag.Newf(
				diag.ClassProtocol,
				"websocket ping payload exceeds %d bytes",
				websocketControlMaxPayload,
			)
		}
		metadata := cloneMetadata(msg.metadata)
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata[wsMetaType] = "ping"
		evt := &stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirSend,
			Timestamp: time.Now(),
			Metadata:  metadata,
			Payload:   payload,
			WS: stream.WSMetadata{
				Opcode: wsOpcodePing,
			},
		}
		if err := rt.writeAndPublish(func() error {
			return wsWriteControl(rt.conn, ctx, wsOpcodePing, payload)
		}, evt); err != nil {
			return diag.WrapAs(diag.ClassProtocol, err, "send websocket ping")
		}
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

		metadata := cloneMetadata(msg.metadata)
		if metadata == nil {
			metadata = map[string]string{}
		}
		metadata[wsMetaType] = "pong"
		evt := &stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirSend,
			Timestamp: time.Now(),
			Metadata:  metadata,
			Payload:   payload,
			WS: stream.WSMetadata{
				Opcode: wsOpcodePong,
			},
		}
		if err := rt.writeAndPublish(func() error {
			return wsWriteControl(rt.conn, ctx, wsOpcodePong, payload)
		}, evt); err != nil {
			return diag.WrapAs(diag.ClassProtocol, err, "send websocket pong")
		}
		return nil
	case wsOutboundClose:
		session.MarkClosing()
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
		evt := &stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirSend,
			Timestamp: time.Now(),
			Metadata:  metadata,
			WS: stream.WSMetadata{
				Opcode: wsOpcodeClose,
				Code:   msg.code,
				Reason: msg.reason,
			},
		}
		if err := rt.writeAndPublish(func() error {
			return rt.conn.Close(msg.code, msg.reason)
		}, evt); err != nil {
			return diag.WrapAs(diag.ClassProtocol, err, "close websocket")
		}
		rt.closeTerminalNow(nil, nil)
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
