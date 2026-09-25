package request

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
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

func TestCaptureRTSVarsReadTheSentValue(t *testing.T) {
	req := &restfile.Request{
		Method:    http.MethodGet,
		URL:       "http://example.test",
		Headers:   http.Header{"X-Id": []string{"{{id}}"}},
		Variables: []restfile.Variable{{Name: "id", Value: "{{$uuid}}", Scope: directive.ScopeRequest}},
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      directive.ScopeRequest,
				Name:       "echo",
				Expression: `vars.get("id")`,
				Mode:       restfile.CaptureExprModeRTS,
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

// Templates see the @const. vars skips it and reads the request value.
func TestExprVarsKeepConstantsOut(t *testing.T) {
	const decl = `# @const id fixed
### one
# @name one
# @request id {{$randomInt(7, 7)}}
`

	t.Run("template", func(t *testing.T) {
		doc, req := parseDoc(t, decl+`GET http://example.test
X-Get: {{= vars.get("id") }}
`)
		sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
		if got := sent.wire.Header.Get("X-Get"); got != "7" {
			t.Fatalf("X-Get = %q, want 7", got)
		}
	})

	t.Run("capture", func(t *testing.T) {
		doc, req := parseDoc(t, decl+`# @capture request tpl {{= vars.get("id") }}
# @capture request expr vars.get("id")
GET http://example.test
`)
		sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
		for _, name := range []string{"tpl", "expr"} {
			if got := executedVar(t, sent.executed, name); got != "7" {
				t.Fatalf("%s = %q, want 7", name, got)
			}
		}
	})

	t.Run("assert", func(t *testing.T) {
		doc, req := parseDoc(t, decl+`# @assert vars.get("id") == "7"
GET http://example.test
`)
		eng, _ := newStubEngine(t)
		res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
		if err != nil {
			t.Fatalf("ExecuteWith() error = %v", err)
		}
		if len(res.Tests) != 1 || !res.Tests[0].Passed {
			t.Fatalf("asserts = %+v, want vars.get to read 7", res.Tests)
		}
	})

	t.Run("qualified constant", func(t *testing.T) {
		doc, req := parseDoc(t, `# @const request.id leaked
`+decl+`GET http://example.test
X-Get: {{= vars.get("id") }}
`)
		sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
		if got := sent.wire.Header.Get("X-Get"); got != "7" {
			t.Fatalf("X-Get = %q, want 7", got)
		}
	})
}

func TestAssertVarsReadTheSentValue(t *testing.T) {
	doc, req := parseDoc(t, `### create
# @name create
# @request id {{$uuid}}
# @assert response.json().id == vars.get("id")
POST http://example.test/orders
Content-Type: application/json

{"id": "{{id}}"}
`)
	res, err := newEchoEngine().ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err != nil || res.ScriptErr != nil {
		t.Fatalf("result error = %v, script error = %v", res.Err, res.ScriptErr)
	}
	if len(res.Tests) != 1 || !res.Tests[0].Passed {
		t.Fatalf("asserts = %+v, want vars.get to read the id the request sent", res.Tests)
	}
}

func TestPollVarsReadTheSentValue(t *testing.T) {
	doc, req := parseDoc(t, `### create
# @name create
# @request id {{$uuid}}
# @poll every=10ms timeout=200ms until=response.json().id == vars.get("id")
POST http://example.test/orders
Content-Type: application/json

{"id": "{{id}}"}
`)

	res, err := newEchoEngine().ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err != nil {
		t.Fatalf("result error = %v, want the first response to match", res.Err)
	}
}

func newEchoEngine() *Engine {
	echo := httpx.NewClientWithOptions(
		httpx.WithHTTPFactory(func(httpx.Options) (*http.Client, error) {
			return &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						return nil, err
					}
					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": {"application/json"}},
						Body:       io.NopCloser(bytes.NewReader(body)),
						Request:    r,
					}, nil
				}),
			}, nil
		}),
	)
	return New(engcfg.Config{Client: echo}, nil)
}
