package httpx

import (
	"cmp"
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

// runWSSteps reports whether a step closed the connection. A step that fails
// ends the session with its error instead of returning it, so the caller can
// still build the transcript.
func (c *Client) runWSSteps(
	session *stream.Session,
	sender *WebSocketSender,
	req *restfile.Request,
	baseDir string,
	opts Options,
) bool {
	wsReq := req.WebSocket
	ctx := session.Context()
	recvWindow := wsRecvWindow(wsReq.Options)
	lookup := newFileLookup(baseDir, opts)
	closedByScript := false

	for idx, step := range wsReq.Steps {
		sender.touch()

		if ctx.Err() != nil {
			return closedByScript
		}

		label := fmt.Sprintf("%d:%s", idx+1, string(step.Type))
		meta := map[string]string{wsMetaStep: label}

		var err error
		switch step.Type {
		case restfile.WebSocketStepSendText:
			meta[wsMetaType] = "text"
			err = sender.SendText(ctx, step.Value, meta)
		case restfile.WebSocketStepSendJSON:
			meta[wsMetaType] = "json"
			err = sender.SendJSON(ctx, cmp.Or(strings.TrimSpace(step.Value), "{}"), meta)
		case restfile.WebSocketStepSendBase64:
			meta[wsMetaType] = "binary"
			err = sender.SendBase64(ctx, step.Value, meta)
		case restfile.WebSocketStepSendFile:
			meta[wsMetaType] = "binary"
			var data []byte
			data, _, err = c.readFile(lookup, step.File, "websocket payload file")
			if err == nil {
				err = sender.SendBinary(ctx, data, meta)
			}
		case restfile.WebSocketStepPing:
			meta[wsMetaType] = "ping"
			err = sender.Ping(ctx, step.Value, meta)
		case restfile.WebSocketStepPong:
			err = sender.Pong(ctx, step.Value, meta)
		case restfile.WebSocketStepWait:
			err = waitForDuration(ctx, step.Duration)
		case restfile.WebSocketStepClose:
			meta[wsMetaType] = "close"
			code := cmp.Or(websocket.StatusCode(step.Code), websocket.StatusNormalClosure)
			err = sender.Close(ctx, code, step.Reason, meta)
			closedByScript = err == nil
		}

		if err != nil {
			if ctx.Err() == nil {
				// The session is still open, so the step itself failed.
				sender.fail(diag.WrapAs(diag.ClassProtocol, err, "websocket step "+label))
			}
			return closedByScript
		}

		if wsStepSettles(step.Type) {
			waitForWindow(ctx, recvWindow)
		}
	}

	return closedByScript
}

// A step that sent a frame pauses for the answer. Wait and close have their own timing.
func wsStepSettles(step restfile.WebSocketStepType) bool {
	switch step {
	case restfile.WebSocketStepWait, restfile.WebSocketStepClose:
		return false
	}
	return true
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
