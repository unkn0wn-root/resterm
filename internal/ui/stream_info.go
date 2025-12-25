package ui

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
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
