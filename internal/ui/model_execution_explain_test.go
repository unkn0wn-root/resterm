package ui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestSetExplainHTTPExtendsPreparedReport(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{}
	req := &restfile.Request{
		Method:   "POST",
		URL:      "https://example.com/source",
		Headers:  http.Header{"X-Req": []string{"1"}},
		Settings: map[string]string{"timeout": "5s"},
		Body:     restfile.BodySource{Text: `{"ok":true}`},
	}

	setExplainPrepared(rep, req, req.Settings, nil, nil)
	setExplainHTTP(rep, &httpclient.Response{
		ReqMethod:      "POST",
		EffectiveURL:   "https://example.com/final",
		RequestHeaders: http.Header{"X-Sent": []string{"2"}},
	})

	if rep.Final == nil {
		t.Fatalf("expected final explain section")
	}
	if rep.Final.Mode != "sent" {
		t.Fatalf("expected sent mode, got %q", rep.Final.Mode)
	}
	if rep.Final.URL != "https://example.com/final" {
		t.Fatalf("expected effective url, got %q", rep.Final.URL)
	}
	if len(rep.Final.Settings) != 1 || rep.Final.Settings[0].Key != "timeout" {
		t.Fatalf("expected prepared settings to survive http finalize, got %#v", rep.Final.Settings)
	}
	if rep.Final.Body == "" {
		t.Fatalf("expected prepared body to survive http finalize")
	}
}

func TestExecuteRequestConflictReturnsExplainReport(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		SSH:    &restfile.SSHSpec{},
		K8s:    &restfile.K8sSpec{},
	}

	msg := model.executeRequest(nil, req, httpclient.Options{}, "", nil)()
	res, ok := msg.(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg, got %T", msg)
	}
	if res.err == nil {
		t.Fatalf("expected conflict error")
	}
	if res.explain == nil {
		t.Fatalf("expected explain report on route conflict")
	}
	if res.explain.Status != xplain.StatusError {
		t.Fatalf("expected explain error status, got %q", res.explain.Status)
	}
	if len(res.explain.Stages) == 0 || res.explain.Stages[0].Name != "route" {
		t.Fatalf("expected route stage, got %#v", res.explain.Stages)
	}
}

func TestExecuteExplainReturnsPreviewWithoutSending(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.client = nil

	req := &restfile.Request{
		Method: "GET",
		URL:    "{{host}}/api",
		Variables: []restfile.Variable{
			{Name: "host", Value: "https://example.com"},
		},
	}

	msg := model.executeExplain(nil, req, httpclient.Options{}, "", nil)()
	res, ok := msg.(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg, got %T", msg)
	}
	if res.err != nil {
		t.Fatalf("expected no preview error, got %v", res.err)
	}
	if !res.preview {
		t.Fatal("expected preview response")
	}
	if res.explain == nil {
		t.Fatal("expected explain report")
	}
	if res.explain.Status != xplain.StatusReady {
		t.Fatalf("expected ready explain status, got %q", res.explain.Status)
	}
	if res.explain.Final == nil {
		t.Fatal("expected final explain snapshot")
	}
	if res.explain.Final.Mode != "prepared" {
		t.Fatalf("expected prepared mode, got %q", res.explain.Final.Mode)
	}
	if res.explain.Final.Protocol != "HTTP" {
		t.Fatalf("expected HTTP protocol, got %q", res.explain.Final.Protocol)
	}
	if res.explain.Final.URL != "https://example.com/api" {
		t.Fatalf("expected expanded preview url, got %q", res.explain.Final.URL)
	}
}

func TestHandleResponseMessagePreviewOpensExplainTab(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.handleResponseMessage(responseMsg{
		preview:     true,
		environment: "dev",
		explain: &xplain.Report{
			Status:   xplain.StatusReady,
			Decision: "Explain preview ready. No request was sent.",
		},
	})
	pane := model.pane(responsePanePrimary)
	if pane == nil || pane.snapshot == nil {
		t.Fatal("expected preview snapshot on primary pane")
	}
	if pane.activeTab != responseTabExplain {
		t.Fatalf("expected explain tab to be active, got %v", pane.activeTab)
	}
	if pane.snapshot.explain.report == nil {
		t.Fatal("expected explain report on preview snapshot")
	}
}

func TestSetExplainPreparedCapturesGRPCDetails(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{}
	req := &restfile.Request{
		Method: "POST",
		GRPC: &restfile.GRPCRequest{
			Target:             "dns:///grpc.example:8443",
			FullMethod:         "/pkg.Service/Call",
			DescriptorSet:      "api.pb",
			UseReflection:      false,
			Plaintext:          true,
			PlaintextSet:       true,
			Authority:          "grpc.example",
			MessageFile:        "req.json",
			MessageExpanded:    `{"ok":true}`,
			MessageExpandedSet: true,
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
		t.Fatalf("expected expanded grpc body, got %q", rep.Final.Body)
	}
	if got := rep.Final.BodyNote; !strings.Contains(got, "expanded gRPC message from req.json") {
		t.Fatalf("expected expanded body note, got %q", got)
	}
	if got := explainPairValue(rep.Final.Details, "RPC"); got != "pkg.Service/Call" {
		t.Fatalf("expected rpc detail, got %q", got)
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
				Compression:      true,
				CompressionSet:   true,
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
		t.Fatalf("expected websocket protocol, got %q", rep.Final.Protocol)
	}
	if got := explainPairValue(rep.Final.Details, "Subprotocols"); got != "chat, events" {
		t.Fatalf("expected subprotocol detail, got %q", got)
	}
	if got := explainPairValue(rep.Final.Details, "Compression"); got != "enabled" {
		t.Fatalf("expected compression detail, got %q", got)
	}
	if len(rep.Final.Steps) != 3 {
		t.Fatalf("expected websocket steps, got %#v", rep.Final.Steps)
	}
	if rep.Final.Steps[0] != `Send JSON {"ping":true}` {
		t.Fatalf("unexpected first websocket step %q", rep.Final.Steps[0])
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
