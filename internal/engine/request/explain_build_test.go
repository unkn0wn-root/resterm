package request

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
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
		ReqHost:        "api.internal",
	})

	if rep.Final == nil {
		t.Fatal("expected final explain section")
	}
	if got := previewHeaders(rep.Final.Headers).Get("Host"); got != "api.internal" {
		t.Fatalf("expected the sent Host, got %q", got)
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
	if got := explainPairValue(rep.Final.Details, "Proto File"); got != "" {
		t.Fatalf("expected no proto file detail for a descriptor set, got %q", got)
	}
}

func TestSetExplainPreparedListsProtoSources(t *testing.T) {
	t.Parallel()

	rep := &xplain.Report{}
	req := &restfile.Request{
		Method: "POST",
		GRPC: &restfile.GRPCRequest{
			Target:      "grpc.example:8443",
			FullMethod:  "/pkg.Service/Call",
			ProtoFiles:  []string{"proto/svc.proto", "proto/types.proto"},
			ImportPaths: []string{"proto", "vendor"},
		},
	}

	setExplainPrepared(rep, req, nil, nil, nil)
	if rep.Final == nil {
		t.Fatal("expected final explain section")
	}
	var files, imports []string
	for _, pair := range rep.Final.Details {
		switch pair.Key {
		case "Proto File":
			files = append(files, pair.Value)
		case "Import Path":
			imports = append(imports, pair.Value)
		}
	}
	if want := []string{"proto/svc.proto", "proto/types.proto"}; !slices.Equal(files, want) {
		t.Fatalf("proto file details = %q, want %q", files, want)
	}
	if want := []string{"proto", "vendor"}; !slices.Equal(imports, want) {
		t.Fatalf("import path details = %q, want %q", imports, want)
	}
	if got := explainPairValue(rep.Final.Details, "Descriptor Set"); got != "" {
		t.Fatalf("expected no descriptor set detail, got %q", got)
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

func TestPreviewShowsTheHostHeader(t *testing.T) {
	eng := New(engcfg.Config{}, nil)
	req := &restfile.Request{
		Method:  http.MethodGet,
		URL:     "http://example.test/x",
		Headers: http.Header{"Host": {"api.internal"}},
	}
	res, err := eng.ExecuteWith(nil, req, envWith(t, "dev", nil), ExecOptions{Mode: ExecModePreview})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() = %v, %v", err, res.Err)
	}
	if got := previewHeaders(res.Explain.Final.Headers).Values("Host"); !slices.Equal(got, []string{"api.internal"}) {
		t.Fatalf("preview Host = %q, want api.internal once", got)
	}
}

func TestPreviewExplainsGraphQLOnEveryTransport(t *testing.T) {
	gql := restfile.GraphQLBody{Query: "subscription { n }", OperationName: "N"}
	for _, tt := range []struct {
		name     string
		req      restfile.Request
		gql      *restfile.GraphQLBody
		env      map[string]string
		protocol string
		url      string
		body     string
		note     string
		details  []xplain.Pair
	}{
		{
			name:     "http get",
			req:      restfile.Request{Method: http.MethodGet, URL: "http://example.test/graphql"},
			protocol: "HTTP",
			url:      "http://example.test/graphql?operationName=N&query=subscription+%7B+n+%7D",
			note:     "graphql query encoded in URL",
			details:  []xplain.Pair{{Key: "GraphQL Operation", Value: "N"}},
		},
		{
			name:     "http post",
			req:      restfile.Request{Method: http.MethodPost, URL: "http://example.test/graphql"},
			protocol: "HTTP",
			url:      "http://example.test/graphql",
			body:     `{"operationName":"N","query":"subscription { n }"}`,
			note:     "graphql payload",
			details:  []xplain.Pair{{Key: "GraphQL Operation", Value: "N"}},
		},
		{
			name: "sse",
			req: restfile.Request{
				Method: http.MethodPost,
				URL:    "http://example.test/graphql",
				SSE:    &restfile.SSERequest{Options: restfile.SSEOptions{IdleTimeout: time.Second}},
			},
			protocol: "SSE",
			url:      "http://example.test/graphql",
			body:     `{"operationName":"N","query":"subscription { n }"}`,
			note:     "graphql payload",
			details:  []xplain.Pair{{Key: "Idle Timeout", Value: "1s"}, {Key: "GraphQL Operation", Value: "N"}},
		},
		{
			name: "websocket",
			req: restfile.Request{
				Method: http.MethodGet,
				URL:    "ws://example.test/graphql",
				WebSocket: &restfile.WebSocketRequest{
					Options: restfile.WebSocketOptions{IdleTimeout: time.Second},
				},
			},
			protocol: "WebSocket",
			url:      "ws://example.test/graphql",
			body:     `{"id":"1","type":"subscribe","payload":{"operationName":"N","query":"subscription { n }"}}`,
			note:     "graphql-transport-ws subscribe message",
			details: []xplain.Pair{
				{Key: "Idle Timeout", Value: "1s"},
				{Key: "Subprotocols", Value: "graphql-transport-ws"},
				{Key: "GraphQL Operation", Value: "N"},
			},
		},
		{
			name: "websocket with variables",
			req: restfile.Request{
				Method:    http.MethodGet,
				URL:       "ws://example.test/graphql",
				WebSocket: &restfile.WebSocketRequest{},
			},
			gql: &restfile.GraphQLBody{
				Query:     "subscription($from: Int) { n(from: $from) }",
				Variables: `{"from": {{from}}}`,
			},
			env:      map[string]string{"from": "3"},
			protocol: "WebSocket",
			url:      "ws://example.test/graphql",
			body:     `{"id":"1","type":"subscribe","payload":{"query":"subscription($from: Int) { n(from: $from) }","variables":{"from":3}}}`,
			note:     "graphql-transport-ws subscribe message",
			details:  []xplain.Pair{{Key: "Subprotocols", Value: "graphql-transport-ws"}},
		},
		{
			name: "websocket with a secret",
			req: restfile.Request{
				Method:    http.MethodGet,
				URL:       "ws://example.test/graphql",
				WebSocket: &restfile.WebSocketRequest{},
				Variables: []restfile.Variable{{Name: "token", Value: "top-secret", Secret: true}},
			},
			gql:      &restfile.GraphQLBody{Query: "subscription { n }", Variables: `{"token": "{{token}}"}`},
			protocol: "WebSocket",
			url:      "ws://example.test/graphql",
			body:     `{"id":"1","type":"subscribe","payload":{"query":"subscription { n }","variables":{"token":"•••"}}}`,
			note:     "graphql-transport-ws subscribe message",
			details:  []xplain.Pair{{Key: "Subprotocols", Value: "graphql-transport-ws"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			req.Body.GraphQL = cmp.Or(tt.gql, &gql)
			res, err := New(engcfg.Config{}, nil).ExecuteWith(
				nil,
				&req,
				envWith(t, "dev", tt.env),
				ExecOptions{Mode: ExecModePreview},
			)
			if err != nil || res.Err != nil {
				t.Fatalf("ExecuteWith() = %v, %v", err, res.Err)
			}
			if out, _ := json.Marshal(res.Explain); bytes.Contains(out, []byte("top-secret")) {
				t.Fatalf("explain report shows a secret: %s", out)
			}
			final := res.Explain.Final
			got := fmt.Sprintf("%s %s body=%s note=%q", final.Protocol, final.URL, final.Body, final.BodyNote)
			want := fmt.Sprintf("%s %s body=%s note=%q", tt.protocol, tt.url, tt.body, tt.note)
			if got != want {
				t.Fatalf("final = %s, want %s", got, want)
			}
			if !slices.Equal(final.Details, tt.details) {
				t.Fatalf("details = %v, want %v", final.Details, tt.details)
			}
			var changes []xplain.Change
			for _, st := range res.Explain.Stages {
				if st.Name != xplain.StageHTTPPrepare {
					continue
				}
				for _, c := range st.Changes {
					if c.Field == "body" || c.Field == "body.note" {
						changes = append(changes, c)
					}
				}
			}
			if len(changes) == 0 {
				t.Fatal("http prepare stage shows no body change, want the built body")
			}
		})
	}
}

func TestPreviewDoesNotDialAGraphQLWebSocket(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)
	req := restfile.Request{
		Method:    http.MethodGet,
		URL:       "ws" + strings.TrimPrefix(srv.URL, "http"),
		WebSocket: &restfile.WebSocketRequest{},
		Body:      restfile.BodySource{GraphQL: &restfile.GraphQLBody{Query: "subscription { n }"}},
	}
	res, err := New(engcfg.Config{}, nil).ExecuteWith(
		nil,
		&req,
		envWith(t, "dev", nil),
		ExecOptions{Mode: ExecModePreview},
	)
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() = %v, %v", err, res.Err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("preview reached the server %d times, want none", n)
	}
}
