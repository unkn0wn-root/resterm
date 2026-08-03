package httpx

import (
	"net/http"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestStreamingWebSocketResponse(t *testing.T) {
	req := &restfile.Request{Method: http.MethodGet}
	resp := StreamingWebSocketResponse(StreamMeta{
		Headers:        http.Header{"X-Test": {"one"}},
		RequestHeaders: http.Header{"X-Request": {"two"}},
		Request:        req,
	})

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}
	if got := resp.Headers.Get(StreamHeaderType); got != "websocket" {
		t.Fatalf("%s = %q, want websocket", StreamHeaderType, got)
	}
	if got := resp.Headers.Get(StreamHeaderSummary); got != "streaming" {
		t.Fatalf("%s = %q, want streaming", StreamHeaderSummary, got)
	}
	resp.Headers.Set("X-Test", "changed")
	resp.RequestHeaders.Set("X-Request", "changed")
	if got := resp.Request; got != req {
		t.Fatalf("Request = %p, want %p", got, req)
	}
}
