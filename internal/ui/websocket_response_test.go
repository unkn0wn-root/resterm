package ui

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
)

type wsTestTranscript struct {
	Events  []map[string]any `json:"events"`
	Summary map[string]any   `json:"summary"`
}

func TestBuildHTTPResponseViewsForWebSocket(t *testing.T) {
	transcript := wsTestTranscript{
		Events: []map[string]any{
			{
				"step":      "1:send_text",
				"direction": "send",
				"type":      "text",
				"size":      5,
				"text":      "hello",
				"timestamp": time.Now(),
			},
			{
				"direction": "receive",
				"type":      "text",
				"size":      5,
				"text":      "hello",
				"timestamp": time.Now(),
			},
		},
		Summary: map[string]any{
			"sentCount":     1,
			"receivedCount": 1,
			"duration":      time.Second,
			"closedBy":      "client",
			"closeCode":     1000,
			"closeReason":   "normal closure",
		},
	}

	body, err := json.Marshal(transcript)
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}

	resp := &httpclient.Response{
		Status:       "101 Switching Protocols",
		StatusCode:   http.StatusSwitchingProtocols,
		Proto:        "HTTP/1.1",
		Headers:      make(http.Header),
		Body:         body,
		Duration:     1500 * time.Millisecond,
		EffectiveURL: "ws://example.com/chat",
	}
	resp.Headers.Set("Content-Type", "application/json; charset=utf-8")
	resp.Headers.Set("X-Resterm-Stream-Type", "websocket")
	resp.Headers.Set("X-Resterm-Stream-Summary", "sent=1 recv=1 closed=client")

	pretty, raw, headers := buildHTTPResponseViews(resp, nil, nil)
	if pretty == "" || raw == "" || headers == "" {
		t.Fatalf("expected response views to be populated")
	}
}
