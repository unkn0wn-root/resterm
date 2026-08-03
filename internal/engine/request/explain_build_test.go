package request

import (
	"net/http"
	"strings"
	"testing"
	"time"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestSetExplainHTTPExtendsPreparedReport(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{}
	req := &restfile.Request{
		Method:   "POST",
		URL:      "https://example.com/source",
		Headers:  http.Header{"X-Req": {"1"}},
		Settings: map[string]string{"timeout": "5s"},
		Body:     restfile.BodySource{Text: `{"ok":true}`},
	}

	setExplainPrepared(rep, req, req.Settings, nil, nil)
	setExplainHTTP(rep, &httpx.Response{
		ReqMethod:      "POST",
		EffectiveURL:   "https://example.com/final",
		RequestHeaders: http.Header{"X-Sent": {"2"}},
	})

	if rep.Final == nil {
		t.Fatal("expected final explain section")
	}
	if rep.Final.Mode != "sent" {
		t.Fatalf("expected sent mode, got %q", rep.Final.Mode)
	}
	if rep.Final.URL != "https://example.com/final" {
		t.Fatalf("expected effective URL, got %q", rep.Final.URL)
	}
	if len(rep.Final.Settings) != 1 || rep.Final.Settings[0].Key != "timeout" {
		t.Fatalf("expected prepared settings to survive HTTP finalize, got %#v", rep.Final.Settings)
	}
	if rep.Final.Body == "" {
		t.Fatal("expected prepared body to survive HTTP finalize")
	}
}

func TestSetExplainPreparedCapturesGRPCDetails(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{}
	req := &restfile.Request{
		Method: "POST",
		GRPC: &restfile.GRPCRequest{
			Target:          "dns:///grpc.example:8443",
			FullMethod:      "/pkg.Service/Call",
			DescriptorSet:   "api.pb",
			Plaintext:       restfile.OptOf(true),
			Authority:       "grpc.example",
			MessageFile:     "req.json",
			MessageExpanded: restfile.OptOf(`{"ok":true}`),
			Metadata: []restfile.MetadataPair{
				{Key: "x-trace-id", Value: "abc123"},
			},
		},
	}

	setExplainPrepared(rep, req, nil, nil, nil)
	if rep.Final == nil {
		t.Fatal("expected final explain section")
	}
	if rep.Final.Protocol != "gRPC" {
		t.Fatalf("expected gRPC protocol, got %q", rep.Final.Protocol)
	}
	if rep.Final.Body != `{"ok":true}` {
		t.Fatalf("expected expanded gRPC body, got %q", rep.Final.Body)
	}
	if got := rep.Final.BodyNote; !strings.Contains(got, "expanded gRPC message from req.json") {
		t.Fatalf("expected expanded body note, got %q", got)
	}
	if got := explainPairValue(rep.Final.Details, "RPC"); got != "pkg.Service/Call" {
		t.Fatalf("expected RPC detail, got %q", got)
	}
	if got := explainPairValue(rep.Final.Details, "Transport"); got != "plaintext" {
		t.Fatalf("expected transport detail, got %q", got)
	}
	if got := explainPairValue(rep.Final.Details, "Reflection"); got != "disabled" {
		t.Fatalf("expected reflection detail, got %q", got)
	}
	if got := explainPairValue(rep.Final.Details, "Metadata"); got != "x-trace-id: abc123" {
		t.Fatalf("expected metadata detail, got %q", got)
	}
}

func TestSetExplainPreparedCapturesWebSocketSteps(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{}
	req := &restfile.Request{
		Method: "GET",
		URL:    "wss://example.com/ws",
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{
				HandshakeTimeout: 3 * time.Second,
				IdleTimeout:      30 * time.Second,
				Subprotocols:     []string{"chat", "events"},
				Compression:      restfile.OptOf(true),
			},
			Steps: []restfile.WebSocketStep{
				{Type: restfile.WebSocketStepSendJSON, Value: `{"ping":true}`},
				{Type: restfile.WebSocketStepWait, Duration: 2 * time.Second},
				{Type: restfile.WebSocketStepClose, Code: 1000, Reason: "done"},
			},
		},
	}

	setExplainPrepared(rep, req, nil, nil, nil)
	if rep.Final == nil {
		t.Fatal("expected final explain section")
	}
	if rep.Final.Protocol != "WebSocket" {
		t.Fatalf("expected WebSocket protocol, got %q", rep.Final.Protocol)
	}
	if got := explainPairValue(rep.Final.Details, "Subprotocols"); got != "chat, events" {
		t.Fatalf("expected subprotocol detail, got %q", got)
	}
	if got := explainPairValue(rep.Final.Details, "Compression"); got != "enabled" {
		t.Fatalf("expected compression detail, got %q", got)
	}
	if len(rep.Final.Steps) != 3 {
		t.Fatalf("expected WebSocket steps, got %#v", rep.Final.Steps)
	}
	if rep.Final.Steps[0] != `Send JSON {"ping":true}` {
		t.Fatalf("unexpected first WebSocket step %q", rep.Final.Steps[0])
	}
}

func explainPairValue(xs []xplain.Pair, key string) string {
	for _, x := range xs {
		if x.Key == key {
			return x.Value
		}
	}
	return ""
}
