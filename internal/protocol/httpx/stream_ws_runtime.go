package httpx

import (
	"context"
	"encoding/binary"
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
	// writeCh is never closed because steps and the console send on it concurrently.
	writeCh chan wsOutbound
	cancel  context.CancelFunc
	pulse   chan struct{}
	// limit is the read limit, named in the error a larger message ends with.
	limit int64
	// q keeps the transcript in the order things happened without holding mu
	// during network I/O. A write holds its place in q while it is in flight,
	// so whatever comes after it waits for its send event.
	mu       sync.Mutex
	out      int
	q        []*wsQueued
	qBytes   int64
	end      bool
	endErr   error
	writeErr error
	done     bool
	// ending is how the session ends. The first ending asked for sets it, and
	// the read loop applies it once Read returns.
	ending stream.WSMetadata
	// idle is the event an idle timeout publishes when it ends the session.
	idle *stream.Event
	// stopErr is the failure a step or the caller ended the session with.
	stopErr error
	// cause is why CompleteWebSocket canceled the session. The session's
	// context only says it was canceled.
	cause error
	// closeTimer gives up on the peer's answer to resterm's close.
	closeTimer *time.Timer
	once       sync.Once
	// closeStarted is claimed by whoever sends the first close frame, so the
	// connection is never closed a second time.
	closeStarted atomic.Bool
}

// wsQueued is a transcript event waiting for the writes that started before
// it. A write's entry gets its send event when the write succeeds.
type wsQueued struct {
	evt     *stream.Event
	writing bool
}

// A frame received while a write is in flight waits for the write's send
// event. Past DefaultWebSocketTranscriptBytes everything but the writes goes
// out first and later frames go out as they arrive, so a long write cannot
// hold them all.
func (rt *wsRuntime) publishReceive(evt *stream.Event) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.done {
		return
	}
	if rt.out > 0 && rt.qBytes < DefaultWebSocketTranscriptBytes {
		rt.q = append(rt.q, &wsQueued{evt: evt})
		rt.qBytes += evt.Size()
		return
	}
	writes := rt.q[:0]
	for _, e := range rt.q {
		if e.writing {
			writes = append(writes, e)
		} else if e.evt != nil {
			rt.session.Publish(e.evt)
		}
	}
	rt.q = writes
	rt.session.Publish(evt)
}

// finish ends the session once Read has returned err. Only the read loop calls
// it, so every frame Read returned is recorded before the session is done.
func (rt *wsRuntime) finish(err error) {
	var ce websocket.CloseError
	peer := errors.As(err, &ce)
	var events []*stream.Event
	if peer {
		events = append(events, &stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirReceive,
			Timestamp: time.Now(),
			WS: stream.WSMetadata{
				Type:     stream.WSClose,
				ClosedBy: stream.WSClosedByServer,
				Code:     stream.WSCloseCode(ce.Code),
				Reason:   ce.Reason,
			},
		})
	} else {
		// A write in flight may hold the real cause. Closing keeps it from blocking.
		_ = rt.conn.CloseNow()
	}
	ctx := rt.session.Context()

	rt.mu.Lock()
	asked := rt.ending.ClosedBy != "" || rt.stopErr != nil
	switch {
	case peer && !asked:
		rt.ending = events[0].WS
		// Read completes the close handshake before returning a close error.
		// Do not let shutdown try to close the connection again.
		rt.closeStarted.Store(true)
		err = nil
	case rt.stopErr != nil:
		err = rt.stopErr
	case peer:
		// The answer to resterm's close, or a close that crossed idle.
		err = nil
	case ctx.Err() != nil:
		err = ctx.Err()
		if rt.cause != nil {
			err = rt.cause
		}
	case asked:
		// Resterm's close or idle closed the connection under Read.
		err = nil
	case errors.Is(err, websocket.ErrMessageTooBig):
		err = diag.Newf(
			diag.ClassProtocol,
			"websocket message exceeds %d bytes, raise it with @websocket max-message-bytes",
			rt.limit,
		)
	default:
		err = diag.Wrap(err, "read websocket message")
	}
	if rt.idle != nil {
		events = append(events, rt.idle)
	}
	if rt.out > 0 {
		for _, ev := range events {
			rt.q = append(rt.q, &wsQueued{evt: ev})
		}
		rt.end, rt.endErr = true, err
		rt.mu.Unlock()
		return
	}
	for _, ev := range events {
		rt.session.Publish(ev)
	}
	err = endCause(err, rt.writeErr)
	rt.done = true
	rt.mu.Unlock()
	rt.session.Close(err)
}

// A read fails with net.ErrClosed when the connection closed under it, as it
// does when a write times out. The failed write then says why the session ended.
func endCause(read, write error) error {
	if write != nil && errors.Is(read, net.ErrClosed) {
		return write
	}
	return read
}

// stop asks the read loop to end the session, as idle with the idle event or
// as a failure with err. The first ending asked for wins, but idle still cuts
// a close the peer has not answered.
func (rt *wsRuntime) stop(idle *stream.Event, err error) {
	rt.mu.Lock()
	first := !rt.done && rt.ending.ClosedBy == "" && rt.stopErr == nil
	if first && idle != nil {
		rt.ending, rt.idle = idle.WS, idle
	} else if first {
		rt.stopErr = err
	}
	rt.mu.Unlock()
	if first || idle != nil {
		_ = rt.conn.CloseNow()
	}
}

// sendClose sends resterm's close frame unless the session already has an
// ending. coder marks the frame as sent, so Read returns the peer's answer
// and the read loop ends the session.
func (rt *wsRuntime) sendClose(ctx context.Context, code stream.WSCloseCode, reason, step string) error {
	rt.mu.Lock()
	if rt.done {
		rt.mu.Unlock()
		return diag.New(diag.ClassProtocol, "websocket session closed")
	}
	first := rt.ending.ClosedBy == "" && rt.stopErr == nil
	if first {
		rt.ending.ClosedBy = stream.WSClosedByClient
		rt.closeStarted.Store(true)
	}
	rt.mu.Unlock()
	if !first {
		return nil
	}
	rt.session.MarkClosing()
	evt := &stream.Event{
		Kind:      stream.KindWebSocket,
		Direction: stream.DirSend,
		Timestamp: time.Now(),
		WS: stream.WSMetadata{
			Type:     stream.WSClose,
			Step:     step,
			ClosedBy: stream.WSClosedByClient,
			Code:     code,
			Reason:   reason,
		},
	}
	payload := append(binary.BigEndian.AppendUint16(nil, uint16(code)), reason...)
	if err := rt.writeAndPublish("close websocket", func() error {
		return wsWriteControl(rt.conn, ctx, wsOpcodeClose, payload)
	}, evt); err != nil {
		_ = rt.conn.CloseNow()
		return err
	}
	rt.mu.Lock()
	if !rt.done {
		// Like coder's Close, the peer gets websocketCloseTimeout to answer.
		rt.closeTimer = time.AfterFunc(websocketCloseTimeout, func() { _ = rt.conn.CloseNow() })
	}
	rt.mu.Unlock()
	return nil
}

// beginOutbound returns the write's place in the transcript, or nil once the
// session is done.
func (rt *wsRuntime) beginOutbound() *wsQueued {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.done {
		return nil
	}
	rt.out++
	w := &wsQueued{writing: true}
	rt.q = append(rt.q, w)
	return w
}

func (rt *wsRuntime) finishOutbound(w *wsQueued, evt *stream.Event, err error) {
	close := false
	var cause error

	rt.mu.Lock()
	rt.out--
	w.writing = false
	if err == nil && evt != nil {
		w.evt = evt
		if evt.WS.Type == stream.WSClose {
			rt.ending.Code, rt.ending.Reason = evt.WS.Code, evt.WS.Reason
		}
	}
	if err != nil && !errors.Is(err, net.ErrClosed) {
		rt.writeErr = err
	}
	i := 0
	for ; i < len(rt.q) && !rt.q[i].writing; i++ {
		if e := rt.q[i]; e.evt != nil {
			rt.session.Publish(e.evt)
		}
	}
	rt.q = rt.q[i:]
	if rt.out == 0 {
		rt.q, rt.qBytes = nil, 0
		if rt.end {
			close = true
			cause = endCause(rt.endErr, err)
			rt.done = true
		}
	}
	rt.mu.Unlock()
	if close {
		rt.session.Close(cause)
	}
}

func (rt *wsRuntime) writeAndPublish(op string, write func() error, evt *stream.Event) error {
	w := rt.beginOutbound()
	if w == nil {
		return diag.Wrap(diag.New(diag.ClassProtocol, "websocket session closed"), op)
	}
	if err := write(); err != nil {
		err = diag.Wrap(err, op)
		rt.finishOutbound(w, nil, err)
		return err
	}
	rt.touchActivity()
	rt.finishOutbound(w, evt, nil)
	return nil
}

func (rt *wsRuntime) readLoop() {
	session := rt.session
	ctx := session.Context()
	defer rt.shutdown()

	for {
		msgType, data, err := rt.conn.Read(ctx)
		if err != nil {
			rt.finish(err)
			return
		}

		rt.touchActivity()

		typ := stream.WSBinary
		if msgType == websocket.MessageText {
			typ = stream.WSText
		}
		rt.receive(typ, data)
	}
}

// Pings and pongs from the peer go into the transcript. They are not activity,
// so a keepalive cannot hold an idle session open. After resterm's close frame
// coder must not answer a ping, because the pong would fail the read.
func (rt *wsRuntime) pingReceived(_ context.Context, payload []byte) bool {
	rt.receive(stream.WSPing, payload)
	return !rt.closeStarted.Load()
}

func (rt *wsRuntime) pongReceived(_ context.Context, payload []byte) {
	rt.receive(stream.WSPong, payload)
}

func (rt *wsRuntime) receive(typ stream.WSType, payload []byte) {
	rt.publishReceive(&stream.Event{
		Kind:      stream.KindWebSocket,
		Direction: stream.DirReceive,
		Timestamp: time.Now(),
		Payload:   append([]byte(nil), payload...),
		WS:        stream.WSMetadata{Type: typ},
	})
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
			rt.stop(&stream.Event{
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

// writeLoop never ends the session, and a failed write fails only its send. A write
// can fail before it touches the connection. readLoop ends the session.
func (rt *wsRuntime) writeLoop() {
	ctx := rt.session.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-rt.writeCh:
			err := rt.performWrite(msg)
			if msg.result != nil {
				msg.result <- err
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
		return rt.writeAndPublish("send websocket frame", func() error {
			return rt.conn.Write(ctx, msgType, msg.payload)
		}, evt)
	case wsOutboundPing:
		return rt.writeAndPublish("send websocket ping", func() error {
			return wsWriteControl(rt.conn, ctx, wsOpcodePing, msg.payload)
		}, evt)
	case wsOutboundPong:
		return rt.writeAndPublish("send websocket pong", func() error {
			return wsWriteControl(rt.conn, ctx, wsOpcodePong, msg.payload)
		}, evt)
	default:
		return nil
	}
}

func (rt *wsRuntime) shutdown() {
	rt.once.Do(func() {
		rt.mu.Lock()
		if rt.closeTimer != nil {
			rt.closeTimer.Stop()
		}
		rt.mu.Unlock()
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
