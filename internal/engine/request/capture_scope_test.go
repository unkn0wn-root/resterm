package request

import (
	"net/http"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func executedVar(t *testing.T, req *restfile.Request, name string) string {
	t.Helper()

	for _, v := range req.Variables {
		if vars.NameKey(v.Name) == vars.NameKey(name) {
			return v.Value
		}
	}
	t.Fatalf("variable %q not captured: %#v", name, req.Variables)
	return ""
}

func TestCaptureWritesReachEveryLaterReader(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      directive.ScopeGlobal,
					Name:       "first",
					Expression: `"one"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "viaVars",
					Expression: `vars.get("first") ?? "missing"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "viaExpr",
					Expression: `{{= vars.get("first") ?? "missing" }}`,
					Mode:       restfile.CaptureExprModeTemplate,
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "viaTpl",
					Expression: `{{first}}`,
					Mode:       restfile.CaptureExprModeTemplate,
				},
			},
		},
	}

	sent := sendRequest(t, nil, req, envWith(t, "dev", nil), ExecOptions{})
	for _, name := range []string{"viaVars", "viaExpr", "viaTpl"} {
		if got := executedVar(t, sent.executed, name); got != "one" {
			t.Fatalf("%s = %q, want %q", name, got, "one")
		}
	}
}

func TestRequestAndFileCapturesReachLaterCaptures(t *testing.T) {
	doc := &restfile.Document{Path: "cap.http"}
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      directive.ScopeRequest,
					Name:       "base",
					Expression: `"r1"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeFile,
					Name:       "fbase",
					Expression: `"f1"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "echoReq",
					Expression: `vars.get("base") ?? "missing"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "echoFile",
					Expression: `{{fbase}}`,
					Mode:       restfile.CaptureExprModeTemplate,
				},
			},
		},
	}

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := executedVar(t, sent.executed, "echoReq"); got != "r1" {
		t.Fatalf("echoReq = %q, want %q", got, "r1")
	}
	if got := executedVar(t, sent.executed, "echoFile"); got != "f1" {
		t.Fatalf("echoFile = %q, want %q", got, "f1")
	}
}

func TestCaptureRefreshKeepsSourcePrecedence(t *testing.T) {
	doc := &restfile.Document{Path: "cap.http"}
	req := &restfile.Request{
		Method:    http.MethodGet,
		URL:       "http://example.test",
		Variables: []restfile.Variable{{Name: "reqvar", Value: "from-request", Scope: directive.ScopeRequest}},
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{rtsPre(`vars.set("token", "from-script")`)},
			Captures: []restfile.CaptureSpec{
				{
					Scope:      directive.ScopeFile,
					Name:       "token",
					Expression: `"from-file"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeGlobal,
					Name:       "reqvar",
					Expression: `"from-global"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "readToken",
					Expression: `vars.get("token")`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "readReqvar",
					Expression: `{{reqvar}}`,
					Mode:       restfile.CaptureExprModeTemplate,
				},
			},
		},
	}

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := executedVar(t, sent.executed, "readToken"); got != "from-script" {
		t.Fatalf("readToken = %q, want the script write to win", got)
	}
	if got := executedVar(t, sent.executed, "readReqvar"); got != "from-request" {
		t.Fatalf("readReqvar = %q, want the request variable to win", got)
	}
}

func TestCapturedValuesStayLiteralInLaterCaptures(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      directive.ScopeRequest,
					Name:       "raw",
					Expression: `"{{token}}"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "echo",
					Expression: `{{raw}}`,
					Mode:       restfile.CaptureExprModeTemplate,
				},
			},
		},
	}

	sent := sendRequest(t, nil, req, envWith(t, "dev", nil), ExecOptions{})
	if got := executedVar(t, sent.executed, "echo"); got != "{{token}}" {
		t.Fatalf("echo = %q, want the captured text kept literal", got)
	}
}

func TestCaptureTemplatesKeepPinnedDynamicValues(t *testing.T) {
	req := &restfile.Request{
		Method:    http.MethodGet,
		URL:       "http://example.test",
		Headers:   http.Header{"X-Id": []string{"{{id}}"}},
		Variables: []restfile.Variable{{Name: "id", Value: "{{$uuid}}", Scope: directive.ScopeRequest}},
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      directive.ScopeGlobal,
					Name:       "stage",
					Expression: `"x"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "echo",
					Expression: `{{id}}`,
					Mode:       restfile.CaptureExprModeTemplate,
				},
			},
		},
	}

	sent := sendRequest(t, nil, req, envWith(t, "dev", nil), ExecOptions{})
	sentID := sent.wire.Header.Get("X-Id")
	if sentID == "" {
		t.Fatal("X-Id was not sent")
	}
	if got := executedVar(t, sent.executed, "echo"); got != sentID {
		t.Fatalf("echo = %q, want the sent value %q", got, sentID)
	}
}

func TestCaptureExpandsNestedDocumentGlobal(t *testing.T) {
	doc := &restfile.Document{
		Path: "caps.http",
		Globals: []restfile.Variable{
			{Name: "base", Value: "expanded", Scope: directive.ScopeGlobal},
			{Name: "nested", Value: "{{base}}", Scope: directive.ScopeGlobal},
		},
	}
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      directive.ScopeRequest,
				Name:       "echo",
				Expression: `{{nested}}`,
				Mode:       restfile.CaptureExprModeTemplate,
			}},
		},
	}

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := executedVar(t, sent.executed, "echo"); got != "expanded" {
		t.Fatalf("echo = %q, want %q", got, "expanded")
	}
}

func TestFailedCaptureBatchLeavesNoPartialState(t *testing.T) {
	eng := newCaptureEngine("dev")
	resp := &scripts.Response{
		Kind:   scripts.ResponseKindHTTP,
		Status: "200 OK",
		Code:   200,
		Body:   []byte(`{"token":"abc123"}`),
	}
	doc := &restfile.Document{Path: "./caps.http"}
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      directive.ScopeFile,
					Name:       "good",
					Expression: "{{response.json.token}}",
				},
				{
					Scope:      directive.ScopeRequest,
					Name:       "keep",
					Expression: "{{response.json.token}}",
				},
				{
					Scope:      directive.ScopeFile,
					Name:       "bad",
					Expression: "{{response.json.missing.x}}",
				},
			},
		},
		Settings: map[string]string{"capture.strict": "true"},
	}

	var out captureResult
	err := eng.applyCaptures(captureRun{
		doc:  doc,
		req:  req,
		resp: resp,
		out:  &out,
		env:  testEnv("dev"),
	})
	if err == nil {
		t.Fatal("expected the strict capture batch to fail")
	}
	if len(doc.Variables) != 0 {
		t.Fatalf("doc.Variables = %#v, want the failed batch to stage nothing", doc.Variables)
	}
	if len(req.Variables) != 0 {
		t.Fatalf("req.Variables = %#v, want the failed batch to stage nothing", req.Variables)
	}
	if store := eng.rt.Files().Snapshot(testEnv("dev").Scope(), doc.Path); len(store) != 0 {
		t.Fatalf("file store = %#v, want the failed batch to persist nothing", store)
	}
}
