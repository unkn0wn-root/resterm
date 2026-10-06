package httpx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

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
	// closeStarted is claimed by whoever sends the first close frame, so the
	// connection is never closed a second time.
	closeStarted atomic.Bool
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
				// Read completes the close handshake before returning a close error.
				// Do not let shutdown try to close the connection again.
				rt.closeStarted.Store(true)
				rt.publishTerminal(&stream.Event{
					Kind:      stream.KindWebSocket,
					Direction: stream.DirReceive,
					Timestamp: time.Now(),
					WS: stream.WSMetadata{
						Type:     stream.WSClose,
						ClosedBy: stream.WSClosedByServer,
						Code:     stream.WSCloseCode(ce.Code),
						Reason:   ce.Reason,
					},
				}, nil)
				return
			}
			if ctx.Err() != nil {
				rt.closeTerminalNow(nil, ctx.Err())
			} else {
				rt.closeTerminalNow(
					nil,
					diag.Wrap(err, "read websocket message"),
				)
			}
			return
		}

		rt.touchActivity()

		typ := stream.WSBinary
		if msgType == websocket.MessageText {
			typ = stream.WSText
		}
		rt.publishReceive(&stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirReceive,
			Timestamp: time.Now(),
			Payload:   append([]byte(nil), data...),
			WS:        stream.WSMetadata{Type: typ},
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
			rt.closeTerminalNow(&stream.Event{
				Kind:      stream.KindWebSocket,
				Direction: stream.DirNA,
				Timestamp: time.Now(),
				WS: stream.WSMetadata{
					ClosedBy: stream.WSClosedByTimeout,
					Reason:   fmt.Sprintf("idle timeout after %s", limit),
				},
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

	evt := &stream.Event{
		Kind:      stream.KindWebSocket,
		Direction: stream.DirSend,
		Timestamp: time.Now(),
		Payload:   msg.payload,
		WS:        stream.WSMetadata{Type: msg.typ, Step: msg.step},
	}
	switch msg.kind {
	case wsOutboundMessage:
		msgType := websocket.MessageText
		if msg.typ == stream.WSBinary {
			msgType = websocket.MessageBinary
		}
		if err := rt.writeAndPublish(func() error {
			return rt.conn.Write(ctx, msgType, msg.payload)
		}, evt); err != nil {
			return diag.Wrap(err, "send websocket frame")
		}
		return nil
	case wsOutboundPing:
		if err := rt.writeAndPublish(func() error {
			return wsWriteControl(rt.conn, ctx, wsOpcodePing, msg.payload)
		}, evt); err != nil {
			return diag.Wrap(err, "send websocket ping")
		}
		return nil
	case wsOutboundPong:
		if err := rt.writeAndPublish(func() error {
			return wsWriteControl(rt.conn, ctx, wsOpcodePong, msg.payload)
		}, evt); err != nil {
			return diag.Wrap(err, "send websocket pong")
		}
		return nil
	case wsOutboundClose:
		if !rt.closeStarted.CompareAndSwap(false, true) {
			return nil
		}
		session.MarkClosing()
		evt.WS.ClosedBy = stream.WSClosedByClient
		evt.WS.Code = msg.code
		evt.WS.Reason = msg.reason
		if err := rt.writeAndPublish(func() error {
			return rt.conn.Close(websocket.StatusCode(msg.code), msg.reason)
		}, evt); err != nil {
			return diag.Wrap(err, "close websocket")
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
		// A close that already started sent its own frame and ends the session
		// its own way. Closing again reports a second, bogus failure.
		if !rt.closeStarted.CompareAndSwap(false, true) {
			return
		}
		if err := rt.conn.Close(websocket.StatusNormalClosure, ""); err != nil &&
			!errors.Is(err, net.ErrClosed) && !errors.Is(err, context.Canceled) {
			rt.session.Close(
				diag.Wrap(err, "close websocket connection"),
			)
		}
	})
}
