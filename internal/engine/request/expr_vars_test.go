package request

import (
	"net/http"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestExprVarsReadTheExpandedValue(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @request id {{$randomInt(7, 7)}}
# @request derived {{= vars.get("id") + "-x" }}
GET http://example.test/{{derived}}
`)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := sent.wire.URL.Path; got != "/7-x" {
		t.Fatalf("path = %q, want /7-x", got)
	}
}

func TestExprVarsShareThePlaceholderValue(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @request id {{$uuid}}
GET http://example.test
X-Id: {{id}}
X-Get: {{= vars.get("id") }}
X-Member: {{= vars.id }}
X-Index: {{= vars["id"] }}
X-Require: {{= vars.require("id") }}
`)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	id := sent.wire.Header.Get("X-Id")
	for _, h := range []string{"X-Get", "X-Member", "X-Index", "X-Require"} {
		if got := sent.wire.Header.Get(h); got != id {
			t.Fatalf("%s = %q, want the X-Id value %q", h, got, id)
		}
	}
}

func TestExprVarsSelfReadIsACycle(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @request a {{= vars.get("a") }}-x
GET http://example.test/{{a}}
`)
	eng, st := newStubEngine(t)

	res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if st.wire != nil {
		t.Fatalf("sent %q, want nothing", st.wire.URL.Path)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "variable cycle") {
		t.Fatalf("result error = %v, want a variable cycle", res.Err)
	}
}

func TestCaptureExprVarsReadTheSentValue(t *testing.T) {
	req := &restfile.Request{
		Method:    http.MethodGet,
		URL:       "http://example.test",
		Headers:   http.Header{"X-Id": []string{"{{id}}"}},
		Variables: []restfile.Variable{{Name: "id", Value: "{{$uuid}}", Scope: directive.ScopeRequest}},
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      directive.ScopeRequest,
				Name:       "echo",
				Expression: `{{= vars.get("id") }}`,
				Mode:       restfile.CaptureExprModeTemplate,
			}},
		},
	}

	sent := sendRequest(t, nil, req, envWith(t, "dev", nil), ExecOptions{})
	if got, want := executedVar(t, sent.executed, "echo"), sent.wire.Header.Get("X-Id"); got != want {
		t.Fatalf("echo = %q, want the sent value %q", got, want)
	}
}

func TestPreRequestScriptsStillReadDeclaredText(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @request id {{$randomInt(7, 7)}}
GET http://example.test
`)
	req.Metadata.Scripts = append(req.Metadata.Scripts,
		rtsPre(`request.setHeader("X-Raw", str(vars.get("id") == "{{$randomInt(7, 7)}}"))`),
	)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := sent.wire.Header.Get("X-Raw"); got != "true" {
		t.Fatalf("X-Raw = %q, want the script to read the declared text", got)
	}
}

func TestExprVarsKeepConstantsOut(t *testing.T) {
	doc, req := parseDoc(t, `# @const id fixed
### one
# @name one
# @request id {{$randomInt(7, 7)}}
GET http://example.test
X-Get: {{= vars.get("id") }}
`)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := sent.wire.Header.Get("X-Get"); got == "fixed" {
		t.Fatal("vars exposed the @const value")
	}
}
