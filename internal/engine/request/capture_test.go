package request

import (
	"context"
	"net/http"
	"strings"
	"testing"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func newCaptureEngine(env string) *Engine {
	return New(engcfg.Config{EnvironmentName: env}, rtrun.New(rtrun.Config{}))
}

func TestApplyCapturesStoresValues(t *testing.T) {
	eng := newCaptureEngine("dev")

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

	resolver := eng.buildResolver(context.Background(), doc, req, "", "", nil, nil)
	var captures captureResult
	if err := eng.applyCaptures(captureRun{
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

	snapshot := eng.rt.Globals().Snapshot("dev")
	if len(snapshot) != 1 {
		t.Fatalf("expected one global, got %d", len(snapshot))
	}
	var entry rtrun.GlobalValue
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
	varsWithReq := eng.collectVariables(doc, req, "")
	if varsWithReq["recentStatus"] != "200 OK" {
		t.Fatalf(
			"expected request capture to be available in collected vars, got %q",
			varsWithReq["recentStatus"],
		)
	}

	store := eng.rt.Files().Snapshot("dev", "./sample.http")
	if len(store) != 1 {
		t.Fatalf("expected one stored file variable, got %d", len(store))
	}
	var stored rtrun.FileValue
	for _, entry := range store {
		stored = entry
	}
	if stored.Name != "lastTrace" || stored.Value != "abc" {
		t.Fatalf("unexpected stored file capture %+v", stored)
	}

	// simulate a fresh parse of the document (no baked-in variables)
	freshDoc := &restfile.Document{Path: "./sample.http"}
	vars := eng.collectVariables(freshDoc, nil, "")
	if vars["lastTrace"] != "abc" {
		t.Fatalf("expected file capture to be applied via runtime store, got %q", vars["lastTrace"])
	}
}

func TestApplyCapturesEvaluatesRSTExpressions(t *testing.T) {
	eng := newCaptureEngine("dev")

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
	if err := eng.applyCaptures(captureRun{
		doc:  doc,
		req:  req,
		resp: resp,
		out:  &captures,
	}); err != nil {
		t.Fatalf("applyCaptures rst: %v", err)
	}

	gl := eng.rt.Globals().Snapshot("dev")
	if len(gl) != 1 {
		t.Fatalf("expected one global capture, got %d", len(gl))
	}
	var g rtrun.GlobalValue
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
	eng := newCaptureEngine("")
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

	if err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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

	err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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

	err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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

	err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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
	if err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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

	err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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

	if err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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

	err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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

	if err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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

	err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("")
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

	err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("dev")

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
	if err := eng.applyCaptures(captureRun{
		doc:  doc,
		req:  req,
		resp: resp,
		out:  &captures,
		env:  "stage",
	}); err != nil {
		t.Fatalf("applyCaptures stage: %v", err)
	}

	if len(eng.rt.Globals().Snapshot("dev")) != 0 {
		t.Fatalf("expected no globals in dev env after stage capture")
	}
	stageGlobals := eng.rt.Globals().Snapshot("stage")
	if len(stageGlobals) != 1 {
		t.Fatalf("expected one global in stage, got %d", len(stageGlobals))
	}

	devStore := eng.rt.Files().Snapshot("dev", "./capture-env.http")
	if len(devStore) != 0 {
		t.Fatalf("expected no file captures in dev store")
	}

	stageStore := eng.rt.Files().Snapshot("stage", "./capture-env.http")
	if len(stageStore) != 1 {
		t.Fatalf("expected one file capture in stage store, got %d", len(stageStore))
	}
}

func TestApplyCapturesStreamNegativeIndex(t *testing.T) {
	eng := newCaptureEngine("")
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
	if err := eng.applyCaptures(captureRun{
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
	eng := newCaptureEngine("dev")

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
	resolver := eng.buildResolver(context.Background(), doc, req, "", "", nil, nil)
	var captures captureResult
	if err := eng.applyCaptures(captureRun{
		doc:    doc,
		req:    req,
		res:    resolver,
		resp:   resp,
		stream: streamInfo,
		out:    &captures,
	}); err != nil {
		t.Fatalf("applyCaptures stream: %v", err)
	}

	vars := eng.collectVariables(doc, req, "")
	if vars["streamKind"] != "websocket" {
		t.Fatalf("expected stream kind capture, got %q", vars["streamKind"])
	}
	if len(doc.Variables) == 0 || doc.Variables[0].Value != "2" {
		t.Fatalf("expected file capture for received count, got %+v", doc.Variables)
	}
	snapshot := eng.rt.Globals().Snapshot("dev")
	if len(snapshot) != 1 {
		t.Fatalf("expected one global capture, got %d", len(snapshot))
	}
	var globalEntry rtrun.GlobalValue
	for _, value := range snapshot {
		globalEntry = value
	}
	if globalEntry.Value != "world" {
		t.Fatalf("expected last message capture to be world, got %q", globalEntry.Value)
	}
}
