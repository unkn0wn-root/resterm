package httpclient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nhooyr.io/websocket"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

func wsRecvWindow(opts restfile.WebSocketOptions) time.Duration {
	win := 250 * time.Millisecond
	if opts.IdleTimeout <= 0 {
		return win
	}
	half := opts.IdleTimeout / 2
	if half > 0 && half < win {
		return half
	}
	return win
}

func (c *Client) runWSSteps(
	session *stream.Session,
	sender *WebSocketSender,
	req *restfile.Request,
	baseDir string,
	opts Options,
) (bool, error) {
	if req == nil || req.WebSocket == nil {
		return false, diag.New(diag.ClassProtocol, "websocket request missing")
	}

	wsReq := req.WebSocket
	ctx := session.Context()
	recvWindow := wsRecvWindow(wsReq.Options)
	lookup := newFileLookup(baseDir, opts)
	closedByScript := false

	for idx, step := range wsReq.Steps {
		sender.touch()

		if err := ensureSessionAlive(session); err != nil {
			return false, err
		}

		label := fmt.Sprintf("%d:%s", idx+1, string(step.Type))
		meta := map[string]string{wsMetaStep: label}
		switch step.Type {
		case restfile.WebSocketStepSendText:
			meta[wsMetaType] = "text"
			if err := sender.SendText(ctx, step.Value, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepSendJSON:
			payload := strings.TrimSpace(step.Value)
			if payload == "" {
				payload = "{}"
			}
			meta[wsMetaType] = "json"
			if err := sender.SendJSON(ctx, payload, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepSendBase64:
			meta[wsMetaType] = "binary"
			if err := sender.SendBase64(ctx, step.Value, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepSendFile:
			data, _, readErr := lookup.read(c, step.File, "websocket payload file")
			if readErr != nil {
				session.Cancel()
				return false, readErr
			}
			meta[wsMetaType] = "binary"
			if err := sender.SendBinary(ctx, data, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepPing:
			meta[wsMetaType] = "ping"
			if err := sender.Ping(ctx, step.Value, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepPong:
			if err := sender.Pong(ctx, step.Value, meta); err != nil {
				session.Cancel()
				return false, err
			}
			waitForWindow(ctx, recvWindow)
		case restfile.WebSocketStepWait:
			if err := waitForDuration(ctx, step.Duration); err != nil {
				session.Cancel()
				return false, err
			}
		case restfile.WebSocketStepClose:
			meta[wsMetaType] = "close"
			code := websocket.StatusNormalClosure
			if step.Code != 0 {
				code = websocket.StatusCode(step.Code)
			}
			if err := sender.Close(ctx, code, step.Reason, meta); err != nil {
				session.Cancel()
				return false, err
			}
			closedByScript = true
		}
	}

	return closedByScript, nil
}

func waitForWindow(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	_ = waitForDuration(ctx, d)
}

func waitForDuration(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func ensureSessionAlive(session *stream.Session) error {
	if session == nil {
		return diag.New(diag.ClassProtocol, "websocket session missing")
	}
	if err := session.Context().Err(); err != nil {
		if sessErr := session.Err(); sessErr != nil {
			return sessErr
		}
		return diag.WrapAs(diag.ClassProtocol, err, "websocket session closed")
	}
	return nil
}
