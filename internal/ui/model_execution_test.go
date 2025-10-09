package ui

import (
	"context"
	"encoding/base64"
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

	resp := &httpclient.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Headers: http.Header{
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
	if err := model.applyCaptures(doc, req, resolver, resp, &captures); err != nil {
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
