package ui

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

func streamInfoFromResponse(
	req *restfile.Request,
	resp *httpclient.Response,
) (*scripts.StreamInfo, error) {
	if req == nil || resp == nil {
		return nil, nil
	}
	streamType := strings.ToLower(resp.Headers.Get(streamHeaderType))
	if req.SSE != nil && streamType == "sse" {
		transcript, err := httpclient.DecodeSSETranscript(resp.Body)
		if err != nil {
			return nil, err
		}
		return convertSSETranscript(transcript), nil
	}
	if req.WebSocket != nil && streamType == "websocket" {
		transcript, err := httpclient.DecodeWebSocketTranscript(resp.Body)
		if err != nil {
			return nil, err
		}
		return convertWebSocketTranscript(transcript), nil
	}
	return nil, nil
}

func convertSSETranscript(t *httpclient.SSETranscript) *scripts.StreamInfo {
	if t == nil {
		return nil
	}
	info := &scripts.StreamInfo{Kind: "sse"}
	summary := map[string]interface{}{
		"eventCount": t.Summary.EventCount,
		"byteCount":  t.Summary.ByteCount,
		"duration":   t.Summary.Duration,
		"reason":     t.Summary.Reason,
	}
	info.Summary = summary
	if len(t.Events) > 0 {
		events := make([]map[string]interface{}, len(t.Events))
		for i, evt := range t.Events {
			events[i] = map[string]interface{}{
				"index":     evt.Index,
				"id":        evt.ID,
				"event":     evt.Event,
				"data":      evt.Data,
				"comment":   evt.Comment,
				"retry":     evt.Retry,
				"timestamp": evt.Timestamp.Format(time.RFC3339Nano),
			}
		}
		info.Events = events
	}
	return info
}

func convertWebSocketTranscript(t *httpclient.WebSocketTranscript) *scripts.StreamInfo {
	if t == nil {
		return nil
	}
	info := &scripts.StreamInfo{Kind: "websocket"}
	summary := map[string]interface{}{
		"sentCount":     t.Summary.SentCount,
		"receivedCount": t.Summary.ReceivedCount,
		"duration":      t.Summary.Duration,
		"closedBy":      t.Summary.ClosedBy,
		"closeCode":     t.Summary.CloseCode,
		"closeReason":   t.Summary.CloseReason,
	}
	info.Summary = summary
	if len(t.Events) > 0 {
		events := make([]map[string]interface{}, len(t.Events))
		for i, evt := range t.Events {
			events[i] = map[string]interface{}{
				"step":      evt.Step,
				"direction": evt.Direction,
				"type":      evt.Type,
				"size":      evt.Size,
				"text":      evt.Text,
				"base64":    evt.Base64,
				"code":      evt.Code,
				"reason":    evt.Reason,
				"timestamp": evt.Timestamp.Format(time.RFC3339Nano),
			}
		}
		info.Events = events
	}
	return info
}

func cloneStreamInfo(info *scripts.StreamInfo) *scripts.StreamInfo {
	if info == nil {
		return nil
	}
	return info.Clone()
}

func copyTranscript(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	return append([]byte(nil), data...)
}

func grpcStreamInfoFromSession(session *stream.Session) (*scripts.StreamInfo, []byte, error) {
	if session == nil {
		return nil, nil, nil
	}
	events := session.EventsSnapshot()
	stats := session.StatsSnapshot()
	state, err := session.State()
	info := &scripts.StreamInfo{
		Kind:    "grpc",
		Summary: make(map[string]interface{}),
	}

	counts := struct {
		sent     int
		received int
	}{}
	status := ""
	reason := ""
	info.Events = make([]map[string]interface{}, 0, len(events))

	for _, evt := range events {
		if evt == nil {
			continue
		}
		item := map[string]interface{}{
			"timestamp": evt.Timestamp.Format(time.RFC3339Nano),
		}
		if method := grpcMetaTrim(evt.Metadata, grpcclient.MetaMethod); method != "" {
			item["method"] = method
		}
		switch evt.Direction {
		case stream.DirSend:
			item["direction"] = "send"
			counts.sent++
		case stream.DirReceive:
			item["direction"] = "receive"
			counts.received++
		default:
			item["direction"] = "summary"
		}
		if typ := grpcMetaTrim(evt.Metadata, grpcclient.MetaMsgType); typ != "" {
			item["messageType"] = typ
		}
		if idxText := grpcMetaTrim(evt.Metadata, grpcclient.MetaMsgIndex); idxText != "" {
			item["index"] = idxText
			if idx, convErr := strconv.Atoi(idxText); convErr == nil {
				item["indexNum"] = idx
			}
		}
		if evt.Direction == stream.DirNA {
			if st := grpcMetaTrim(evt.Metadata, grpcclient.MetaStatus); st != "" {
				item["status"] = st
				status = st
			}
			if msg := grpcMetaTrim(evt.Metadata, grpcclient.MetaReason); msg != "" {
				item["reason"] = msg
				reason = msg
			}
		} else {
			text := strings.TrimSpace(string(evt.Payload))
			item["size"] = len(evt.Payload)
			item["text"] = text
			if len(evt.Payload) > 0 {
				var payload any
				if json.Unmarshal(evt.Payload, &payload) == nil {
					item["json"] = payload
				}
			}
		}
		info.Events = append(info.Events, item)
	}

	if status == "" {
		switch state {
		case stream.StateClosed:
			status = "OK"
		case stream.StateFailed:
			status = "FAILED"
		default:
			status = strings.ToUpper(strings.TrimSpace(streamStateString(state, err)))
		}
	}
	if reason == "" && err != nil {
		reason = err.Error()
	}

	dur := time.Duration(0)
	if !stats.EndedAt.IsZero() {
		dur = stats.EndedAt.Sub(stats.StartedAt)
	}
	info.Summary["sentCount"] = counts.sent
	info.Summary["receivedCount"] = counts.received
	info.Summary["eventCount"] = len(info.Events)
	info.Summary["duration"] = dur
	info.Summary["status"] = status
	info.Summary["reason"] = reason

	raw, encErr := json.MarshalIndent(map[string]any{
		"summary": info.Summary,
		"events":  info.Events,
	}, "", "  ")
	if encErr != nil {
		return nil, nil, encErr
	}
	return info, raw, nil
}

func grpcMetaTrim(md map[string]string, key string) string {
	if md == nil {
		return ""
	}
	return strings.TrimSpace(md[key])
}
