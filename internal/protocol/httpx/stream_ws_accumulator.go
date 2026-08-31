package httpx

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

const DefaultWebSocketTranscriptBytes = 8 << 20

type wsAccumulator struct {
	events  []WebSocketEvent
	summary WebSocketSummary
	bytes   int64
	limit   int64
	closed  bool
}

func newWSAccumulator() *wsAccumulator {
	return &wsAccumulator{
		events: make([]WebSocketEvent, 0, 16),
		limit:  DefaultWebSocketTranscriptBytes,
	}
}

func (a *wsAccumulator) keep(evt *stream.Event) bool {
	size := evt.Size()
	if a.limit > 0 && a.bytes+size > a.limit {
		a.summary.Dropped++
		return false
	}
	a.bytes += size
	return true
}

func (a *wsAccumulator) consume(evt *stream.Event) {
	if evt == nil {
		return
	}
	meta := evt.Metadata
	typ := ""
	if meta != nil {
		typ = meta[wsMetaType]
	}
	switch evt.Direction {
	case stream.DirSend, stream.DirReceive:
		if typ == "" {
			typ = opcodeToType(evt.WS.Opcode)
		}
		jsonEvt := WebSocketEvent{
			Direction: directionToString(evt.Direction),
			Type:      typ,
			Timestamp: evt.Timestamp,
			Size:      len(evt.Payload),
		}
		if meta != nil {
			if step, ok := meta[wsMetaStep]; ok {
				jsonEvt.Step = step
			}
		}
		switch typ {
		case "text", "json", "pong", "ping":
			jsonEvt.Text = string(evt.Payload)
		case "binary":
			jsonEvt.Base64 = base64.StdEncoding.EncodeToString(evt.Payload)
		case "close":
			if meta != nil {
				if codeStr, ok := meta[wsMetaCloseCode]; ok {
					if code, err := strconv.Atoi(codeStr); err == nil {
						jsonEvt.Code = code
					}
				}
				if reason, ok := meta[wsMetaCloseReason]; ok {
					jsonEvt.Reason = reason
				}
			}
			if evt.WS.Code != 0 && jsonEvt.Code == 0 {
				jsonEvt.Code = int(evt.WS.Code)
			}
			if evt.WS.Reason != "" && jsonEvt.Reason == "" {
				jsonEvt.Reason = evt.WS.Reason
			}
		}
		if a.keep(evt) {
			a.events = append(a.events, jsonEvt)
		}
		if evt.Direction == stream.DirSend {
			a.summary.SentCount++
		} else {
			a.summary.ReceivedCount++
		}
		// Both sides send a close frame, so only the first one names who ended
		// the session. A close resterm sends is published before the reply it
		// gets back, and a close it never managed to send is not published at
		// all, which leaves the peer's frame first.
		if typ == "close" && !a.closed {
			a.closed = true
			if meta != nil {
				if by, ok := meta[wsMetaClosedBy]; ok {
					a.summary.ClosedBy = by
				}
				if reason, ok := meta[wsMetaCloseReason]; ok && reason != "" {
					a.summary.CloseReason = reason
				}
				if codeStr, ok := meta[wsMetaCloseCode]; ok {
					if code, err := strconv.Atoi(codeStr); err == nil {
						a.summary.CloseCode = code
					}
				}
			}
			if jsonEvt.Code != 0 {
				a.summary.CloseCode = jsonEvt.Code
			}
			if jsonEvt.Reason != "" {
				a.summary.CloseReason = jsonEvt.Reason
			}
		}
	case stream.DirNA:
		if meta != nil {
			if by, ok := meta[wsMetaClosedBy]; ok {
				a.summary.ClosedBy = by
			}
			if codeStr, ok := meta[wsMetaCloseCode]; ok {
				if code, err := strconv.Atoi(codeStr); err == nil {
					a.summary.CloseCode = code
				}
			}
			if reason, ok := meta[wsMetaCloseReason]; ok {
				a.summary.CloseReason = reason
			}
		}
	}
}

func directionToString(dir stream.Direction) string {
	switch dir {
	case stream.DirSend:
		return "send"
	case stream.DirReceive:
		return "receive"
	default:
		return "info"
	}
}

// applyWebSocketSummaryDefaults fills fields not set by terminal events. Idle
// timeouts have no error class, while caller deadlines do.
func applyWebSocketSummaryDefaults(sum *WebSocketSummary, state stream.State, stateErr error) {
	if sum == nil {
		return
	}
	if sum.ClosedBy == "" {
		switch {
		case errors.Is(stateErr, context.Canceled):
			sum.ClosedBy = wsClosedByCanceled
		case errors.Is(stateErr, context.DeadlineExceeded):
			sum.ClosedBy = wsClosedByTimeout
		case state == stream.StateFailed || stateErr != nil:
			sum.ClosedBy = wsClosedByError
		default:
			sum.ClosedBy = wsClosedByClient
		}
	}
	if stateErr == nil {
		return
	}
	switch sum.ClosedBy {
	case wsClosedByCanceled, wsClosedByTimeout, wsClosedByError:
		if class := diag.ClassOf(stateErr); class.Known() {
			sum.ErrorClass = class
		}
		if sum.CloseReason == "" {
			sum.CloseReason = stateErr.Error()
		}
	}
}
