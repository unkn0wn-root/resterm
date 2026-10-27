package httpx

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

const DefaultWebSocketTranscriptBytes = 8 << 20

type wsAccumulator struct {
	events  []WebSocketEvent
	summary WebSocketSummary
	bytes   int64
	limit   int64
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
	ws := evt.WS
	switch evt.Direction {
	case stream.DirSend, stream.DirReceive:
		jsonEvt := WebSocketEvent{
			Step:      ws.Step,
			Direction: directionToString(evt.Direction),
			Type:      ws.Type,
			Timestamp: evt.Timestamp,
			Size:      len(evt.Payload),
			Code:      int(ws.Code),
			Reason:    ws.Reason,
		}
		switch ws.Type {
		case stream.WSText, stream.WSJSON, stream.WSPing, stream.WSPong:
			jsonEvt.Text = string(evt.Payload)
		case stream.WSBinary:
			jsonEvt.Base64 = base64.StdEncoding.EncodeToString(evt.Payload)
		}
		if a.keep(evt) {
			a.events = append(a.events, jsonEvt)
		}
		if evt.Direction == stream.DirSend {
			a.summary.SentCount++
		} else {
			a.summary.ReceivedCount++
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
			sum.ClosedBy = stream.WSClosedByCanceled
		case errors.Is(stateErr, context.DeadlineExceeded):
			sum.ClosedBy = stream.WSClosedByTimeout
		case state == stream.StateFailed || stateErr != nil:
			sum.ClosedBy = stream.WSClosedByError
		default:
			sum.ClosedBy = stream.WSClosedByClient
		}
	}
	if stateErr == nil {
		return
	}
	switch sum.ClosedBy {
	case stream.WSClosedByCanceled, stream.WSClosedByTimeout, stream.WSClosedByError:
		if class := diag.ClassOf(stateErr); class.Known() {
			sum.ErrorClass = class
		}
		if sum.CloseReason == "" {
			sum.CloseReason = stateErr.Error()
		}
	}
}
