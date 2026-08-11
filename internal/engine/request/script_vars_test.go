package request

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func rtsPre(body string) restfile.ScriptBlock {
	return restfile.ScriptBlock{Kind: "pre-request", Lang: "rts", Body: body}
}

func jsPre(body string) restfile.ScriptBlock {
	return restfile.ScriptBlock{Kind: "pre-request", Lang: "js", Body: body}
}

func envWith(t *testing.T, name string, values map[string]string) vars.Environment {
	t.Helper()

	cat, err := vars.NewCatalog(vars.EnvironmentSet{name: values})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	sel, err := cat.Select(name, nil)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	env, err := cat.Resolve(sel)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return env
}

type sentRequest struct {
	wire     *http.Request
	executed *restfile.Request
}

type stubTransport struct{ wire *http.Request }

func (s *stubTransport) client() *httpx.Client {
	return httpx.NewClientWithOptions(
		httpx.WithHTTPFactory(func(httpx.Options) (*http.Client, error) {
			return &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					s.wire = r
					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader("ok")),
						Request:    r,
					}, nil
				}),
			}, nil
		}),
	)
}

func newStubEngine(t *testing.T) (*Engine, *stubTransport) {
	t.Helper()

	st := &stubTransport{}
	return New(engcfg.Config{Client: st.client()}, nil), st
}

func sendRequest(
	t *testing.T,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
	opt ExecOptions,
) sentRequest {
	t.Helper()

	eng, st := newStubEngine(t)
	return sendWith(t, eng, st, doc, req, env, opt)
}

func sendWith(
	t *testing.T,
	eng *Engine,
	st *stubTransport,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
	opt ExecOptions,
) sentRequest {
	t.Helper()

	res, err := eng.ExecuteWith(doc, req, env, opt)
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err != nil {
		t.Fatalf("ExecuteWith() result error = %v", res.Err)
	}
	if st.wire == nil {
		t.Fatal("request was never sent")
	}
	return sentRequest{wire: st.wire, executed: res.Executed}
}

func TestJSPreRequestReadsVariablesCaseInsensitively(t *testing.T) {
	doc := &restfile.Document{
		Path:      "scripts.http",
		Variables: []restfile.Variable{{Name: "token", Value: "file-token"}},
	}
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{jsPre(`
				request.setHeader("X-Upper", vars.get("TOKEN"));
				request.setHeader("X-Mixed", vars.get("ToKeN"));
				request.setHeader("X-Has", String(vars.has("TOKEN")));
			`)},
		},
	}

	sent := sendRequest(t, doc, req, envWith(t, "dev", map[string]string{"TOKEN": "env-token"}), ExecOptions{})
	for _, name := range []string{"X-Upper", "X-Mixed"} {
		if got := sent.wire.Header.Get(name); got != "file-token" {
			t.Fatalf("%s = %q, want %q", name, got, "file-token")
		}
	}
	if got := sent.wire.Header.Get("X-Has"); got != "true" {
		t.Fatalf("X-Has = %q, want %q", got, "true")
	}
}

func TestJSPreRequestSeesRTSValueOverWorkflowValue(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{
				rtsPre(`vars.set("token", "rts")`),
				jsPre(`request.setHeader("X-Seen", vars.get("token"));`),
			},
		},
	}

	sent := sendRequest(t, nil, req, testEnv(""), ExecOptions{
		Extra: map[string]string{"token": "workflow"},
	})
	if got := sent.wire.Header.Get("X-Seen"); got != "rts" {
		t.Fatalf("X-Seen = %q, want %q", got, "rts")
	}
}

func TestScriptCaseVariantsResolveToTheLastWrite(t *testing.T) {
	tests := []struct {
		name string
		rts  string
		js   string
	}{
		{name: "rts lower then js upper", rts: "token", js: "TOKEN"},
		{name: "rts upper then js lower", rts: "TOKEN", js: "token"},
		{name: "rts mixed then js mixed", rts: "ToKeN", js: "toKEN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{
				Method: http.MethodGet,
				URL:    "http://example.test",
				Headers: http.Header{
					"X-Template": []string{"{{token}}"},
				},
				Metadata: restfile.RequestMetadata{
					Scripts: []restfile.ScriptBlock{
						rtsPre(`vars.set("` + tt.rts + `", "from-rts")`),
						jsPre(`vars.set("` + tt.js + `", "from-js");
							request.setHeader("X-Read", vars.get("token"));`),
					},
				},
			}

			sent := sendRequest(t, nil, req, testEnv(""), ExecOptions{})
			if got := sent.wire.Header.Get("X-Read"); got != "from-js" {
				t.Fatalf("X-Read = %q, want %q", got, "from-js")
			}
			if got := sent.wire.Header.Get("X-Template"); got != "from-js" {
				t.Fatalf("X-Template = %q, want %q", got, "from-js")
			}
		})
	}
}

func TestScriptRepeatedCaseVariantsKeepTheLastWrite(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Headers: http.Header{
			"X-Template": []string{"{{token}}"},
		},
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{jsPre(`
				vars.set("token", "first");
				vars.set("TOKEN", "second");
				vars.set("ToKeN", "third");
			`)},
		},
	}

	sent := sendRequest(t, nil, req, testEnv(""), ExecOptions{})
	if got := sent.wire.Header.Get("X-Template"); got != "third" {
		t.Fatalf("X-Template = %q, want %q", got, "third")
	}

	named := 0
	for _, v := range sent.executed.Variables {
		if vars.NameKey(v.Name) == "token" {
			named++
		}
	}
	if named != 1 {
		t.Fatalf("request holds %d forms of token, want 1: %#v", named, sent.executed.Variables)
	}
}

func TestRTSRepeatedCaseVariantsKeepTheLastWrite(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Headers: http.Header{
			"X-Template": []string{"{{token}}"},
		},
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{rtsPre(`vars.set("token", "first")
vars.set("TOKEN", "second")`)},
		},
	}

	sent := sendRequest(t, nil, req, testEnv(""), ExecOptions{})
	if got := sent.wire.Header.Get("X-Template"); got != "second" {
		t.Fatalf("X-Template = %q, want %q", got, "second")
	}
}

func TestDisplayResolverOmitsSecretsFromBothPaths(t *testing.T) {
	eng := New(engcfg.Config{}, nil)
	env := envWith(t, "dev", nil)
	eng.rt.Globals().Set(env.Scope(), "gsecret", "global-secret", true)

	doc := &restfile.Document{
		Path: "secrets.http",
		Variables: []restfile.Variable{
			{Name: "fsecret", Value: "file-secret", Secret: true},
			{Name: "shown", Value: "visible"},
		},
		Globals: []restfile.Variable{{Name: "dsecret", Value: "doc-secret", Secret: true}},
	}
	req := &restfile.Request{
		Method:    http.MethodGet,
		URL:       "http://example.test",
		Variables: []restfile.Variable{{Name: "rsecret", Value: "request-secret", Secret: true}},
	}

	res := eng.DisplayResolver(context.Background(), doc, req, env, "", rts.Locals{})
	if got, err := res.ExpandTemplates("{{shown}}"); err != nil || got != "visible" {
		t.Fatalf("{{shown}} = %q, %v, want %q", got, err, "visible")
	}
	for _, name := range []string{"fsecret", "dsecret", "rsecret", "gsecret"} {
		if got, err := res.ExpandTemplates("{{" + name + "}}"); err == nil {
			t.Fatalf("{{%s}} resolved to %q, want it withheld", name, got)
		}
		got, err := res.ExpandTemplates(`{{= vars.get("` + name + `") ?? "withheld" }}`)
		if err != nil {
			t.Fatalf(`vars.get(%q) error = %v`, name, err)
		}
		if got != "withheld" {
			t.Fatalf(`vars.get(%q) = %q, want it withheld`, name, got)
		}
	}
}
