package headless

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/reflection"
	"nhooyr.io/websocket"
)

func TestEngineExecuteRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	eng := New(engine.Config{})
	doc, req := testDocumentRequest(srv.URL)

	res, err := eng.ExecuteRequest(doc, req, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("unexpected request error: %v", res.Err)
	}
	if res.Response == nil || res.Response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response: %+v", res.Response)
	}
}

func TestEngineExecuteCompareProfileAndWorkflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	eng := New(engine.Config{})

	doc, compareReq := testDocumentRequest(srv.URL)
	compare, err := eng.ExecuteCompare(doc, compareReq, &restfile.CompareSpec{
		Environments: []string{"one", "two"},
		Baseline:     "one",
	}, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteCompare: %v", err)
	}
	if compare == nil || len(compare.Rows) != 2 {
		t.Fatalf("unexpected compare result: %+v", compare)
	}

	docProfile, profileReq := testDocumentRequest(srv.URL)
	profileReq.Metadata.Profile = &restfile.ProfileSpec{Count: 2}
	profile, err := eng.ExecuteProfile(docProfile, profileReq, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteProfile: %v", err)
	}
	if profile == nil || profile.Count != 2 {
		t.Fatalf("unexpected profile result: %+v", profile)
	}

	wf := &restfile.Workflow{
		Name: "smoke",
		Steps: []restfile.WorkflowStep{{
			Kind:  restfile.WorkflowStepKindRequest,
			Using: "ok",
		}},
	}
	out, err := eng.ExecuteWorkflow(doc, wf, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}
	if out == nil || len(out.Steps) != 1 {
		t.Fatalf("unexpected workflow result: %+v", out)
	}
}

func TestEngineExecuteRequestIsolatesCookiesPerEnvironment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set/dev":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "dev123", Path: "/"})
		case "/set/prod":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "prod456", Path: "/"})
		case "/echo":
			if cookie, err := r.Cookie("session"); err == nil {
				if _, err := fmt.Fprint(w, cookie.String()); err != nil {
					t.Fatalf("write echo response: %v", err)
				}
				return
			}
		}
	}))
	defer srv.Close()

	eng := New(engine.Config{})

	doc := &restfile.Document{Path: "cookies.http"}
	setDev := &restfile.Request{Method: http.MethodGet, URL: srv.URL + "/set/dev"}
	setProd := &restfile.Request{Method: http.MethodGet, URL: srv.URL + "/set/prod"}
	echo := &restfile.Request{Method: http.MethodGet, URL: srv.URL + "/echo"}

	if _, err := eng.ExecuteRequest(doc, setDev, testSelection("dev")); err != nil {
		t.Fatalf("set dev cookie: %v", err)
	}
	res, err := eng.ExecuteRequest(doc, echo, testSelection("dev"))
	if err != nil {
		t.Fatalf("echo dev cookie: %v", err)
	}
	if got := strings.TrimSpace(string(res.Response.Body)); got != "session=dev123" {
		t.Fatalf("expected dev cookie, got %q", got)
	}

	res, err = eng.ExecuteRequest(doc, echo, testSelection("prod"))
	if err != nil {
		t.Fatalf("echo prod cookie before set: %v", err)
	}
	if got := strings.TrimSpace(string(res.Response.Body)); got != "" {
		t.Fatalf("expected no prod cookie before set, got %q", got)
	}

	if _, err := eng.ExecuteRequest(doc, setProd, testSelection("prod")); err != nil {
		t.Fatalf("set prod cookie: %v", err)
	}
	res, err = eng.ExecuteRequest(doc, echo, testSelection("prod"))
	if err != nil {
		t.Fatalf("echo prod cookie: %v", err)
	}
	if got := strings.TrimSpace(string(res.Response.Body)); got != "session=prod456" {
		t.Fatalf("expected prod cookie, got %q", got)
	}

	res, err = eng.ExecuteRequest(doc, echo, testSelection("dev"))
	if err != nil {
		t.Fatalf("echo dev cookie after prod set: %v", err)
	}
	if got := strings.TrimSpace(string(res.Response.Body)); got != "session=dev123" {
		t.Fatalf("expected dev cookie to remain isolated, got %q", got)
	}
}

func TestWorkflowScriptErr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	eng := New(engine.Config{})
	doc, req := testDocumentRequest(srv.URL)
	req.Metadata.Scripts = []restfile.ScriptBlock{
		{
			Kind: "test",
			Body: `client.test("status", function() { tests.assert(response.statusCode === 200, "status code"); });`,
		},
		{
			Kind: "test",
			Body: `throw new Error("boom");`,
		},
	}

	reqRes, err := eng.ExecuteRequest(doc, req, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if reqRes.Err != nil {
		t.Fatalf("unexpected request error: %v", reqRes.Err)
	}
	if reqRes.ScriptErr == nil {
		t.Fatal("expected request script error")
	}
	if len(reqRes.Tests) == 0 {
		t.Fatal("expected passing tests from earlier script block")
	}
	for _, test := range reqRes.Tests {
		if !test.Passed {
			t.Fatalf("expected earlier tests to pass, got %+v", reqRes.Tests)
		}
	}

	wf := &restfile.Workflow{
		Name: "smoke",
		Steps: []restfile.WorkflowStep{{
			Kind:  restfile.WorkflowStepKindRequest,
			Using: "ok",
		}},
	}

	out, err := eng.ExecuteWorkflow(doc, wf, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}
	if out == nil || len(out.Steps) != 1 {
		t.Fatalf("unexpected workflow result: %+v", out)
	}
	if out.Success {
		t.Fatalf("expected workflow to fail on script error, got %+v", out)
	}
	if out.Steps[0].Success {
		t.Fatalf("expected workflow step to fail on script error, got %+v", out.Steps[0])
	}
	if out.Steps[0].ScriptErr == nil {
		t.Fatalf("expected workflow step script error, got %+v", out.Steps[0])
	}
}

func TestRequestAssertParseErrorSourceSpanGated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := "# @assert status == 200 && ok\nGET " + srv.URL + "\n"
	doc := parser.Parse("basic.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	// Default (headless / `resterm run`): expression-relative column (base 1) and
	// no source attached, exactly as before the source-span change.
	res, err := New(engine.Config{}).ExecuteRequest(
		doc,
		doc.Requests[0],
		testSelection(""),
	)
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.ScriptErr == nil {
		t.Fatal("expected script error for malformed assert")
	}
	rep := diag.ReportOf(res.ScriptErr)
	if len(rep.Items) == 0 {
		t.Fatal("expected diagnostic items")
	}
	if len(rep.Source) != 0 {
		t.Fatalf("expected no source attached without SourceDiagnostics, got %q", rep.Source)
	}
	if col := rep.Items[0].Span.Start.Col; col != 15 {
		t.Fatalf("expected pre-gate expression-relative column 15, got %d", col)
	}

	// TUI (SourceDiagnostics): precise column at the '&' plus source for the caret.
	res, err = New(engine.Config{SourceDiagnostics: true}).ExecuteRequest(
		doc,
		doc.Requests[0],
		testSelection(""),
	)
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.ScriptErr == nil {
		t.Fatal("expected script error for malformed assert")
	}
	rep = diag.ReportOf(res.ScriptErr)
	if len(rep.Items) == 0 {
		t.Fatal("expected diagnostic items")
	}
	if !strings.Contains(string(rep.Source), "@assert") {
		t.Fatalf("expected file source attached for caret rendering, got %q", rep.Source)
	}
	if span := rep.Items[0].Span.Start; span.Line != 1 || span.Col != 25 {
		t.Fatalf("expected span at the '&' (1:25), got %d:%d", span.Line, span.Col)
	}
	if got := res.ScriptErr.Error(); !strings.Contains(got, ":1:25:") {
		t.Fatalf("expected position in error string, got %q", got)
	}
}

func TestRequestCaptureParseErrorSourceSpanGated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := "# @capture request id status && ok\nGET " + srv.URL + "\n"
	doc := parser.Parse("capture.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	// Default (headless / `resterm run`): expression-relative column (base 1) and
	// no source attached, exactly as before the source-span change.
	res, err := New(engine.Config{}).ExecuteRequest(
		doc,
		doc.Requests[0],
		testSelection(""),
	)
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected request error for malformed capture")
	}
	rep := diag.ReportOf(res.Err)
	if len(rep.Items) == 0 {
		t.Fatal("expected diagnostic items")
	}
	if len(rep.Source) != 0 {
		t.Fatalf("expected no source attached without SourceDiagnostics, got %q", rep.Source)
	}
	if col := rep.Items[0].Span.Start.Col; col != 8 {
		t.Fatalf("expected pre-gate expression-relative column 8, got %d", col)
	}

	// TUI (SourceDiagnostics): precise column at the '&' plus source for the caret.
	res, err = New(engine.Config{SourceDiagnostics: true}).ExecuteRequest(
		doc,
		doc.Requests[0],
		testSelection(""),
	)
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected request error for malformed capture")
	}
	rep = diag.ReportOf(res.Err)
	if len(rep.Items) == 0 {
		t.Fatal("expected diagnostic items")
	}
	if !strings.Contains(string(rep.Source), "@capture") {
		t.Fatalf("expected file source attached for caret rendering, got %q", rep.Source)
	}
	if span := rep.Items[0].Span.Start; span.Line != 1 || span.Col != 30 {
		t.Fatalf("expected span at the '&' (1:30), got %d:%d", span.Line, span.Col)
	}
}

func TestRequestRTSModuleParseErrorCarriesSourceSpan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := "# @rts pre-request\n> let ok = 1 && 2\nGET " + srv.URL + "\n"
	doc := parser.Parse("module.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	res, err := New(engine.Config{}).ExecuteRequest(
		doc,
		doc.Requests[0],
		testSelection(""),
	)
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected request error for malformed pre-request RTS block")
	}

	rep := diag.ReportOf(res.Err)
	if len(rep.Items) == 0 {
		t.Fatal("expected diagnostic items")
	}
	if rep.Items[0].Message != "unexpected '&'" {
		t.Fatalf("expected precise lexer message, got %q", rep.Items[0].Message)
	}
	if !strings.Contains(string(rep.Source), "let ok") {
		t.Fatalf("expected file source attached for caret rendering, got %q", rep.Source)
	}
	if span := rep.Items[0].Span.Start; span.Line != 2 {
		t.Fatalf("expected span on the script line (2), got line %d", span.Line)
	}
}

type streamSvc struct {
	testgrpc.UnimplementedTestServiceServer
}

func (s *streamSvc) StreamingOutputCall(
	_ *testgrpc.StreamingOutputCallRequest,
	stream testgrpc.TestService_StreamingOutputCallServer,
) error {
	if err := stream.Send(&testgrpc.StreamingOutputCallResponse{
		Payload: &testgrpc.Payload{Body: []byte("one")},
	}); err != nil {
		return err
	}
	return stream.Send(&testgrpc.StreamingOutputCallResponse{
		Payload: &testgrpc.Payload{Body: []byte("two")},
	})
}

func TestEngineExecuteRequestCapturesScriptedWebSocketTranscript(t *testing.T) {
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
				_ = conn.Close(websocket.StatusNormalClosure, "done")
			}()
			_, data, err := conn.Read(r.Context())
			if err != nil {
				t.Fatalf("websocket read failed: %v", err)
			}
			if err := conn.Write(
				r.Context(),
				websocket.MessageText,
				[]byte("pong:"+string(data)),
			); err != nil {
				t.Fatalf("websocket write failed: %v", err)
			}
		}),
	)
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	addr := "ws" + strings.TrimPrefix(srv.URL, "http")
	req := &restfile.Request{
		Method: "GET",
		URL:    addr,
		WebSocket: &restfile.WebSocketRequest{
			Steps: []restfile.WebSocketStep{{
				Type:  restfile.WebSocketStepSendText,
				Value: "ping",
			}},
		},
		Metadata: restfile.RequestMetadata{
			Asserts: []restfile.AssertSpec{{
				Expression: "stream.summary().receivedCount == 2",
			}},
			Captures: []restfile.CaptureSpec{{
				Scope:      directive.ScopeRequest,
				Name:       "ws.reply",
				Expression: "{{stream.events[1].text}}",
			}},
		},
	}
	doc := &restfile.Document{Path: "ws.http", Requests: []*restfile.Request{req}}

	eng := New(engine.Config{})
	res, err := eng.ExecuteRequest(doc, req, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("unexpected request error: %v", res.Err)
	}
	if res.Stream == nil || res.Stream.Kind != "websocket" {
		t.Fatalf("unexpected stream info: %+v", res.Stream)
	}
	if got := res.Stream.Summary["receivedCount"]; got != 2 {
		t.Fatalf("expected websocket reply plus close event, got %#v", got)
	}
	if len(res.Transcript) == 0 {
		t.Fatalf("expected websocket transcript")
	}
	if res.ScriptErr != nil {
		t.Fatalf("unexpected script error: %v", res.ScriptErr)
	}
	if len(res.Tests) != 1 || !res.Tests[0].Passed {
		t.Fatalf("unexpected tests: %+v", res.Tests)
	}
	got, ok := findReqVar(res.Executed, "ws.reply")
	if !ok || got.Value != "pong:ping" {
		t.Fatalf("unexpected captured ws reply: %+v %v", got, ok)
	}
}

func TestEngineExecuteRequestCapturesGRPCTranscript(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	testgrpc.RegisterTestServiceServer(srv, &streamSvc{})
	reflection.Register(srv)
	go func() {
		_ = srv.Serve(lis)
	}()
	defer func() {
		srv.Stop()
		_ = lis.Close()
	}()

	req := &restfile.Request{
		Method:   "GRPC",
		Settings: map[string]string{},
		GRPC: &restfile.GRPCRequest{
			Target:        lis.Addr().String(),
			Package:       "grpc.testing",
			Service:       "TestService",
			Method:        "StreamingOutputCall",
			FullMethod:    "/grpc.testing.TestService/StreamingOutputCall",
			UseReflection: true,
			Plaintext:     restfile.OptOf(true),
		},
		Metadata: restfile.RequestMetadata{
			Asserts: []restfile.AssertSpec{{
				Expression: "stream.summary().receivedCount == 2",
			}},
			Captures: []restfile.CaptureSpec{{
				Scope:      directive.ScopeRequest,
				Name:       "grpc.count",
				Expression: "{{stream.summary.receivedCount}}",
			}},
		},
	}
	doc := &restfile.Document{Path: "grpc.http", Requests: []*restfile.Request{req}}

	eng := New(engine.Config{
		GRPCOptions: grpcx.Options{
			DefaultPlaintext: restfile.OptOf(true),
			DialTimeout:      time.Second,
		},
	})
	res, err := eng.ExecuteRequest(doc, req, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("unexpected grpc error: %v", res.Err)
	}
	if res.GRPC == nil {
		t.Fatalf("expected grpc response")
	}
	if res.Stream == nil || res.Stream.Kind != "grpc" {
		t.Fatalf("unexpected grpc stream info: %+v", res.Stream)
	}
	if got := res.Stream.Summary["receivedCount"]; got != 2 {
		t.Fatalf("expected two received grpc messages, got %#v", got)
	}
	if len(res.Transcript) == 0 {
		t.Fatalf("expected grpc transcript")
	}
	if res.ScriptErr != nil {
		t.Fatalf("unexpected script error: %v", res.ScriptErr)
	}
	if len(res.Tests) != 1 || !res.Tests[0].Passed {
		t.Fatalf("unexpected tests: %+v", res.Tests)
	}
	got, ok := findReqVar(res.Executed, "grpc.count")
	if !ok || got.Value != "2" {
		t.Fatalf("unexpected captured grpc count: %+v %v", got, ok)
	}
}

func findReqVar(req *restfile.Request, name string) (restfile.Variable, bool) {
	if req == nil {
		return restfile.Variable{}, false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	for _, v := range req.Variables {
		if strings.ToLower(strings.TrimSpace(v.Name)) == key {
			return v, true
		}
	}
	return restfile.Variable{}, false
}

func testDocumentRequest(url string) (*restfile.Document, *restfile.Request) {
	req := &restfile.Request{
		Method: "GET",
		URL:    url,
		Metadata: restfile.RequestMetadata{
			Name: "ok",
		},
	}
	return &restfile.Document{
		Path:     "smoke.http",
		Requests: []*restfile.Request{req},
	}, req
}

func TestEngineExpandsDeclaredVariableTemplates(t *testing.T) {
	type observedRequest struct {
		body  string
		trace string
		err   error
	}
	observed := make(chan observedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			observed <- observedRequest{err: fmt.Errorf("read request body: %w", err)}
			return
		}
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			observed <- observedRequest{err: fmt.Errorf("write response: %w", err)}
			return
		}
		observed <- observedRequest{body: string(data), trace: r.Header.Get("X-Trace-Id")}
	}))
	defer srv.Close()

	src := strings.Join([]string{
		"# @name nested",
		"# @request trace.id {{$uuid}}",
		"POST " + srv.URL,
		"X-Trace-Id: {{trace.id}}",
		"Content-Type: application/json",
		"",
		`{"request_id": "{{trace.id}}", "inline": "{{$uuid}}"}`,
		"",
	}, "\n")
	doc := parser.Parse("nested.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(doc.Requests))
	}

	res, err := New(engine.Config{}).ExecuteRequest(doc, doc.Requests[0], testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("unexpected request error: %v", res.Err)
	}
	got := <-observed
	if got.err != nil {
		t.Fatal(got.err)
	}

	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !uuidRe.MatchString(got.trace) {
		t.Fatalf("expected header to carry expanded uuid, got %q", got.trace)
	}

	var payload struct {
		RequestID string `json:"request_id"`
		Inline    string `json:"inline"`
	}
	if err := json.Unmarshal([]byte(got.body), &payload); err != nil {
		t.Fatalf("parse sent body %q: %v", got.body, err)
	}
	if payload.RequestID != got.trace {
		t.Fatalf("expected body and header to share one value, got %q vs %q", payload.RequestID, got.trace)
	}
	if !uuidRe.MatchString(payload.Inline) || payload.Inline == got.trace {
		t.Fatalf("expected inline dynamic to stay fresh, got %q vs %q", payload.Inline, got.trace)
	}
}

func TestRequestAssertSpansCommentLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := "# @assert (\n#   status == 200\n#   and  statusText == \"200 OK\"\n# ) => \"healthy\"\nGET " + srv.URL + "\n"
	doc := parser.Parse("basic.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}

	res, err := New(engine.Config{}).ExecuteRequest(doc, doc.Requests[0], testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.ScriptErr != nil {
		t.Fatalf("script error: %v", res.ScriptErr)
	}
	if len(res.Tests) != 1 {
		t.Fatalf("tests = %+v, want 1", res.Tests)
	}
	tc := res.Tests[0]
	if !tc.Passed || tc.Message != "healthy" {
		t.Fatalf("test = %+v", tc)
	}
	if tc.Name != `( status == 200 and  statusText == "200 OK" )` {
		t.Fatalf("name = %q", tc.Name)
	}
}

func TestRequestSpanningConditionSkipReasonIsOneLine(t *testing.T) {
	src := "# @when (\n#   1 == 2\n#   and 3 == 3\n# )\nGET https://example.invalid/\n"
	doc := parser.Parse("basic.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}

	res, err := New(engine.Config{}).ExecuteRequest(doc, doc.Requests[0], testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if !res.Skipped {
		t.Fatalf("result = %+v, want a skip", res)
	}
	if res.SkipReason != "@when evaluated to false: ( 1 == 2 and 3 == 3 )" {
		t.Fatalf("reason = %q", res.SkipReason)
	}
}

func TestRequestAssertErrorPointsAtTheContinuationLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := "# @assert (\n#   status == 200\n#   and status status\n# )\nGET " + srv.URL + "\n"
	doc := parser.Parse("basic.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}

	res, err := New(engine.Config{SourceDiagnostics: true}).ExecuteRequest(
		doc,
		doc.Requests[0],
		testSelection(""),
	)
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.ScriptErr == nil {
		t.Fatal("expected a script error for the malformed assertion")
	}
	rep := diag.ReportOf(res.ScriptErr)
	if len(rep.Items) == 0 {
		t.Fatal("expected diagnostic items")
	}
	if span := rep.Items[0].Span.Start; span.Line != 3 || span.Col != 16 {
		t.Fatalf("span = %d:%d, want the second \"status\" at 3:16", span.Line, span.Col)
	}
}

func TestRequestCaptureExpressionTemplateReadsTheCurrentResponse(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprintf(w, `{"token":"tok-%d"}`, atomic.AddInt32(&n, 1)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := "### one\n# @capture request first response.json.token\nGET " + srv.URL + "/1\n\n" +
		"### two\n" +
		"# @capture request rts response.json.token\n" +
		"# @capture request plain {{response.json.token}}\n" +
		"# @capture request expr {{= response.json(\"token\") }}\n" +
		"# @capture request prev {{= last.json(\"token\") }}\n" +
		"GET " + srv.URL + "/2\n"
	doc := parser.Parse("chain.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}

	eng := New(engine.Config{})
	got := make(map[string]string)
	for _, req := range doc.Requests {
		res, err := eng.ExecuteRequest(doc, req, testSelection(""))
		if err != nil {
			t.Fatalf("ExecuteRequest: %v", err)
		}
		for _, v := range res.Executed.Variables {
			got[v.Name] = v.Value
		}
	}

	want := map[string]string{
		"first": "tok-1",
		"rts":   "tok-2",
		"plain": "tok-2",
		"expr":  "tok-2",
		"prev":  "tok-1",
	}
	for name, value := range want {
		if got[name] != value {
			t.Errorf("capture %q = %q, want %q", name, got[name], value)
		}
	}
}

func TestRequestPreRequestTemplateRejectsResponse(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprintf(w, `{"token":"tok-%d"}`, atomic.AddInt32(&n, 1)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	run := func(t *testing.T, tail string) engine.RequestResult {
		t.Helper()
		src := "### one\nGET " + srv.URL + "/1\n\n### two\n" + tail
		doc := parser.Parse("chain.http", []byte(src))
		if len(doc.Errors) != 0 {
			t.Fatalf("parse errors: %+v", doc.Errors)
		}
		eng := New(engine.Config{})
		var last engine.RequestResult
		for _, req := range doc.Requests {
			res, err := eng.ExecuteRequest(doc, req, testSelection(""))
			if err != nil {
				t.Fatalf("ExecuteRequest: %v", err)
			}
			last = res
		}
		return last
	}

	wantsLast := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected an error naming last, got none")
		}
		if !strings.Contains(err.Error(), "use last for the previous response") {
			t.Fatalf("error = %v, want it to name last", err)
		}
	}

	t.Run("header", func(t *testing.T) {
		wantsLast(t, run(t, "GET "+srv.URL+"/2\nX-T: {{= response.json(\"token\") }}\n").Err)
	})

	t.Run("header behind try", func(t *testing.T) {
		tail := "GET " + srv.URL + "/2\nX-T: {{= (try response.json(\"token\")).value ?? \"none\" }}\n"
		wantsLast(t, run(t, tail).Err)
	})

	t.Run("condition", func(t *testing.T) {
		wantsLast(t, run(t, "# @when response.statusCode == 200\nGET "+srv.URL+"/2\n").Err)
	})

	t.Run("bare condition", func(t *testing.T) {
		wantsLast(t, run(t, "# @when response\nGET "+srv.URL+"/2\n").Err)
	})

	t.Run("nested variable", func(t *testing.T) {
		tail := "# @request nested {{= response.statusCode }}\n" +
			"GET " + srv.URL + "/2\nX-T: {{nested}}\n"
		wantsLast(t, run(t, tail).Err)
	})

	t.Run("last still reads the previous response", func(t *testing.T) {
		var m int32
		seen := make(chan string, 2)
		rec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen <- r.Header.Get("X-T")
			if _, err := fmt.Fprintf(w, `{"token":"tok-%d"}`, atomic.AddInt32(&m, 1)); err != nil {
				t.Errorf("write response: %v", err)
			}
		}))
		defer rec.Close()

		src := "### one\nGET " + rec.URL + "/1\n\n### two\nGET " + rec.URL + "/2\n" +
			"X-T: {{= last.json(\"token\") }}\n"
		doc := parser.Parse("chain.http", []byte(src))
		if len(doc.Errors) != 0 {
			t.Fatalf("parse errors: %+v", doc.Errors)
		}
		eng := New(engine.Config{})
		for _, req := range doc.Requests {
			res, err := eng.ExecuteRequest(doc, req, testSelection(""))
			if err != nil {
				t.Fatalf("ExecuteRequest: %v", err)
			}
			if res.Err != nil {
				t.Fatalf("unexpected error: %v", res.Err)
			}
		}
		close(seen)

		var sent []string
		for h := range seen {
			sent = append(sent, h)
		}
		if len(sent) != 2 || sent[1] != "tok-1" {
			t.Fatalf("headers = %q, want the second request to carry the first response", sent)
		}
	})
}

func TestRequestCaptureExpressionsUseTheConfiguredBaseDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	// Put the file in both directories so a wrong base returns the wrong value.
	docDir, baseDir := t.TempDir(), t.TempDir()
	for dir, where := range map[string]string{docDir: "doc-dir", baseDir: "base-dir"} {
		body := fmt.Sprintf(`{"where":%q}`, where)
		if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", dir, err)
		}
	}

	docPath := filepath.Join(docDir, "capture.http")
	src := "# @capture request tpl {{= (try json.file(\"data.json\")).value.where ?? \"none\" }}\n" +
		"# @capture request rts (try json.file(\"data.json\")).value.where ?? \"none\"\n" +
		"GET " + srv.URL + "/\n"
	if err := os.WriteFile(docPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write document: %v", err)
	}

	doc := parser.Parse(docPath, []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}

	res, err := New(engine.Config{
		FilePath:    docPath,
		HTTPOptions: httpx.Options{BaseDir: baseDir},
	}).ExecuteRequest(doc, doc.Requests[0], testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}

	got := make(map[string]string, 2)
	for _, v := range res.Executed.Variables {
		got[v.Name] = v.Value
	}
	for _, name := range []string{"tpl", "rts"} {
		if got[name] != "base-dir" {
			t.Errorf("capture %q = %q, want the file under the configured base directory", name, got[name])
		}
	}
}
