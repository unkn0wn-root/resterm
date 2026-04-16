package ui

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
	"google.golang.org/grpc/codes"
	"nhooyr.io/websocket"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func startUIWebSocketServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := httptest.NewUnstartedServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
			if err != nil {
				t.Fatalf("websocket accept failed: %v", err)
			}
			defer func() {
				if err := conn.Close(websocket.StatusNormalClosure, "bye"); err != nil {
					t.Logf("close websocket: %v", err)
				}
			}()
			<-r.Context().Done()
		}),
	)
	srv.Listener = ln
	srv.Start()

	return srv, func() {
		srv.Close()
	}
}

func TestPrepareGRPCRequestExpandsTemplKeepMsg(t *testing.T) {
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{
		"userId": "123",
		"token":  "abcd",
	}))

	req := &restfile.Request{
		Method: "GRPC",
		Body:   restfile.BodySource{Text: "{\"id\":\"{{userId}}\"}"},
		GRPC: &restfile.GRPCRequest{
			Target:     " localhost:50051 ",
			FullMethod: "/pkg.Service/GetUser",
			Message:    "{\"id\":\"{{userId}}\"}",
			Metadata: []restfile.MetadataPair{
				{Key: "authorization", Value: "Bearer {{token}}"},
			},
		},
	}

	var model Model
	if err := model.prepareGRPCRequest(req, resolver, ""); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}

	if req.URL != "localhost:50051" {
		t.Fatalf("expected URL to be trimmed target, got %q", req.URL)
	}
	if strings.Contains(req.GRPC.Message, "{{") {
		t.Fatalf("expected message templates to be expanded, got %q", req.GRPC.Message)
	}
	if req.GRPC.MessageFile != "" {
		t.Fatalf("expected message file to be cleared when inline body provided")
	}
	want := "Bearer abcd"
	found := false
	for _, pair := range req.GRPC.Metadata {
		if pair.Key == "authorization" && pair.Value == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected metadata to be expanded to %q", want)
	}
}

func TestInlineRequestFromLineURL(t *testing.T) {
	req := inlineRequestFromLine(" https://example.com/v1/users ", 3)
	if req == nil {
		t.Fatalf("expected inline request to be created")
	}
	if req.Method != "GET" {
		t.Fatalf("expected default method GET, got %q", req.Method)
	}
	if req.URL != "https://example.com/v1/users" {
		t.Fatalf("expected URL to be trimmed, got %q", req.URL)
	}
	if req.LineRange.Start != 3 || req.LineRange.End != 3 {
		t.Fatalf("expected line range to be set to cursor line")
	}
}

func TestInlineRequestFromLineWithMethod(t *testing.T) {
	req := inlineRequestFromLine("POST https://api.example.com/data", 5)
	if req == nil {
		t.Fatalf("expected inline request to be created")
	}
	if req.Method != "POST" {
		t.Fatalf("expected method POST, got %q", req.Method)
	}
	if req.URL != "https://api.example.com/data" {
		t.Fatalf("unexpected url %q", req.URL)
	}
}

func TestInlineRequestFromLineRejectsInvalid(t *testing.T) {
	req := inlineRequestFromLine("example.com", 2)
	if req != nil {
		t.Fatalf("expected non-http line to be ignored")
	}
}

func TestRequestAtCursorBeforeRequestsReturnsNil(t *testing.T) {
	content := "# preface\n\n### first\nGET https://example.com/one\n"
	doc := parser.Parse("sample.http", []byte(content))
	var model Model

	req, inline := model.requestAtCursor(doc, content, 1)
	if req != nil || inline {
		t.Fatalf(
			"expected no request at cursor before first request, got req=%v inline=%v",
			req,
			inline,
		)
	}
}

func TestRequestAtCursorFallsBackToLastRequest(t *testing.T) {
	content := "### first\nGET https://example.com/one\n\n### second\nGET https://example.com/two\n\n"
	doc := parser.Parse("sample.http", []byte(content))
	var model Model

	req, inline := model.requestAtCursor(doc, content, 6)
	if inline {
		t.Fatalf("expected document request, not inline")
	}
	if req == nil || strings.TrimSpace(req.URL) != "https://example.com/two" {
		t.Fatalf("expected last request when cursor after requests, got %+v", req)
	}
}

func TestInlineRequestStripsHTTPVersionToken(t *testing.T) {
	content := "http://example.com HTTP/1.1"
	req := buildInlineRequest(content, 1)
	if req == nil {
		t.Fatalf("expected inline request to be parsed")
	}
	if req.Method != "GET" || req.URL != "http://example.com" {
		t.Fatalf("unexpected request %s %s", req.Method, req.URL)
	}
	if req.Settings["http-version"] != "1.1" {
		t.Fatalf("expected http-version=1.1, got %q", req.Settings["http-version"])
	}
}

func TestInlineCurlRequestSingleLine(t *testing.T) {
	content := "curl https://example.com"
	req := buildInlineRequest(content, 1)
	if req == nil {
		t.Fatalf("expected curl request to be parsed")
	}
	if req.Method != "GET" || req.URL != "https://example.com" {
		t.Fatalf("unexpected request %s %s", req.Method, req.URL)
	}
	if req.LineRange.Start != 1 || req.LineRange.End != 1 {
		t.Fatalf("expected single line range, got %+v", req.LineRange)
	}
}

func TestInlineCurlRequestMultiline(t *testing.T) {
	content := `curl https://api.example.com/users \
-H 'Content-Type: application/json' \
--data '{"name":"Sam"}'`
	req := buildInlineRequest(content, 2)
	if req == nil {
		t.Fatalf("expected curl request to be parsed")
	}
	if req.Method != "POST" {
		t.Fatalf("expected POST from curl data, got %s", req.Method)
	}
	if req.Headers.Get("Content-Type") != "application/json" {
		t.Fatalf("expected content-type header")
	}
	if req.Body.Text != "{\"name\":\"Sam\"}" {
		t.Fatalf("unexpected body %q", req.Body.Text)
	}
	if req.LineRange.Start != 1 || req.LineRange.End != 3 {
		t.Fatalf("expected multi-line range, got %+v", req.LineRange)
	}
}

func TestPrepareGRPCRequestUsesBodyOverride(t *testing.T) {
	resolver := vars.NewResolver()
	req := &restfile.Request{
		Method: "GRPC",
		Body:   restfile.BodySource{Text: "{\"name\":\"sam\"}"},
		GRPC: &restfile.GRPCRequest{
			Target:  "localhost:50051",
			Service: "UserService",
			Method:  "Create",
		},
	}

	var model Model
	if err := model.prepareGRPCRequest(req, resolver, ""); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.FullMethod != "/UserService/Create" {
		t.Fatalf("expected full method to be inferred, got %q", req.GRPC.FullMethod)
	}
	if req.GRPC.Message != "{\"name\":\"sam\"}" {
		t.Fatalf("expected body override to populate grpc message, got %q", req.GRPC.Message)
	}
}

func TestPrepareGRPCRequestNormalizesSchemedTarget(t *testing.T) {
	resolver := vars.NewResolver()
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			Target:     "grpc://localhost:8082",
			FullMethod: "/pkg.Service/Call",
		},
	}

	var model Model
	if err := model.prepareGRPCRequest(req, resolver, ""); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.Target != "localhost:8082" {
		t.Fatalf("expected target to be normalized, got %q", req.GRPC.Target)
	}
	if req.URL != "localhost:8082" {
		t.Fatalf("expected URL to mirror normalized target, got %q", req.URL)
	}
}

func TestPrepareGRPCRequestNormalizesSecureSchemes(t *testing.T) {
	resolver := vars.NewResolver()
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			Target:     "grpcs://api.example.com:8443",
			FullMethod: "/pkg.Service/Call",
		},
	}

	var model Model
	if err := model.prepareGRPCRequest(req, resolver, ""); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.Target != "api.example.com:8443" {
		t.Fatalf("expected target to drop grpcs scheme, got %q", req.GRPC.Target)
	}
	if !req.GRPC.PlaintextSet || req.GRPC.Plaintext {
		t.Fatalf(
			"expected secure scheme to enforce TLS, got plaintext=%v set=%v",
			req.GRPC.Plaintext,
			req.GRPC.PlaintextSet,
		)
	}
}

func TestNormalizeGRPCTargetPreservesQuery(t *testing.T) {
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			Target:     "grpc://localhost:9000/service?alt=blue",
			FullMethod: "/svc.Method",
		},
	}

	var model Model
	if err := model.prepareGRPCRequest(req, vars.NewResolver(), ""); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.Target != "localhost:9000/service?alt=blue" {
		t.Fatalf("expected query to be preserved, got %q", req.GRPC.Target)
	}
}

func TestPrepareGRPCRequestExpandsDescriptorSet(t *testing.T) {
	resolver := vars.NewResolver(
		vars.NewMapProvider(
			"doc",
			map[string]string{"grpc.descriptor": "./testdata/example.protoset"},
		),
	)
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			Target:        "localhost:50051",
			FullMethod:    "/pkg.Svc/Call",
			DescriptorSet: "{{grpc.descriptor}}",
		},
	}

	var model Model
	if err := model.prepareGRPCRequest(req, resolver, ""); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.DescriptorSet != "./testdata/example.protoset" {
		t.Fatalf("expected descriptor set to be expanded, got %q", req.GRPC.DescriptorSet)
	}
}

func TestPrepareGRPCRequestExpandsMessageFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "msg.json")
	if err := os.WriteFile(path, []byte(`{"id":"{{userId}}"}`), 0o600); err != nil {
		t.Fatalf("write message file: %v", err)
	}

	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{
		"userId": "abc",
	}))
	req := &restfile.Request{
		Method: "GRPC",
		Body: restfile.BodySource{
			FilePath: "msg.json",
			Options:  restfile.BodyOptions{ExpandTemplates: true},
		},
		GRPC: &restfile.GRPCRequest{
			Target:     "localhost:50051",
			FullMethod: "/pkg.Service/Get",
		},
	}

	var model Model
	if err := model.prepareGRPCRequest(req, resolver, dir); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.MessageFile != "msg.json" {
		t.Fatalf("expected message file to be preserved, got %q", req.GRPC.MessageFile)
	}
	if req.GRPC.Message != "" {
		t.Fatalf("expected inline message to stay empty, got %q", req.GRPC.Message)
	}
	if !req.GRPC.MessageExpandedSet {
		t.Fatalf("expected expanded message to be set")
	}
	if req.GRPC.MessageExpanded != `{"id":"abc"}` {
		t.Fatalf("expected expanded message, got %q", req.GRPC.MessageExpanded)
	}
}

func TestHandleResponseMsgShowsGrpcErrors(t *testing.T) {
	model := New(Config{})
	model.ready = true
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			FullMethod: "/pkg.Service/Missing",
		},
	}
	resp := &grpcclient.Response{
		StatusCode:    codes.NotFound,
		StatusMessage: "not found",
		Message:       "{}",
	}
	err := errdef.New(errdef.CodeHTTP, "invoke grpc method")

	model.handleResponseMessage(responseMsg{
		grpc:     resp,
		err:      err,
		executed: req,
	})

	if model.lastGRPC != resp {
		t.Fatalf("expected lastGRPC to be set")
	}
	if model.lastResponse != nil {
		t.Fatalf("expected lastResponse to be cleared for grpc errors")
	}
	if model.statusMessage.level != statusWarn {
		t.Fatalf("expected warning status for non-OK grpc code, got %v", model.statusMessage.level)
	}
	if model.lastError != err {
		t.Fatalf("expected lastError to retain grpc invoke err")
	}
	if model.responseLatest == nil || !strings.Contains(model.responseLatest.pretty, "NotFound") {
		var got string
		if model.responseLatest != nil {
			got = model.responseLatest.pretty
		}
		t.Fatalf("expected response view to mention grpc status, got %q", got)
	}
}

func TestConsumeGRPCResponseUsesBinaryBody(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	wire := []byte{0x00, 0x01, 0x02, 0x03}
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			FullMethod: "/pkg.Service/Binary",
		},
	}
	resp := &grpcclient.Response{
		StatusCode:      codes.OK,
		StatusMessage:   "OK",
		Message:         `{"ok":true}`,
		Body:            []byte(`{"ok":true}`),
		Wire:            wire,
		ContentType:     "application/json",
		WireContentType: "application/grpc+proto",
	}

	cmd := model.consumeGRPCResponse(resp, nil, nil, req, "", nil)
	if cmd != nil {
		collectMsgs(cmd)
	}

	snap := model.responseLatest
	if snap == nil || !snap.ready {
		t.Fatalf("expected response snapshot to be ready")
	}
	if snap.bodyMeta.Kind != binaryview.KindText {
		t.Fatalf("expected meta kind to allow text view, got %v", snap.bodyMeta.Kind)
	}
	if snap.rawMode != rawViewText {
		t.Fatalf("expected raw mode to default to text for gRPC message, got %v", snap.rawMode)
	}
	if snap.rawHex == "" {
		t.Fatalf("expected hex dump to remain available")
	}
	if !strings.Contains(snap.raw, "{") {
		t.Fatalf("expected raw view to show json message, got %q", snap.raw)
	}
}

func TestHandleResponseMsgShowsHTTPErrorInPane(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	err := errdef.New(errdef.CodeHTTP, "send request failed")
	cmd := model.handleResponseMessage(responseMsg{err: err})
	if cmd != nil {
		collectMsgs(cmd)
	}

	if model.showErrorModal {
		t.Fatalf("expected error modal to stay closed for request errors")
	}
	if model.responseLatest == nil || !model.responseLatest.ready {
		t.Fatalf("expected latest snapshot to be ready")
	}
	if !strings.Contains(model.responseLatest.pretty, "send request failed") {
		t.Fatalf("expected pretty view to include error text, got %q", model.responseLatest.pretty)
	}
	viewport := model.pane(responsePanePrimary).viewport.View()
	if !strings.Contains(viewport, "send request failed") {
		t.Fatalf("expected viewport to include error details, got %q", viewport)
	}
	if model.statusMessage.level != statusError {
		t.Fatalf("expected status message to record error, got %v", model.statusMessage.level)
	}
	if model.suppressNextErrorModal {
		t.Fatalf("expected suppress flag to reset after status update")
	}
}

func TestHandleResponseMsgShowsScriptErrorInPane(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 100
	model.height = 30
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	err := errdef.Wrap(errdef.CodeScript, errors.New("boom"), "pre-request script")
	cmd := model.handleResponseMessage(responseMsg{err: err})
	if cmd != nil {
		collectMsgs(cmd)
	}

	if model.showErrorModal {
		t.Fatalf("expected error modal to stay closed for script errors")
	}
	if model.statusMessage.level != statusWarn {
		t.Fatalf("expected script errors to show warning status, got %v", model.statusMessage.level)
	}
	if model.responseLatest == nil ||
		!strings.Contains(model.responseLatest.pretty, "pre-request script") {
		var pretty string
		if model.responseLatest != nil {
			pretty = model.responseLatest.pretty
		}
		t.Fatalf("expected pretty view to mention script failure, got %q", pretty)
	}
	viewport := model.pane(responsePanePrimary).viewport.View()
	if !strings.Contains(viewport, "pre-request script") {
		t.Fatalf("expected viewport to include script error details, got %q", viewport)
	}
	if model.suppressNextErrorModal {
		t.Fatalf("expected suppress flag to reset after script error")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExecuteRequestRunsScriptsForSSE(t *testing.T) {
	fakeClient := httpclient.NewClient(nil)
	fakeClient.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		transport := transportFunc(func(req *http.Request) (*http.Response, error) {
			reader, writer := io.Pipe()
			go func() {
				defer func() {
					if err := writer.Close(); err != nil {
						t.Logf("close writer: %v", err)
					}
				}()
				_, _ = io.WriteString(writer, "data: hello\n\n")
			}()
			resp := &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Header:     make(http.Header),
				Body:       reader,
				Request:    req,
			}
			resp.Header.Set("Content-Type", "text/event-stream")
			return resp, nil
		})
		return &http.Client{Transport: transport}, nil
	})

	model := New(Config{Client: fakeClient})
	doc := &restfile.Document{}
	model.doc = doc

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/events",
		SSE:    &restfile.SSERequest{},
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "stream.count",
				Expression: "{{response.json.summary.eventCount}}",
			}},
			Scripts: []restfile.ScriptBlock{{
				Kind: "test",
				Body: `{% tests.assert(response.json().summary.eventCount === 1, "event count"); %}`,
			}},
		},
	}
	doc.Requests = []*restfile.Request{req}

	cmd := model.executeRequest(doc, req, model.cfg.HTTPOptions, "", nil)
	if cmd == nil {
		t.Fatalf("expected executeRequest to return command")
	}

	msg, ok := cmd().(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg from command")
	}
	if msg.err != nil {
		t.Fatalf("unexpected error from executeRequest: %v", msg.err)
	}
	if msg.response == nil {
		t.Fatalf("expected response in message")
	}
	if msg.scriptErr != nil {
		t.Logf("response body: %s", string(msg.response.Body))
		t.Fatalf("unexpected script error: %v", msg.scriptErr)
	}
	if len(msg.tests) != 1 {
		t.Fatalf("expected one test result, got %d", len(msg.tests))
	}
	if !msg.tests[0].Passed {
		t.Fatalf("expected test to pass, got %+v", msg.tests[0])
	}
	found := false
	for _, v := range msg.executed.Variables {
		if v.Name == "stream.count" && v.Value == "1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected capture to populate request variable, got %+v", msg.executed.Variables)
	}
}

func TestExecuteRequestRTSGlobalMutationPreservesRequestVarPrecedenceForJS(t *testing.T) {
	model := New(Config{EnvironmentName: "dev"})
	model.globalsStore().Set("dev", "token", "global-token", false)

	var seenHeader string
	model.client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenHeader = req.Header.Get("X-Seen")
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}, nil
		})
		return &http.Client{Transport: transport}, nil
	})

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Variables: []restfile.Variable{
			{Name: "token", Value: "request-token"},
		},
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{
				{
					Kind: "pre-request",
					Lang: "rts",
					Body: `vars.global.set("other", "1", false)`,
				},
				{
					Kind: "pre-request",
					Body: `request.setHeader("X-Seen", vars.get("token"));`,
				},
			},
		},
	}

	msg, ok := model.executeRequest(nil, req, httpclient.Options{}, "", nil)().(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg")
	}
	if msg.err != nil {
		t.Fatalf("unexpected error from executeRequest: %v", msg.err)
	}
	if got := seenHeader; got != "request-token" {
		t.Fatalf("expected request var to win over global, got %q", got)
	}
}

func TestExecuteExplainRTSGlobalMutationPreservesRequestVarPrecedenceForJS(t *testing.T) {
	model := New(Config{EnvironmentName: "dev"})
	model.globalsStore().Set("dev", "token", "global-token", false)

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Variables: []restfile.Variable{
			{Name: "token", Value: "request-token"},
		},
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{
				{
					Kind: "pre-request",
					Lang: "rts",
					Body: `vars.global.set("other", "1", false)`,
				},
				{
					Kind: "pre-request",
					Body: `request.setHeader("X-Seen", vars.get("token"));`,
				},
			},
		},
	}

	msg, ok := model.executeExplain(nil, req, httpclient.Options{}, "", nil)().(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg")
	}
	if msg.err != nil {
		t.Fatalf("unexpected error from executeExplain: %v", msg.err)
	}
	if !msg.preview {
		t.Fatalf("expected preview response")
	}
	if got := msg.executed.Headers.Get("X-Seen"); got != "request-token" {
		t.Fatalf("expected preview request var to win over global, got %q", got)
	}
	if msg.explain == nil || msg.explain.Final == nil {
		t.Fatalf("expected explain report with final request")
	}
	found := false
	for _, header := range msg.explain.Final.Headers {
		if header.Name == "X-Seen" {
			found = true
			if header.Value != "request-token" {
				t.Fatalf("expected explain header to use request var, got %q", header.Value)
			}
		}
	}
	if !found {
		t.Fatalf("expected explain preview to include X-Seen header")
	}
}

func TestResolveRequestTimeout(t *testing.T) {
	req := &restfile.Request{Settings: map[string]string{"timeout": "5s"}}
	if got := resolveRequestTimeout(req, 30*time.Second); got != 5*time.Second {
		t.Fatalf("expected timeout override to return 5s, got %s", got)
	}

	req.Settings["timeout"] = "invalid"
	if got := resolveRequestTimeout(req, 10*time.Second); got != 10*time.Second {
		t.Fatalf("expected fallback to base timeout, got %s", got)
	}

	if got := resolveRequestTimeout(nil, 15*time.Second); got != 15*time.Second {
		t.Fatalf("expected base timeout when request nil, got %s", got)
	}
}

func TestBuildHTTPRequestUsesInheritedFileAuth(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}
	doc := &restfile.Document{
		Path: "/tmp/inherited-auth.http",
		Auth: []restfile.AuthProfile{{
			Scope: restfile.AuthScopeFile,
			Spec: restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "file-token"},
			},
			Line: 1,
		}},
	}
	req := &restfile.Request{
		Method:    "GET",
		URL:       "https://example.com",
		LineRange: restfile.LineRange{Start: 2, End: 3},
	}

	exec := newExecContext(&model, doc, req, httpclient.Options{}, "", nil, nil)
	if msg := exec.runPreRequestScripts(); msg != nil {
		t.Fatalf("runPreRequestScripts: %v", msg.err)
	}
	exec.buildResolver()

	client := httpclient.NewClient(nil)
	httpReq, _, _, err := client.BuildHTTPRequest(
		context.Background(),
		req,
		exec.resolver,
		httpclient.Options{},
	)
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer file-token" {
		t.Fatalf("expected inherited bearer header, got %q", got)
	}
}

func TestResolveInheritedAuthUsesGlobalFallback(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}
	model.registryIndex().Sync(&restfile.Document{
		Path: "/tmp/other.http",
		Auth: []restfile.AuthProfile{{
			Scope: restfile.AuthScopeGlobal,
			Spec: restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "global-token"},
			},
			Line: 1,
		}},
	})

	req := &restfile.Request{}
	model.resolveInheritedAuth(&restfile.Document{Path: "/tmp/current.http"}, req)

	if req.Metadata.Auth == nil {
		t.Fatalf("expected inherited global auth")
	}
	if got := req.Metadata.Auth.Params["token"]; got != "global-token" {
		t.Fatalf("expected global token, got %q", got)
	}
}

func TestRunPreRequestScriptsApplyCanClearInheritedAuth(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}
	doc := &restfile.Document{
		Path: "/tmp/inherited-auth-apply.http",
		Auth: []restfile.AuthProfile{{
			Scope: restfile.AuthScopeFile,
			Spec: restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "file-token"},
			},
			Line: 1,
		}},
	}
	req := &restfile.Request{
		Method:    "GET",
		URL:       "https://example.com",
		LineRange: restfile.LineRange{Start: 2, End: 4},
		Metadata: restfile.RequestMetadata{
			Applies: []restfile.ApplySpec{{
				Expression: `{auth: null}`,
				Line:       2,
				Col:        1,
			}},
		},
	}

	exec := newExecContext(&model, doc, req, httpclient.Options{}, "", nil, nil)
	if msg := exec.runPreRequestScripts(); msg != nil {
		t.Fatalf("runPreRequestScripts: %v", msg.err)
	}
	if req.Metadata.Auth != nil {
		t.Fatalf("expected @apply to clear inherited auth")
	}
	if !req.Metadata.AuthDisabled {
		t.Fatalf("expected cleared inherited auth to stay disabled for this execution")
	}
}

func TestEnsureOAuthSetsAuthorizationHeader(t *testing.T) {
	var calls int32
	var lastAuth string
	var lastForm url.Values

	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}

	model.oauthMgr().SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			atomic.AddInt32(&calls, 1)
			values, err := url.ParseQuery(req.Body.Text)
			if err != nil {
				t.Fatalf("parse form: %v", err)
			}
			lastForm = copyValues(values)
			lastAuth = req.Headers.Get("Authorization")
			return &httpclient.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body: []byte(
					`{"access_token":"token-basic","token_type":"Bearer","expires_in":3600}`,
				),
				Headers: http.Header{},
			}, nil
		},
	)

	auth := &restfile.AuthSpec{Type: "oauth2", Params: map[string]string{
		"token_url":     "https://auth.local/token",
		"client_id":     "client",
		"client_secret": "secret",
		"scope":         "read",
	}}
	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}
	resolver := vars.NewResolver()
	if err := model.ensureOAuth(
		context.Background(),
		req,
		resolver,
		httpclient.Options{},
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureOAuth: %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer token-basic" {
		t.Fatalf("expected bearer header, got %q", got)
	}
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("client:secret"))
	if lastAuth != expectedAuth {
		t.Fatalf("expected auth header %q, got %q", expectedAuth, lastAuth)
	}
	if lastForm.Get("grant_type") != "client_credentials" {
		t.Fatalf("expected grant_type client_credentials, got %q", lastForm.Get("grant_type"))
	}

	req2 := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}
	if err := model.ensureOAuth(
		context.Background(),
		req2,
		resolver,
		httpclient.Options{},
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureOAuth second: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected cached token to prevent additional calls, got %d", calls)
	}
}

func TestEnsureOAuthSkipsWhenHeaderPresent(t *testing.T) {
	called := int32(0)
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}
	model.oauthMgr().SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			atomic.AddInt32(&called, 1)
			return &httpclient.Response{
				Status:     "200",
				StatusCode: 200,
				Body:       []byte(`{"access_token":"x"}`),
				Headers:    http.Header{},
			}, nil
		},
	)
	req := &restfile.Request{
		Headers: http.Header{"Authorization": {"Bearer manual"}},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{Type: "oauth2", Params: map[string]string{
				"token_url": "https://auth.local/token",
			}},
		},
	}
	if err := model.ensureOAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		httpclient.Options{},
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureOAuth with existing header: %v", err)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatalf("expected no oauth call when header is preset")
	}
	if req.Headers.Get("Authorization") != "Bearer manual" {
		t.Fatalf("expected header to remain unchanged")
	}
}

func copyValues(src url.Values) url.Values {
	dst := make(url.Values, len(src))
	for k, v := range src {
		cloned := make([]string, len(v))
		copy(cloned, v)
		dst[k] = cloned
	}
	return dst
}

func TestEnsureOAuthUsesEnvironmentOverride(t *testing.T) {
	var requests int32
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}
	model.oauthMgr().SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			atomic.AddInt32(&requests, 1)
			return &httpclient.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       []byte(`{"access_token":"token","token_type":"Bearer"}`),
				Headers:    http.Header{},
			}, nil
		},
	)

	auth := &restfile.AuthSpec{Type: "oauth2", Params: map[string]string{
		"token_url":     "https://auth.local/token",
		"client_id":     "client",
		"client_secret": "secret",
		"scope":         "read",
	}}
	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}

	if err := model.ensureOAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		httpclient.Options{},
		"stage",
		time.Second,
	); err != nil {
		t.Fatalf("ensureOAuth stage: %v", err)
	}
	req.Headers = nil
	if err := model.ensureOAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		httpclient.Options{},
		"stage",
		time.Second,
	); err != nil {
		t.Fatalf("ensureOAuth stage cached: %v", err)
	}
	if atomic.LoadInt32(&requests) != 1 {
		t.Fatalf("expected cached token for repeated stage env, got %d", requests)
	}

	req.Headers = nil
	if err := model.ensureOAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		httpclient.Options{},
		"dev",
		time.Second,
	); err != nil {
		t.Fatalf("ensureOAuth dev: %v", err)
	}
	if atomic.LoadInt32(&requests) != 2 {
		t.Fatalf("expected new token request when env changes, got %d", requests)
	}
}

func TestEnsureOAuthCancelsWithContext(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}

	model.oauthMgr().SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)

	auth := &restfile.AuthSpec{Type: "oauth2", Params: map[string]string{
		"token_url": "https://auth.local/token",
	}}
	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}
	resolver := vars.NewResolver()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := model.ensureOAuth(
			ctx,
			req,
			resolver,
			httpclient.Options{},
			"",
			time.Minute,
		); !errors.Is(
			err,
			context.Canceled,
		) {
			t.Errorf("expected context cancellation, got %v", err)
		}
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ensureOAuth did not return after cancellation")
	}
}

func TestEnsureCommandAuthSetsAuthorizationHeader(t *testing.T) {
	var calls int32
	var seen authcmd.Config

	model := Model{
		cfg:         Config{EnvironmentName: "dev"},
		currentFile: "/tmp/example.http",
	}
	model.authCmdMgr().SetExecFunc(func(_ context.Context, cfg authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		seen = cfg
		return []byte("token-basic"), nil
	})

	auth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"argv":      `["gh","auth","token"]`,
		"cache_key": "github",
	}}
	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}

	res, err := model.ensureCommandAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		"",
		5*time.Second,
	)
	if err != nil {
		t.Fatalf("ensureCommandAuth: %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer token-basic" {
		t.Fatalf("expected bearer header, got %q", got)
	}
	if res.Token != "token-basic" {
		t.Fatalf("expected token result, got %q", res.Token)
	}
	if seen.Dir != "/tmp" {
		t.Fatalf("expected command dir /tmp, got %q", seen.Dir)
	}
	if seen.Timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %s", seen.Timeout)
	}

	req2 := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}
	if _, err := model.ensureCommandAuth(
		context.Background(),
		req2,
		vars.NewResolver(),
		"",
		5*time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth second: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected cached command auth result, got %d calls", calls)
	}
}

func TestEnsureCommandAuthSkipsWhenHeaderPresent(t *testing.T) {
	called := int32(0)

	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}
	model.authCmdMgr().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&called, 1)
		return []byte("token-basic"), nil
	})

	req := &restfile.Request{
		Headers: http.Header{"Authorization": {"Bearer manual"}},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{Type: "command", Params: map[string]string{
				"argv": `["gh","auth","token"]`,
			}},
		},
	}

	if _, err := model.ensureCommandAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth with existing header: %v", err)
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatalf("expected no command auth execution, got %d", called)
	}
	if req.Headers.Get("Authorization") != "Bearer manual" {
		t.Fatalf("expected header to remain unchanged")
	}
}

func TestEnsureCommandAuthCacheOnlyReuseInheritsSeededConfig(t *testing.T) {
	var calls int32

	model := Model{
		cfg:         Config{EnvironmentName: "dev"},
		currentFile: "/tmp/example.http",
	}
	model.authCmdMgr().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("token-basic"), nil
	})

	seedAuth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"argv":      `["gh","auth","token"]`,
		"cache_key": "github",
		"header":    "X-Registry-Token",
		"scheme":    "Token",
	}}
	cacheOnlyAuth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"cache_key": "github",
	}}

	seedReq := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: seedAuth}}
	if _, err := model.ensureCommandAuth(
		context.Background(),
		seedReq,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth seed: %v", err)
	}

	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: cacheOnlyAuth}}
	res, err := model.ensureCommandAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		"",
		time.Second,
	)
	if err != nil {
		t.Fatalf("ensureCommandAuth cache-only: %v", err)
	}
	if got := req.Headers.Get("X-Registry-Token"); got != "Token token-basic" {
		t.Fatalf("expected inherited seeded header, got %q", got)
	}
	if res.Header != "X-Registry-Token" || res.Value != "Token token-basic" {
		t.Fatalf("unexpected command auth result %#v", res)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected cache-only reuse to skip execution, got %d calls", calls)
	}
}

func TestBuildCommandAuthConfigExpandsArgvAfterJSONDecode(t *testing.T) {
	model := Model{
		cfg:         Config{EnvironmentName: "dev"},
		currentFile: "/tmp/example.http",
	}

	auth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"argv": `["aws","--profile","{{aws.profile}}","ecr","get-login-password"]`,
	}}
	resolver := vars.NewResolver(vars.NewMapProvider("aws", map[string]string{
		"profile": `qa"blue\team`,
	}))

	cfg, err := model.buildCommandAuthConfig(auth, resolver, 5*time.Second)
	if err != nil {
		t.Fatalf("buildCommandAuthConfig: %v", err)
	}

	if got := cfg.Argv[2]; got != `qa"blue\team` {
		t.Fatalf("expected expanded argv value with quotes and slashes preserved, got %q", got)
	}
	if cfg.Timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %s", cfg.Timeout)
	}
}

func TestPrepareExplainAuthPreviewCommandUsesCacheOnly(t *testing.T) {
	var calls int32

	model := Model{
		cfg:         Config{EnvironmentName: "dev"},
		currentFile: "/tmp/example.http",
	}
	model.authCmdMgr().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		return []byte("token-basic"), nil
	})

	auth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"argv":      `["gh","auth","token"]`,
		"cache_key": "github",
		"header":    "X-Registry-Token",
		"scheme":    "Token",
	}}

	prime := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}
	if _, err := model.ensureCommandAuth(
		context.Background(),
		prime,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth prime: %v", err)
	}

	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
		Type: "command",
		Params: map[string]string{
			"cache_key": "github",
		},
	}}}
	preview, err := model.prepareExplainAuthPreview(req, vars.NewResolver(), "")
	if err != nil {
		t.Fatalf("prepareExplainAuthPreview: %v", err)
	}
	if preview.status != xplain.StageOK {
		t.Fatalf("expected preview ok, got %v", preview.status)
	}
	if preview.summary != explainSummaryAuthPrepared {
		t.Fatalf("expected auth prepared summary, got %q", preview.summary)
	}
	if got := req.Headers.Get("X-Registry-Token"); got != "Token token-basic" {
		t.Fatalf("expected cached auth header, got %q", got)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected preview to reuse cache only, got %d executions", calls)
	}
	if len(preview.extraSecrets) == 0 {
		t.Fatalf("expected preview secrets to include cached auth values")
	}
}

func TestPrepareExplainAuthPreviewCommandCacheOnlyRequiresSeed(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}

	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{Type: "command", Params: map[string]string{
				"cache_key": "github",
			}},
		},
	}

	_, err := model.prepareExplainAuthPreview(req, vars.NewResolver(), "")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "seed the cache") {
		t.Fatalf("expected seed hint, got %q", got)
	}
}

func TestPrepareExplainAuthPreviewCommandSkipsWithoutCache(t *testing.T) {
	called := int32(0)

	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}
	model.authCmdMgr().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&called, 1)
		return []byte("token-basic"), nil
	})

	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{Type: "command", Params: map[string]string{
				"argv": `["gh","auth","token"]`,
			}},
		},
	}

	preview, err := model.prepareExplainAuthPreview(req, vars.NewResolver(), "")
	if err != nil {
		t.Fatalf("prepareExplainAuthPreview: %v", err)
	}
	if preview.status != xplain.StageSkipped {
		t.Fatalf("expected preview skip, got %v", preview.status)
	}
	if preview.summary != explainSummaryCommandAuthExecutionSkipped {
		t.Fatalf("expected command skip summary, got %q", preview.summary)
	}
	if req.Headers.Get("Authorization") != "" {
		t.Fatalf("expected no auth header injection without cache")
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatalf("expected preview to skip command execution, got %d calls", called)
	}
}

func TestResolveHTTPOptionsFallbackEnvEnable(t *testing.T) {
	model := &Model{currentFile: "/tmp/request.http", workspaceRoot: "/workspace"}
	opts := httpclient.Options{BaseDir: "", FallbackBaseDirs: []string{"/extra"}}

	t.Setenv("RESTERM_ENABLE_FALLBACK", "true")
	resolved := model.resolveHTTPOptions(opts)
	if len(resolved.FallbackBaseDirs) == 0 {
		t.Fatalf("expected fallbacks enabled, got %v", resolved.FallbackBaseDirs)
	}
	if resolved.NoFallback {
		t.Fatalf("expected NoFallback to be false when enabled")
	}
}

func TestExecuteRequestCancelsBeforePreRequest(t *testing.T) {
	model := Model{
		cfg:          Config{EnvironmentName: "dev"},
		scriptRunner: scripts.NewRunner(nil),
	}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
	}

	cmd := model.executeRequest(nil, req, httpclient.Options{}, "", nil)
	if cmd == nil {
		t.Fatalf("expected executeRequest to return command")
	}
	if model.sendCancel == nil {
		t.Fatalf("expected sendCancel to be set")
	}

	model.sendCancel()
	msg := cmd()
	resp, ok := msg.(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg, got %T", msg)
	}
	if !errors.Is(resp.err, context.Canceled) {
		t.Fatalf("expected cancellation error, got %v", resp.err)
	}
}

func TestExecuteRequestInteractiveWebSocketStaysAlive(t *testing.T) {
	srv, cleanup := startUIWebSocketServer(t)
	defer cleanup()

	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/ws/chat"
	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    wsURL,
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{},
		},
	}

	cmd := model.executeRequest(nil, req, httpclient.Options{}, "", nil)
	if cmd == nil {
		t.Fatalf("expected executeRequest to return command")
	}

	msg := cmd()
	resp, ok := msg.(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg, got %T", msg)
	}
	if resp.err != nil {
		t.Fatalf("unexpected error: %v", resp.err)
	}
	if resp.response == nil {
		t.Fatalf("expected placeholder websocket response")
	}
	if got := resp.response.Headers.Get(streamHeaderType); got != "websocket" {
		t.Fatalf("expected websocket placeholder header, got %q", got)
	}

	sessionID := model.sessionIDForRequest(req)
	if sessionID == "" {
		t.Fatalf("expected websocket session to be recorded")
	}
	session := model.sessionHandles[sessionID]
	if session == nil {
		t.Fatalf("expected websocket session handle for %s", sessionID)
	}

	select {
	case <-session.Done():
		t.Fatal("interactive websocket session canceled immediately after request returned")
	case <-time.After(150 * time.Millisecond):
	}

	session.Cancel()
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket session shutdown")
	}
}

func TestExecuteRequestRejectsNilRequest(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}

	cmd := model.executeRequest(nil, nil, httpclient.Options{}, "", nil)
	if cmd == nil {
		t.Fatalf("expected executeRequest to return command")
	}

	msg := cmd()
	resp, ok := msg.(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg, got %T", msg)
	}
	if resp.err == nil || !strings.Contains(resp.err.Error(), "request is nil") {
		t.Fatalf("expected nil request error, got %v", resp.err)
	}
}

func TestExecuteRequestRejectsSSHAndK8sBeforeResolve(t *testing.T) {
	model := Model{
		cfg:          Config{EnvironmentName: "dev"},
		scriptRunner: scripts.NewRunner(nil),
	}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		SSH:    &restfile.SSHSpec{},
		K8s:    &restfile.K8sSpec{},
	}

	cmd := model.executeRequest(nil, req, httpclient.Options{}, "", nil)
	if cmd == nil {
		t.Fatalf("expected executeRequest to return command")
	}

	msg := cmd()
	resp, ok := msg.(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg, got %T", msg)
	}
	if resp.err == nil || !strings.Contains(resp.err.Error(), "@ssh cannot be combined with @k8s") {
		t.Fatalf("expected transport conflict error, got %v", resp.err)
	}
}

func TestCancelActiveRunsStopsSend(t *testing.T) {
	model := New(Config{})
	model.sending = true
	model.statusPulseBase = "Sending test"
	model.statusPulseFrame = 2
	model.statusPulseOn = true

	canceled := false
	model.sendCancel = func() { canceled = true }

	cmd := model.cancelActiveRuns()
	if cmd != nil {
		t.Fatalf("expected cancelActiveRuns to return nil command, got %v", cmd)
	}
	if model.sending {
		t.Fatalf("expected sending flag to reset")
	}
	if model.statusPulseBase != "" || model.statusPulseFrame != 0 {
		t.Fatalf(
			"expected pulse state cleared, got %q/%d",
			model.statusPulseBase,
			model.statusPulseFrame,
		)
	}
	if model.statusPulseOn {
		t.Fatalf("expected pulse to stop")
	}
	if !canceled {
		t.Fatalf("expected sendCancel to be invoked")
	}
	if text := strings.ToLower(model.statusMessage.text); !strings.Contains(text, "canceling") {
		t.Fatalf("expected cancel status message, got %q", model.statusMessage.text)
	}
}

func TestCancelActiveRunsNoopWhenIdle(t *testing.T) {
	model := New(Config{})
	cmd := model.cancelActiveRuns()
	if cmd != nil {
		t.Fatalf("expected nil command when nothing is active, got %v", cmd)
	}
	if model.statusMessage.text != "" {
		t.Fatalf("did not expect status message, got %q", model.statusMessage.text)
	}
}

func TestStartStatusPulseIdempotent(t *testing.T) {
	m := New(Config{})
	m.sending = true
	cmd := m.startStatusPulse()
	if cmd == nil {
		t.Fatalf("expected startStatusPulse to return command")
	}
	if !m.statusPulseOn {
		t.Fatalf("expected pulse to start")
	}
	m.statusPulseFrame = 2

	cmd2 := m.startStatusPulse()
	if cmd2 != nil {
		t.Fatalf("expected startStatusPulse to be idempotent")
	}
	if m.statusPulseFrame != 2 {
		t.Fatalf("expected pulse frame preserved, got %d", m.statusPulseFrame)
	}
}

func TestScheduleStatusPulseWhenRunActive(t *testing.T) {
	m := New(Config{})
	m.statusPulseOn = true
	m.sending = true

	cmd := m.scheduleStatusPulse()
	if cmd == nil {
		t.Fatalf("expected scheduleStatusPulse to return command")
	}
}

func TestApplyCapturesStoresValues(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}

	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Header: http.Header{
			"X-Trace": {"abc"},
		},
		Body: []byte(`{"token":"abc123","nested":{"value":42}}`),
	}

	doc := &restfile.Document{Path: "./sample.http"}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      restfile.CaptureScopeGlobal,
					Name:       "authToken",
					Expression: "Bearer {{response.json.token}}",
					Secret:     true,
				},
				{
					Scope:      restfile.CaptureScopeFile,
					Name:       "lastTrace",
					Expression: "{{response.headers.X-Trace}}",
					Secret:     false,
				},
				{
					Scope:      restfile.CaptureScopeRequest,
					Name:       "recentStatus",
					Expression: "{{response.status}}",
					Secret:     false,
				},
			},
		},
	}

	resolver := model.buildResolver(context.Background(), doc, req, "", "", nil)
	var captures captureResult
	if err := model.applyCaptures(captureRun{
		doc:  doc,
		req:  req,
		res:  resolver,
		resp: resp,
		out:  &captures,
	}); err != nil {
		t.Fatalf("applyCaptures: %v", err)
	}

	if _, ok := captures.requestVars["recentstatus"]; !ok {
		t.Fatalf("expected request capture to be recorded: %+v", captures.requestVars)
	}
	if _, ok := captures.fileVars["lasttrace"]; !ok {
		t.Fatalf("expected file capture to be recorded: %+v", captures.fileVars)
	}

	snapshot := model.globalsStore().Snapshot("dev")
	if len(snapshot) != 1 {
		t.Fatalf("expected one global, got %d", len(snapshot))
	}
	var entry globalValue
	found := false
	for _, v := range snapshot {
		if strings.EqualFold(v.Name, "authToken") {
			entry = v
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("authToken not found in globals: %+v", snapshot)
	}
	if entry.Value != "Bearer abc123" {
		t.Fatalf("unexpected global value %q", entry.Value)
	}
	if !entry.Secret {
		t.Fatalf("expected global secret flag")
	}

	if len(doc.Variables) != 1 {
		t.Fatalf("expected one file variable, got %d", len(doc.Variables))
	}
	if doc.Variables[0].Name != "lastTrace" || doc.Variables[0].Value != "abc" {
		t.Fatalf("unexpected file variable %+v", doc.Variables[0])
	}
	if len(req.Variables) != 1 {
		t.Fatalf("expected one request variable, got %d", len(req.Variables))
	}
	if req.Variables[0].Name != "recentStatus" || req.Variables[0].Value != "200 OK" {
		t.Fatalf("unexpected request variable %+v", req.Variables[0])
	}
	varsWithReq := model.collectVariables(doc, req, "")
	if varsWithReq["recentStatus"] != "200 OK" {
		t.Fatalf(
			"expected request capture to be available in collected vars, got %q",
			varsWithReq["recentStatus"],
		)
	}

	store := model.fileStore().Snapshot("dev", "./sample.http")
	if len(store) != 1 {
		t.Fatalf("expected one stored file variable, got %d", len(store))
	}
	var stored fileVariable
	for _, entry := range store {
		stored = entry
	}
	if stored.Name != "lastTrace" || stored.Value != "abc" {
		t.Fatalf("unexpected stored file capture %+v", stored)
	}

	// simulate a fresh parse of the document (no baked-in variables)
	freshDoc := &restfile.Document{Path: "./sample.http"}
	vars := model.collectVariables(freshDoc, nil, "")
	if vars["lastTrace"] != "abc" {
		t.Fatalf("expected file capture to be applied via runtime store, got %q", vars["lastTrace"])
	}
}

func TestApplyCapturesEvaluatesRSTExpressions(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}

	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Header: http.Header{
			"X-Amzn-Trace-Id": {"t-123"},
		},
		Body: []byte(`{"token":"abc123","data":{"id":"u-1"}}`),
	}
	doc := &restfile.Document{Path: "./capture-rst.http"}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      restfile.CaptureScopeGlobal,
					Name:       "auth.token",
					Expression: `response.json.token`,
					Secret:     true,
					Line:       2,
				},
				{
					Scope:      restfile.CaptureScopeFile,
					Name:       "user.id",
					Expression: `response.json.data.id`,
					Line:       3,
				},
				{
					Scope:      restfile.CaptureScopeRequest,
					Name:       "trace",
					Expression: `response.headers["x-amzn-trace-id"]`,
					Line:       4,
				},
			},
		},
	}

	var captures captureResult
	if err := model.applyCaptures(captureRun{
		doc:  doc,
		req:  req,
		resp: resp,
		out:  &captures,
	}); err != nil {
		t.Fatalf("applyCaptures rst: %v", err)
	}

	gl := model.globalsStore().Snapshot("dev")
	if len(gl) != 1 {
		t.Fatalf("expected one global capture, got %d", len(gl))
	}
	var g globalValue
	for _, v := range gl {
		g = v
	}
	if g.Value != "abc123" {
		t.Fatalf("expected token capture, got %q", g.Value)
	}
	if !g.Secret {
		t.Fatalf("expected secret global capture")
	}

	if len(doc.Variables) != 1 || doc.Variables[0].Value != "u-1" {
		t.Fatalf("expected file capture u-1, got %+v", doc.Variables)
	}
	if len(req.Variables) != 1 || req.Variables[0].Value != "t-123" {
		t.Fatalf("expected request trace capture, got %+v", req.Variables)
	}
}

func TestApplyCapturesRSTKeepsQuotedTemplateMarkersLiteral(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"token":"abc123"}`),
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "quoted",
				Expression: `"{{response.json.token}}"`,
			}},
		},
	}

	if err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	}); err != nil {
		t.Fatalf("applyCaptures rst quoted template markers: %v", err)
	}
	if len(req.Variables) != 1 {
		t.Fatalf("expected one request capture, got %d", len(req.Variables))
	}
	if req.Variables[0].Value != "{{response.json.token}}" {
		t.Fatalf("expected literal quoted markers, got %q", req.Variables[0].Value)
	}
}

func TestApplyCapturesFailsOnMixedTemplateRTSCall(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"name":"alice"}`),
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "mixed",
				Expression: `contains({{name}}, "ali")`,
				Mode:       restfile.CaptureExprModeTemplate,
			}},
		},
	}

	err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	})
	if err == nil {
		t.Fatalf("expected mixed template/rts call syntax to fail")
	}
	if !strings.Contains(err.Error(), "mixed capture syntax is not supported") {
		t.Fatalf("expected mixed-syntax error, got %q", err.Error())
	}
}

func TestApplyCapturesFailsOnMixedTemplateRTSCallAutoMode(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"name":"alice"}`),
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "mixed",
				Expression: `contains({{name}}, "ali")`,
			}},
		},
	}

	err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	})
	if err == nil {
		t.Fatalf("expected mixed template/rts call syntax to fail in auto mode")
	}
	if !strings.Contains(err.Error(), "mixed capture syntax is not supported") {
		t.Fatalf("expected mixed-syntax error, got %q", err.Error())
	}
}

func TestApplyCapturesFailsOnMixedTemplateRTSSingleArgCall(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"name":"alice"}`),
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "mixed",
				Expression: `contains({{name}})`,
				Mode:       restfile.CaptureExprModeTemplate,
			}},
		},
	}

	err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	})
	if err == nil {
		t.Fatalf("expected mixed single-arg template/rts call syntax to fail")
	}
	if !strings.Contains(err.Error(), "mixed capture syntax is not supported") {
		t.Fatalf("expected mixed-syntax error, got %q", err.Error())
	}
}

func TestApplyCapturesRSTStreamExpression(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{Kind: scripts.ResponseKindHTTP, Status: "101"}
	stream := &scripts.StreamInfo{
		Kind: "websocket",
		Events: []map[string]interface{}{
			{"text": "hello"},
			{"text": "world"},
		},
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "last",
				Expression: "stream.events()[1].text",
			}},
		},
	}

	var captures captureResult
	if err := model.applyCaptures(captureRun{
		req:    req,
		resp:   resp,
		stream: stream,
		out:    &captures,
	}); err != nil {
		t.Fatalf("applyCaptures stream rst: %v", err)
	}
	if len(req.Variables) != 1 || req.Variables[0].Value != "world" {
		t.Fatalf("expected stream capture world, got %+v", req.Variables)
	}
}

func TestApplyCapturesStrictModeFailsOnMissingJSONPath(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"token":"abc123"}`),
	}
	req := &restfile.Request{
		Method: "POST",
		URL:    "https://example.com/login",
		Metadata: restfile.RequestMetadata{
			Name: "Login",
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "auth.token",
				Expression: "{{response.json.missing.token}}",
				Line:       7,
			}},
		},
		Settings: map[string]string{
			"capture.strict": "true",
		},
	}

	err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	})
	if err == nil {
		t.Fatalf("expected strict capture to fail on missing json path")
	}
	msg := err.Error()
	for _, want := range []string{
		`evaluate capture "auth.token"`,
		`request="Login"`,
		`line=7`,
		`expr="{{response.json.missing.token}}"`,
		`json path "json.missing.token" failed at "json.missing"`,
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected error to include %q, got %q", want, msg)
		}
	}
}

func TestApplyCapturesNonStrictKeepsTemplateMissingJSONEmpty(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"token":"abc123"}`),
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "auth.token",
				Expression: "{{response.json.missing.token}}",
			}},
		},
	}

	if err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	}); err != nil {
		t.Fatalf("expected non-strict capture to stay backward compatible, got: %v", err)
	}
	if len(req.Variables) != 1 {
		t.Fatalf("expected one request variable, got %d", len(req.Variables))
	}
	if req.Variables[0].Value != "" {
		t.Fatalf("expected missing json path to resolve empty in non-strict mode")
	}
}

func TestApplyCapturesErrorIncludesNormalizedExpression(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"token":"abc123"}`),
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Name: "BrokenCapture",
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "token",
				Expression: "response.json.token[",
				Line:       5,
			}},
		},
	}

	err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	})
	if err == nil {
		t.Fatalf("expected invalid capture expression to fail")
	}
	msg := err.Error()
	for _, want := range []string{
		`evaluate capture "token"`,
		`request="BrokenCapture"`,
		`line=5`,
		`expr="response.json.token["`,
		`norm="response.json().token["`,
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected error to include %q, got %q", want, msg)
		}
	}
}

func TestApplyCapturesStrictModeSupportsMultiIndexJSONPath(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"m":[["a","b"],["c","d"]]}`),
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "v",
				Expression: "{{response.json.m[0][1]}}",
			}},
		},
		Settings: map[string]string{
			"capture.strict": "true",
		},
	}

	if err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	}); err != nil {
		t.Fatalf("expected strict multi-index capture to succeed, got: %v", err)
	}
	if len(req.Variables) != 1 || req.Variables[0].Value != "b" {
		t.Fatalf("expected capture value b, got %+v", req.Variables)
	}
}

func TestApplyCapturesStrictModeFailsOnMalformedJSONPath(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"items":[1,2,3]}`),
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "v",
				Expression: "{{response.json.items[}}",
				Line:       9,
			}},
		},
		Settings: map[string]string{
			"capture.strict": "true",
		},
	}

	err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	})
	if err == nil {
		t.Fatalf("expected malformed strict json path to fail")
	}
	if !strings.Contains(err.Error(), "missing closing bracket") {
		t.Fatalf("expected malformed path detail in error, got %q", err.Error())
	}
}

func TestApplyCapturesStrictModeFailsOnUnexpectedJSONPathChar(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"foo":{"bar":"ok"}}`),
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "v",
				Expression: "{{response.json.foo]bar}}",
				Line:       11,
			}},
		},
		Settings: map[string]string{
			"capture.strict": "true",
		},
	}

	err := model.applyCaptures(captureRun{
		req:  req,
		resp: resp,
	})
	if err == nil {
		t.Fatalf("expected malformed strict json path to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, `json path "json.foo]bar"`) {
		t.Fatalf("expected malformed path in error, got %q", msg)
	}
	if !strings.Contains(msg, "got ']'") {
		t.Fatalf("expected offending char detail in error, got %q", msg)
	}
}

func TestApplyCapturesUsesEnvironmentOverride(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}

	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
	}

	doc := &restfile.Document{Path: "./capture-env.http"}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      restfile.CaptureScopeGlobal,
					Name:       "status",
					Expression: "{{response.status}}",
				},
				{
					Scope:      restfile.CaptureScopeFile,
					Name:       "lastStatus",
					Expression: "{{response.status}}",
				},
			},
		},
	}

	var captures captureResult
	if err := model.applyCaptures(captureRun{
		doc:  doc,
		req:  req,
		resp: resp,
		out:  &captures,
		env:  "stage",
	}); err != nil {
		t.Fatalf("applyCaptures stage: %v", err)
	}

	if len(model.globalsStore().Snapshot("dev")) != 0 {
		t.Fatalf("expected no globals in dev env after stage capture")
	}
	stageGlobals := model.globalsStore().Snapshot("stage")
	if len(stageGlobals) != 1 {
		t.Fatalf("expected one global in stage, got %d", len(stageGlobals))
	}

	devStore := model.fileStore().Snapshot("dev", "./capture-env.http")
	if len(devStore) != 0 {
		t.Fatalf("expected no file captures in dev store")
	}

	stageStore := model.fileStore().Snapshot("stage", "./capture-env.http")
	if len(stageStore) != 1 {
		t.Fatalf("expected one file capture in stage store, got %d", len(stageStore))
	}
}

func TestApplyCapturesStreamNegativeIndex(t *testing.T) {
	model := Model{}
	resp := &scripts.Response{Kind: scripts.ResponseKindHTTP, Status: "200"}
	stream := &scripts.StreamInfo{
		Kind: "sse",
		Events: []map[string]interface{}{
			{"event": "ready"},
			{"event": "change", "data": "value"},
		},
	}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "last",
				Expression: "{{stream.events[-1].event}}",
			}},
		},
	}
	var captures captureResult
	if err := model.applyCaptures(captureRun{
		req:    req,
		resp:   resp,
		stream: stream,
		out:    &captures,
	}); err != nil {
		t.Fatalf("applyCaptures stream: %v", err)
	}
	if len(req.Variables) == 0 || req.Variables[len(req.Variables)-1].Value != "change" {
		t.Fatalf("expected last event to be change, got %+v", req.Variables)
	}
}

func TestApplyCapturesWithStreamData(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}

	streamInfo := &scripts.StreamInfo{
		Kind: "websocket",
		Summary: map[string]interface{}{
			"sentCount":     1,
			"receivedCount": 2,
		},
		Events: []map[string]interface{}{
			{"text": "hello"},
			{"text": "world"},
		},
	}

	resp := &scripts.Response{Kind: scripts.ResponseKindHTTP, Status: "101 Switching Protocols"}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      restfile.CaptureScopeRequest,
					Name:       "streamKind",
					Expression: "{{stream.kind}}",
				},
				{
					Scope:      restfile.CaptureScopeFile,
					Name:       "received",
					Expression: "{{stream.summary.receivedCount}}",
				},
				{
					Scope:      restfile.CaptureScopeGlobal,
					Name:       "lastMessage",
					Expression: "{{stream.events[1].text}}",
				},
			},
		},
	}

	doc := &restfile.Document{Path: "./stream.http"}
	resolver := model.buildResolver(context.Background(), doc, req, "", "", nil)
	var captures captureResult
	if err := model.applyCaptures(captureRun{
		doc:    doc,
		req:    req,
		res:    resolver,
		resp:   resp,
		stream: streamInfo,
		out:    &captures,
	}); err != nil {
		t.Fatalf("applyCaptures stream: %v", err)
	}

	vars := model.collectVariables(doc, req, "")
	if vars["streamKind"] != "websocket" {
		t.Fatalf("expected stream kind capture, got %q", vars["streamKind"])
	}
	if len(doc.Variables) == 0 || doc.Variables[0].Value != "2" {
		t.Fatalf("expected file capture for received count, got %+v", doc.Variables)
	}
	snapshot := model.globalsStore().Snapshot("dev")
	if len(snapshot) != 1 {
		t.Fatalf("expected one global capture, got %d", len(snapshot))
	}
	var globalEntry globalValue
	for _, value := range snapshot {
		globalEntry = value
	}
	if globalEntry.Value != "world" {
		t.Fatalf("expected last message capture to be world, got %q", globalEntry.Value)
	}
}

func TestShowGlobalSummary(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
		doc: &restfile.Document{
			Globals: []restfile.Variable{
				{Name: "docVar", Value: "foo"},
				{Name: "secretDoc", Value: "bar", Secret: true},
			},
		},
	}
	model.globalsStore().Set("dev", "token", "secretValue", true)
	model.globalsStore().Set("dev", "refresh", "xyz", false)

	model.showGlobalSummary()

	expected := "Globals: refresh=xyz, token=••• | Doc: docVar=foo, secretDoc=•••"
	if model.statusMessage.text != expected {
		t.Fatalf("expected summary %q, got %q", expected, model.statusMessage.text)
	}
	if model.statusMessage.level != statusInfo {
		t.Fatalf("expected info status, got %v", model.statusMessage.level)
	}
}

func TestClearGlobalValues(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}
	model.globalsStore().Set("dev", "token", "value", false)
	model.cookieStore().Jar("dev")
	if snap := model.globalsStore().Snapshot("dev"); len(snap) == 0 {
		t.Fatalf("expected snapshot to contain entries before clearing")
	}
	model.clearGlobalValues()
	if snap := model.globalsStore().Snapshot("dev"); len(snap) != 0 {
		t.Fatalf("expected globals to be cleared, got %v", snap)
	}
	if !strings.Contains(model.statusMessage.text, "Cleared globals and cookies") {
		t.Fatalf("expected confirmation message, got %q", model.statusMessage.text)
	}
	if model.statusMessage.level != statusInfo {
		t.Fatalf("expected info level, got %v", model.statusMessage.level)
	}
}

func TestClearGlobalValuesClearsCookiesForEnvironment(t *testing.T) {
	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}
	u, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	model.cookieStore().Jar("dev").SetCookies(u, []*http.Cookie{{Name: "session", Value: "dev123"}})
	model.cookieStore().Jar("prod").SetCookies(u, []*http.Cookie{{Name: "session", Value: "prod456"}})

	model.clearGlobalValues()

	if got := model.cookieStore().Jar("dev").Cookies(u); len(got) != 0 {
		t.Fatalf("expected dev cookies to be cleared, got %+v", got)
	}
	if got := model.cookieStore().Jar("prod").Cookies(u); len(got) != 1 || got[0].Value != "prod456" {
		t.Fatalf("expected prod cookies to remain, got %+v", got)
	}
}

func TestExecuteRequestIsolatesCookiesPerEnvironment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "dev123", Path: "/"})
		case "/echo":
			if cookie, err := r.Cookie("session"); err == nil {
				_, _ = io.WriteString(w, cookie.String())
			}
		}
	}))
	defer srv.Close()

	model := New(Config{EnvironmentName: "dev"})

	setReq := &restfile.Request{Method: http.MethodGet, URL: srv.URL + "/set"}
	echoReq := &restfile.Request{Method: http.MethodGet, URL: srv.URL + "/echo"}

	msg, ok := model.executeRequest(nil, setReq, httpclient.Options{}, "dev", nil)().(responseMsg)
	if !ok || msg.err != nil {
		t.Fatalf("unexpected set response: %#v", msg)
	}

	msg, ok = model.executeRequest(nil, echoReq, httpclient.Options{}, "dev", nil)().(responseMsg)
	if !ok || msg.err != nil {
		t.Fatalf("unexpected dev echo response: %#v", msg)
	}
	if got := strings.TrimSpace(string(msg.response.Body)); got != "session=dev123" {
		t.Fatalf("expected dev cookie, got %q", got)
	}

	msg, ok = model.executeRequest(nil, echoReq, httpclient.Options{}, "prod", nil)().(responseMsg)
	if !ok || msg.err != nil {
		t.Fatalf("unexpected prod echo response: %#v", msg)
	}
	if got := strings.TrimSpace(string(msg.response.Body)); got != "" {
		t.Fatalf("expected no prod cookie, got %q", got)
	}
}

func TestExecuteRequestNoCookiesSettingDisablesJar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/echo":
			if cookie, err := r.Cookie("session"); err == nil {
				_, _ = io.WriteString(w, cookie.String())
			}
		}
	}))
	defer srv.Close()

	model := New(Config{EnvironmentName: "dev"})
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	model.cookieStore().Jar("dev").SetCookies(u, []*http.Cookie{{Name: "session", Value: "dev123"}})

	req := &restfile.Request{
		Method:   http.MethodGet,
		URL:      srv.URL + "/echo",
		Settings: map[string]string{"no-cookies": "true"},
	}
	msg, ok := model.executeRequest(nil, req, httpclient.Options{}, "dev", nil)().(responseMsg)
	if !ok || msg.err != nil {
		t.Fatalf("unexpected response: %#v", msg)
	}
	if got := strings.TrimSpace(string(msg.response.Body)); got != "" {
		t.Fatalf("expected no cookie to be sent, got %q", got)
	}
}

func TestExecuteRequestWithTraceSpecPopulatesTimeline(t *testing.T) {
	model := New(Config{})

	client := model.client
	client.SetHTTPFactory(func(opts httpclient.Options) (*http.Client, error) {
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clientTrace := httptrace.ContextClientTrace(req.Context())
			if clientTrace != nil {
				now := time.Now()
				if clientTrace.DNSStart != nil {
					clientTrace.DNSStart(httptrace.DNSStartInfo{Host: req.URL.Host})
				}
				time.Sleep(100 * time.Microsecond)
				if clientTrace.DNSDone != nil {
					clientTrace.DNSDone(
						httptrace.DNSDoneInfo{Addrs: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}},
					)
				}
				time.Sleep(100 * time.Microsecond)
				if clientTrace.ConnectStart != nil {
					clientTrace.ConnectStart("tcp", req.URL.Host)
				}
				time.Sleep(100 * time.Microsecond)
				if clientTrace.ConnectDone != nil {
					clientTrace.ConnectDone("tcp", req.URL.Host, nil)
				}
				time.Sleep(100 * time.Microsecond)
				if clientTrace.WroteHeaders != nil {
					clientTrace.WroteHeaders()
				}
				time.Sleep(100 * time.Microsecond)
				if clientTrace.WroteRequest != nil {
					clientTrace.WroteRequest(httptrace.WroteRequestInfo{})
				}
				time.Sleep(100 * time.Microsecond)
				if clientTrace.GotFirstResponseByte != nil {
					clientTrace.GotFirstResponseByte()
				}
				_ = now
			}

			resp := &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("ok")),
				Request:    req,
			}
			return resp, nil
		})
		return &http.Client{Transport: transport}, nil
	})

	content := "### Trace\n# @trace total<=1s\nGET https://example.com\n\n"
	doc := parser.Parse("trace.http", []byte(content))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected single request")
	}
	req := doc.Requests[0]
	cmd := model.executeRequest(doc, req, model.cfg.HTTPOptions, "", nil)
	if cmd == nil {
		t.Fatalf("expected executeRequest command")
	}
	msg, ok := cmd().(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg")
	}
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}

	model.handleResponseMessage(msg)
	if model.responseLatest == nil || model.responseLatest.timeline == nil {
		t.Fatalf("expected timeline to be populated in snapshot")
	}
}

func TestApplyNoCookiesSetting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, cookie := range r.Cookies() {
			_, _ = fmt.Fprintf(w, "%s\n", cookie.String())
		}
	}))
	defer srv.Close()

	model := Model{
		cfg: Config{EnvironmentName: "dev"},
	}

	// Prepare the cookie jar
	u, _ := url.Parse(srv.URL)
	model.cookieStore().Jar("dev").SetCookies(u, []*http.Cookie{
		{Name: "foo", Value: "bar"},
	})

	// First request to check the cookie is set by default
	req := &restfile.Request{
		Method: "GET",
		URL:    srv.URL,
	}

	cmd := model.executeRequest(nil, req, httpclient.Options{NoFallback: true}, "dev", nil)
	if cmd == nil {
		t.Fatalf("expected executeRequest to return command")
	}

	msg, ok := cmd().(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg")
	}
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}

	respBodyString := strings.TrimSpace(string(msg.response.Body))
	if respBodyString != "foo=bar" {
		t.Fatalf("expected cookie foo=bar in dev env, got %q", respBodyString)
	}

	// Second request with setting to skip cookies
	reqWithSetting := &restfile.Request{
		Method:   "GET",
		URL:      srv.URL,
		Settings: map[string]string{"no-cookies": "true"},
	}

	cmd = model.executeRequest(nil, reqWithSetting, httpclient.Options{NoFallback: true}, "dev", nil)
	msg, ok = cmd().(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg")
	}
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}

	respBodyString = strings.TrimSpace(string(msg.response.Body))
	if respBodyString != "" {
		t.Fatalf("expected no cookies to be sent, but got %q", respBodyString)
	}
}
