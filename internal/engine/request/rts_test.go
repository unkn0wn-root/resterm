package request

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rtshost"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
	"google.golang.org/grpc/codes"
)

type testMockInspector struct{}

func (testMockInspector) Count(context.Context, mock.RequestPattern) (uint64, error) {
	return 0, nil
}

func TestRTSExtensionsBindMockAndYieldToLocals(t *testing.T) {
	e := New(engcfg.Config{MockInspector: testMockInspector{}}, nil)
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}

	bound, err := e.re.Eval(
		context.Background(),
		rtshost.Runtime{Extensions: e.rtsExtensions()},
		"mock",
		pos,
	)
	if err != nil {
		t.Fatalf("eval mock: %v", err)
	}
	if bound.K != rts.VObj {
		t.Fatalf("mock = %+v, want the inspector object", bound)
	}

	rt := rtshost.Runtime{
		Extensions: e.rtsExtensions(),
		Locals:     rts.Local("mock", rts.Str("loop item")),
	}
	shadowed, err := e.re.Eval(context.Background(), rt, "mock", pos)
	if err != nil {
		t.Fatalf("eval shadowed mock: %v", err)
	}
	if shadowed.K != rts.VStr || shadowed.S != "loop item" {
		t.Fatalf("mock = %+v, want the loop item to win", shadowed)
	}
}

func TestRTSExtensionsAreEmptyWithoutAnInspector(t *testing.T) {
	e := New(engcfg.Config{}, nil)
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}

	_, err := e.re.Eval(
		context.Background(),
		rtshost.Runtime{Extensions: e.rtsExtensions()},
		"mock",
		pos,
	)
	if err == nil || !strings.Contains(err.Error(), `undefined name "mock"`) {
		t.Fatalf("eval mock without an inspector: error = %v, want it undefined", err)
	}
}

func TestRunRTSPreRequestErrorRendersInlineSource(t *testing.T) {
	eng := New(engcfg.Config{}, nil)
	src := `### RTS
# @rts pre-request
> request.setHeader("X", missing.value)
GET https://example.com
`
	doc := parser.Parse("sample.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	_, err := eng.runRTSPreRequest(
		context.Background(),
		doc,
		doc.Requests[0],
		testEnv(""),
		"",
		evalScope{},
		rts.Locals{},
		nil,
	)
	if err == nil {
		t.Fatalf("expected rts error")
	}

	out := diag.Render(diag.WrapAs(diag.ClassScript, err, "pre-request rts script"))
	checks := []string{
		`error[script]: undefined name "missing"`,
		"--> sample.http:3:26",
		`   3 | > request.setHeader("X", missing.value)`,
		"Stack:",
		"  at sample.http:3:1 in @script pre-request",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("expected rendered error to contain %q:\n%s", want, out)
		}
	}
}

func TestRunRTSPreRequestRejectsResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "member",
			body: `request.setHeader("X", response.statusCode)`,
		},
		{
			name: "member behind try",
			body: `request.setHeader("X", (try response.statusCode).value ?? "none")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "### RTS\n# @rts pre-request\n> " + tt.body + "\nGET https://example.com\n"
			doc := parser.Parse("sample.http", []byte(src))
			if len(doc.Errors) != 0 {
				t.Fatalf("parse errors: %+v", doc.Errors)
			}

			eng := New(engcfg.Config{}, nil)
			_, err := eng.runRTSPreRequest(
				context.Background(),
				doc,
				doc.Requests[0],
				testEnv(""),
				"",
				evalScope{},
				rts.Locals{},
				nil,
			)
			if err == nil {
				t.Fatal("expected response to be rejected before the request")
			}
			if !strings.Contains(err.Error(), "use last for the previous response") {
				t.Fatalf("error = %v, want it to name last", err)
			}
		})
	}
}

func TestEvalForEachErrorCarriesSource(t *testing.T) {
	eng := New(engcfg.Config{SourceDiagnostics: true}, nil)
	src := `### Req
# @for-each item in missing.value
GET https://example.com
`
	doc := parser.Parse("sample.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	_, err := eng.EvalForEachItems(
		context.Background(),
		doc,
		doc.Requests[0],
		testEnv(""),
		"",
		ForEachSpec{Expr: "missing.value", Line: 2},
		nil,
		rts.Locals{},
	)
	if err == nil {
		t.Fatalf("expected for-each error")
	}

	out := diag.Render(err)
	checks := []string{
		`error[script]: undefined name "missing"`,
		"--> sample.http:2:1",
		"   2 | # @for-each item in missing.value", // source snippet, attached via rtsErr
		"in @for-each missing.value",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Fatalf("expected rendered for-each error to contain %q:\n%s", want, out)
		}
	}
}

func TestRTSRequestHeadersReachTheRuntimeWithCardinalityShape(t *testing.T) {
	e := New(engcfg.Config{}, nil)
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.test/path",
		Headers: http.Header{
			"Content-Type": {"application/json"},
			"X-Token":      {"abc"},
		},
	}

	rq, err := e.rtsReq(req)
	if err != nil {
		t.Fatalf("rtsReq: %v", err)
	}
	rt, err := e.buildRT(rtIn{req: req})
	if err != nil {
		t.Fatalf("buildRT: %v", err)
	}
	tests := map[string]string{
		`request.header("content-type")`: "application/json",
		`request.header("Content-Type")`: "application/json",
		`request.header("x-token")`:      "abc",
		`request.headers["x-token"]`:     "abc",
	}
	for src, want := range tests {
		v, err := e.re.Eval(context.Background(), rt, src, pos)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if v.K != rts.VStr || v.S != want {
			t.Errorf("%s = %+v, want %q", src, v, want)
		}
	}

	want := []string{"application/json"}
	if got := rq.Headers["content-type"]; !slices.Equal(got, want) {
		t.Fatalf("Request.Headers[content-type] = %q, want %q", got, want)
	}
}

func TestRTSRequestRejectsEquivalentHeaderNames(t *testing.T) {
	e := New(engcfg.Config{}, nil)
	req := &restfile.Request{Headers: http.Header{
		"Content-Type": {"application/json"},
		"content-type": {"text/plain"},
	}}
	if _, err := e.rtsReq(req); err == nil || !strings.Contains(err.Error(), "same HTTP header") {
		t.Fatalf("rtsReq error = %v, want a header collision", err)
	}
}

func TestRTSRequestQueryAllowsTemplatedTargets(t *testing.T) {
	e := New(engcfg.Config{}, nil)
	rq, err := e.rtsReq(&restfile.Request{URL: "{{host}}:50051"})
	if err != nil {
		t.Fatalf("rtsReq without query: %v", err)
	}
	if len(rq.Query) != 0 {
		t.Fatalf("Request.Query = %v, want empty", rq.Query)
	}

	req := &restfile.Request{URL: "{{host?fallback=true}}:50051/path?a=1&a=2&b=3&q={{a&b}}#fragment"}
	rt, err := e.buildRT(rtIn{req: req})
	if err != nil {
		t.Fatalf("buildRT: %v", err)
	}
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}
	tests := []struct {
		name string
		src  string
		kind rts.VKind
		n    int
		want string
	}{
		{name: "one", src: "request.query.b", kind: rts.VStr, want: "3"},
		{name: "multiple", src: "request.query.a", kind: rts.VList, n: 2},
		{name: "template separators", src: "request.query.q", kind: rts.VStr, want: "{{a&b}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := e.re.Eval(context.Background(), rt, tt.src, pos)
			if err != nil {
				t.Fatalf("Eval: %v", err)
			}
			if v.K != tt.kind || len(v.L) != tt.n || v.S != tt.want {
				t.Fatalf("%s = %+v, want kind %v, len %d, string %q", tt.src, v, tt.kind, tt.n, tt.want)
			}
		})
	}
}

func testEnvValues(t *testing.T, name string, values map[string]string) vars.Environment {
	t.Helper()
	cat, err := vars.NewCatalog(vars.EnvironmentSet{name: values})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	sel, err := cat.Select(name, nil)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	env, err := cat.Resolve(sel)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return env
}

// The selection label reaches the runtime beside the values, so an environment
// variable may still be called NAME and env.meta.name reports the selection.
func TestRTSEnvKeepsTheSelectionOutOfTheValues(t *testing.T) {
	env := testEnvValues(t, "dev", map[string]string{"NAME": "payments-service"})
	e := New(engcfg.Config{}, nil)
	rt, err := e.buildRT(rtIn{env: env})
	if err != nil {
		t.Fatalf("buildRT: %v", err)
	}
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}

	tests := map[string]string{
		`env.meta.name`:   "dev",
		`env.NAME`:        "payments-service",
		`env.get("name")`: "payments-service",
	}
	for src, want := range tests {
		v, err := e.re.Eval(context.Background(), rt, src, pos)
		if err != nil {
			t.Errorf("%s: %v", src, err)
			continue
		}
		if v.K != rts.VStr || v.S != want {
			t.Errorf("%s = %+v, want %q", src, v, want)
		}
	}
}

// Environment construction resolves its own layering before the strict RTS
// boundary receives the selected values.
func TestRTSEnvSelectionHasUniqueNames(t *testing.T) {
	e := New(engcfg.Config{}, nil)
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}

	// Resolve the environment inside the loop: the collapse is what has to be
	// stable, and reusing one runtime would only re-read a map already keyed.
	for range 16 {
		env := testEnvValues(t, "dev", map[string]string{" token": "a", "token": "b"})
		rt, err := e.buildRT(rtIn{env: env})
		if err != nil {
			t.Fatalf("buildRT: %v", err)
		}
		v, err := e.re.Eval(context.Background(), rt, `env.get("token")`, pos)
		if err != nil {
			t.Fatalf(`env.get("token"): %v`, err)
		}
		if v.K != rts.VStr || v.S != "b" {
			t.Fatalf(`env.get("token") = %+v, want "b" on every run`, v)
		}
	}
}

// A caller-supplied variable map is validated rather than silently choosing a
// winner between equivalent names.
func TestRTSRejectsAmbiguousCallerSuppliedVars(t *testing.T) {
	e := New(engcfg.Config{}, nil)
	pos := rts.Pos{Path: "test", Line: 1, Col: 1}

	for range 16 {
		_, err := e.EvalValue(context.Background(), EvalInput{
			Expr: `vars.get("token")`,
			Pos:  pos,
			Vars: map[string]string{"Token": "a", "token": "b"},
		})
		if err == nil || !strings.Contains(err.Error(), "same name") {
			t.Fatalf("EvalValue error = %v, want an ambiguous-name error", err)
		}
	}
}

// A map that keeps its names apart is handed to the runtime as it is, so the
// writes a script makes through the mutator stay visible to the caller.
func TestRTSKeepsCallerVarMapsUntouched(t *testing.T) {
	vv := map[string]string{"token": "a"}
	got := newEvalScope(vv, vars.Globals{}).vars
	got["added"] = "b"
	if vv["added"] != "b" {
		t.Fatalf("newEvalScope copied a map that keeps its names apart: %#v", got)
	}

	amb := map[string]string{"Token": "a", "token": "b"}
	if out := newEvalScope(amb, vars.Globals{}).vars; len(out) != 2 || len(amb) != 2 {
		t.Fatalf("newEvalScope(ambiguous) = %#v, want the input left intact", out)
	}
}

// The pre-request runtime is built on its own path, so it has to carry the
// selection too.
func TestRTSPreRequestSeesTheSelection(t *testing.T) {
	src := `### RTS
# @rts pre-request
> vars.set("label", env.meta.name + "/" + env.get("NAME"))
GET https://example.com
`
	doc := parser.Parse("sample.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d", len(doc.Requests))
	}

	e := New(engcfg.Config{}, nil)
	out, err := e.runRTSPreRequest(
		context.Background(),
		doc,
		doc.Requests[0],
		testEnvValues(t, "dev", map[string]string{"NAME": "payments-service"}),
		"",
		evalScope{},
		rts.Locals{},
		nil,
	)
	if err != nil {
		t.Fatalf("pre-request: %v", err)
	}
	got, ok := out.Variables.Get("label")
	if !ok || got != "dev/payments-service" {
		t.Fatalf("label = %q (set %v), want %q", got, ok, "dev/payments-service")
	}
}

func TestRTSGRPCExposesEncodedTrailers(t *testing.T) {
	raw := string([]byte{0x00, 0x01, 0xff})
	resp := &grpcx.Response{
		StatusCode:    codes.InvalidArgument,
		StatusMessage: "invalid page size",
		Headers:       map[string][]string{"x-shared": {"header"}},
		Trailers: map[string][]string{
			"x-shared":  {"trailer"},
			"trace-bin": {raw},
		},
	}

	rr, err := rtsGRPC(resp)
	if err != nil {
		t.Fatalf("rtsGRPC: %v", err)
	}
	h := rr.Headers
	if got := h["x-shared"]; len(got) != 1 || got[0] != "header" {
		t.Fatalf("x-shared = %q, want the header value to survive the trailer", got)
	}
	if got := h["grpc-trailer-x-shared"]; len(got) != 1 || got[0] != "trailer" {
		t.Fatalf("Grpc-Trailer-x-shared = %q, want the trailer under its prefix", got)
	}
	want := base64.RawStdEncoding.EncodeToString([]byte(raw))
	if got := h["grpc-trailer-trace-bin"]; len(got) != 1 || got[0] != want {
		t.Fatalf("Grpc-Trailer-trace-bin = %q, want %q", got, want)
	}
}

func TestRTSResponseAdaptersRejectInvalidHeaders(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "http",
			run: func() error {
				_, err := rtsHTTP(&httpx.Response{Headers: http.Header{"X Bad": {"x"}}})
				return err
			},
		},
		{
			name: "grpc",
			run: func() error {
				_, err := rtsGRPC(&grpcx.Response{Headers: map[string][]string{"x bad": {"x"}}})
				return err
			},
		},
		{
			name: "script",
			run: func() error {
				_, err := rtsScriptResp(&scripts.Response{Header: http.Header{"X Bad": {"x"}}})
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if err == nil || !strings.Contains(err.Error(), "not an HTTP header name") {
				t.Fatalf("adapter error = %v, want an invalid-header error", err)
			}
		})
	}
}

func TestRTSRuntimeRejectsARequestWithABadHeaderName(t *testing.T) {
	src := "### RTS\nGET https://example.com\nX Token: raw\nX-Ok: yes\n"
	doc := parser.Parse("sample.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d", len(doc.Requests))
	}

	e := New(engcfg.Config{}, nil)
	_, err := e.buildRT(rtIn{req: doc.Requests[0]})
	if err == nil || !strings.Contains(err.Error(), "not an HTTP header name") {
		t.Fatalf("buildRT error = %v, want an invalid-header error", err)
	}
}

// Invalid headers fail at the RTS host boundary, so neither a script nor the
// transport can observe a request shape that HTTP itself refuses.
func TestBadHeaderNameFailsBeforeDispatch(t *testing.T) {
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	src := "### RTS\nGET " + srv.URL + "\nX Token: raw\nX-Ok: yes\n"
	doc := parser.Parse("sample.http", []byte(src))
	e := New(engcfg.Config{}, nil)

	res, err := e.Execute(doc, doc.Requests[0], testEnv(""))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), `request headers: "X Token" is not an HTTP header name`) {
		t.Fatalf("res.Err = %v, want an RTS request-boundary error", res.Err)
	}
	if called.Load() {
		t.Fatal("invalid request reached the transport")
	}
}
