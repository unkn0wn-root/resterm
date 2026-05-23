package httpclient

import (
	"encoding/base64"
	"strconv"

	"github.com/unkn0wn-root/resterm/internal/stream"
)

type wsAccumulator struct {
	events  []WebSocketEvent
	summary WebSocketSummary
}

func newWSAccumulator() *wsAccumulator {
	return &wsAccumulator{
		events:  make([]WebSocketEvent, 0, 16),
		summary: WebSocketSummary{},
	}
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
		a.events = append(a.events, jsonEvt)
		if evt.Direction == stream.DirSend {
			a.summary.SentCount++
		} else {
			a.summary.ReceivedCount++
		}
		if typ == "close" {
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

func applyWebSocketSummaryDefaults(sum *WebSocketSummary, state stream.State, stateErr error) {
	if sum == nil {
		return
	}
	if sum.ClosedBy == "" {
		if state == stream.StateFailed || stateErr != nil {
			sum.ClosedBy = "error"
			if sum.CloseReason == "" && stateErr != nil {
				sum.CloseReason = stateErr.Error()
			}
		} else {
			sum.ClosedBy = "client"
		}
	}
	if sum.CloseReason == "" && stateErr != nil && sum.ClosedBy == "error" {
		sum.CloseReason = stateErr.Error()
	}
}
