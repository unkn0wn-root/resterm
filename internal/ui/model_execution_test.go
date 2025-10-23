package ui

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
	"google.golang.org/grpc/codes"
)

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
			Metadata:   map[string]string{"authorization": "Bearer {{token}}"},
		},
	}

	var model Model
	if err := model.prepareGRPCRequest(req, resolver); err != nil {
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
	if want := "Bearer abcd"; req.GRPC.Metadata["authorization"] != want {
		t.Fatalf("expected metadata to be expanded to %q, got %q", want, req.GRPC.Metadata["authorization"])
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
	if err := model.prepareGRPCRequest(req, resolver); err != nil {
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
	if err := model.prepareGRPCRequest(req, resolver); err != nil {
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
	if err := model.prepareGRPCRequest(req, resolver); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.Target != "api.example.com:8443" {
		t.Fatalf("expected target to drop grpcs scheme, got %q", req.GRPC.Target)
	}
	if !req.GRPC.PlaintextSet || req.GRPC.Plaintext {
		t.Fatalf("expected secure scheme to enforce TLS, got plaintext=%v set=%v", req.GRPC.Plaintext, req.GRPC.PlaintextSet)
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
	if err := model.prepareGRPCRequest(req, vars.NewResolver()); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.Target != "localhost:9000/service?alt=blue" {
		t.Fatalf("expected query to be preserved, got %q", req.GRPC.Target)
	}
}

func TestPrepareGRPCRequestExpandsDescriptorSet(t *testing.T) {
	resolver := vars.NewResolver(vars.NewMapProvider("doc", map[string]string{"grpc.descriptor": "./testdata/example.protoset"}))
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			Target:        "localhost:50051",
			FullMethod:    "/pkg.Svc/Call",
			DescriptorSet: "{{grpc.descriptor}}",
		},
	}

	var model Model
	if err := model.prepareGRPCRequest(req, resolver); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.DescriptorSet != "./testdata/example.protoset" {
		t.Fatalf("expected descriptor set to be expanded, got %q", req.GRPC.DescriptorSet)
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
	if model.responseLatest == nil || !strings.Contains(model.responseLatest.pretty, "pre-request script") {
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

	cmd := model.executeRequest(doc, req, model.cfg.HTTPOptions)
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

func TestEnsureOAuthSetsAuthorizationHeader(t *testing.T) {
	var calls int32
	var lastAuth string
	var lastForm url.Values

	model := Model{
		cfg:     Config{EnvironmentName: "dev"},
		oauth:   oauth.NewManager(nil),
		globals: newGlobalStore(),
	}

	model.oauth.SetRequestFunc(func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
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
			Body:       []byte(`{"access_token":"token-basic","token_type":"Bearer","expires_in":3600}`),
			Headers:    http.Header{},
		}, nil
	})

	auth := &restfile.AuthSpec{Type: "oauth2", Params: map[string]string{
		"token_url":     "https://auth.local/token",
		"client_id":     "client",
		"client_secret": "secret",
		"scope":         "read",
	}}
	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}
	resolver := vars.NewResolver()
	if err := model.ensureOAuth(req, resolver, httpclient.Options{}, time.Second); err != nil {
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
	if err := model.ensureOAuth(req2, resolver, httpclient.Options{}, time.Second); err != nil {
		t.Fatalf("ensureOAuth second: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected cached token to prevent additional calls, got %d", calls)
	}
}

func TestEnsureOAuthSkipsWhenHeaderPresent(t *testing.T) {
	called := int32(0)
	model := Model{
		cfg:     Config{EnvironmentName: "dev"},
		oauth:   oauth.NewManager(nil),
		globals: newGlobalStore(),
	}
	model.oauth.SetRequestFunc(func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
		atomic.AddInt32(&called, 1)
		return &httpclient.Response{Status: "200", StatusCode: 200, Body: []byte(`{"access_token":"x"}`), Headers: http.Header{}}, nil
	})
	req := &restfile.Request{
		Headers: http.Header{"Authorization": {"Bearer manual"}},
		Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{Type: "oauth2", Params: map[string]string{
			"token_url": "https://auth.local/token",
		}}},
	}
	if err := model.ensureOAuth(req, vars.NewResolver(), httpclient.Options{}, time.Second); err != nil {
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

func TestApplyCapturesStoresValues(t *testing.T) {
	model := Model{
		cfg:      Config{EnvironmentName: "dev"},
		globals:  newGlobalStore(),
		fileVars: newFileStore(),
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
				{Scope: restfile.CaptureScopeGlobal, Name: "authToken", Expression: "Bearer {{response.json.token}}", Secret: true},
				{Scope: restfile.CaptureScopeFile, Name: "lastTrace", Expression: "{{response.headers.X-Trace}}", Secret: false},
				{Scope: restfile.CaptureScopeRequest, Name: "recentStatus", Expression: "{{response.status}}", Secret: false},
			},
		},
	}

	resolver := model.buildResolver(doc, req, nil)
	var captures captureResult
	if err := model.applyCaptures(doc, req, resolver, resp, nil, &captures); err != nil {
		t.Fatalf("applyCaptures: %v", err)
	}

	if _, ok := captures.requestVars["recentstatus"]; !ok {
		t.Fatalf("expected request capture to be recorded: %+v", captures.requestVars)
	}
	if _, ok := captures.fileVars["lasttrace"]; !ok {
		t.Fatalf("expected file capture to be recorded: %+v", captures.fileVars)
	}

	snapshot := model.globals.snapshot("dev")
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
	varsWithReq := model.collectVariables(doc, req)
	if varsWithReq["recentStatus"] != "200 OK" {
		t.Fatalf("expected request capture to be available in collected vars, got %q", varsWithReq["recentStatus"])
	}

	store := model.fileVars.snapshot("dev", "./sample.http")
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
	vars := model.collectVariables(freshDoc, nil)
	if vars["lastTrace"] != "abc" {
		t.Fatalf("expected file capture to be applied via runtime store, got %q", vars["lastTrace"])
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
	if err := model.applyCaptures(nil, req, nil, resp, stream, &captures); err != nil {
		t.Fatalf("applyCaptures stream: %v", err)
	}
	if len(req.Variables) == 0 || req.Variables[len(req.Variables)-1].Value != "change" {
		t.Fatalf("expected last event to be change, got %+v", req.Variables)
	}
}

func TestApplyCapturesWithStreamData(t *testing.T) {
	model := Model{
		cfg:      Config{EnvironmentName: "dev"},
		globals:  newGlobalStore(),
		fileVars: newFileStore(),
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
				{Scope: restfile.CaptureScopeRequest, Name: "streamKind", Expression: "{{stream.kind}}"},
				{Scope: restfile.CaptureScopeFile, Name: "received", Expression: "{{stream.summary.receivedCount}}"},
				{Scope: restfile.CaptureScopeGlobal, Name: "lastMessage", Expression: "{{stream.events[1].text}}"},
			},
		},
	}

	doc := &restfile.Document{Path: "./stream.http"}
	resolver := model.buildResolver(doc, req, nil)
	var captures captureResult
	if err := model.applyCaptures(doc, req, resolver, resp, streamInfo, &captures); err != nil {
		t.Fatalf("applyCaptures stream: %v", err)
	}

	vars := model.collectVariables(doc, req)
	if vars["streamKind"] != "websocket" {
		t.Fatalf("expected stream kind capture, got %q", vars["streamKind"])
	}
	if len(doc.Variables) == 0 || doc.Variables[0].Value != "2" {
		t.Fatalf("expected file capture for received count, got %+v", doc.Variables)
	}
	snapshot := model.globals.snapshot("dev")
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
		cfg:     Config{EnvironmentName: "dev"},
		globals: newGlobalStore(),
		doc: &restfile.Document{
			Globals: []restfile.Variable{
				{Name: "docVar", Value: "foo"},
				{Name: "secretDoc", Value: "bar", Secret: true},
			},
		},
	}
	model.globals.set("dev", "token", "secretValue", true)
	model.globals.set("dev", "refresh", "xyz", false)

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
		cfg:     Config{EnvironmentName: "dev"},
		globals: newGlobalStore(),
	}
	model.globals.set("dev", "token", "value", false)
	if snap := model.globals.snapshot("dev"); len(snap) == 0 {
		t.Fatalf("expected snapshot to contain entries before clearing")
	}
	model.clearGlobalValues()
	if snap := model.globals.snapshot("dev"); len(snap) != 0 {
		t.Fatalf("expected globals to be cleared, got %v", snap)
	}
	if !strings.Contains(model.statusMessage.text, "Cleared globals") {
		t.Fatalf("expected confirmation message, got %q", model.statusMessage.text)
	}
	if model.statusMessage.level != statusInfo {
		t.Fatalf("expected info level, got %v", model.statusMessage.level)
	}
}
