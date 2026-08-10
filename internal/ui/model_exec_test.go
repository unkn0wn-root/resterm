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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/authcmd"
	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/engine"
	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
	"google.golang.org/grpc/codes"
	"nhooyr.io/websocket"
)

// runCmd flattens startRun's pair for tests that only need the command.
func runCmd(cmd tea.Cmd, _ bool) tea.Cmd { return cmd }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newHTTPClientWithFactory(factory httpx.HTTPClientFactory) *httpx.Client {
	return httpx.NewClientWithOptions(httpx.WithHTTPFactory(factory))
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
	resp := &grpcx.Response{
		StatusCode:    codes.NotFound,
		StatusMessage: "not found",
		Message:       "{}",
	}
	err := diag.New(diag.ClassProtocol, "invoke grpc method")

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
	resp := &grpcx.Response{
		StatusCode:      codes.OK,
		StatusMessage:   "OK",
		Message:         `{"ok":true}`,
		Body:            []byte(`{"ok":true}`),
		Wire:            wire,
		ContentType:     "application/json",
		WireContentType: "application/grpc+proto",
	}

	cmd := model.consumeGRPCResponse(responseMsg{grpc: resp, executed: req})
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

	err := diag.New(
		diag.ClassProtocol,
		"send request failed",
		diag.WithComponent(diag.ComponentHTTP),
	)
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
	if model.statusMessage.text != "Request failed ✗" ||
		model.statusMessage.level != statusError {
		t.Fatalf("unexpected request error status: %+v", model.statusMessage)
	}
}

func TestHandleResponseMsgShowsRequestCauseWithoutNotes(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	err := diag.Wrap(
		&url.Error{
			Op:  "Get",
			URL: "https://api.local",
			Err: &net.DNSError{Err: "no such host", Name: "api.local"},
		},
		"perform request",
		diag.WithComponent(diag.ComponentHTTP),
	)
	cmd := model.handleResponseMessage(responseMsg{err: err})
	if cmd != nil {
		collectMsgs(cmd)
	}

	if model.responseLatest == nil || !model.responseLatest.ready {
		t.Fatalf("expected latest snapshot to be ready")
	}
	plain := stripANSIEscape(model.responseLatest.pretty)
	for _, want := range []string{
		"error[network]: request failed",
		"perform request",
		"╰─> Get \"https://api.local\"",
		"    ╰─> lookup api.local: no such host",
		"help: No response payload was received.",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected request error view to contain %q, got %q", want, plain)
		}
	}
	if strings.Contains(plain, "note:") {
		t.Fatalf("expected request error view not to use notes, got %q", plain)
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

	err := diag.WrapAs(diag.ClassScript, errors.New("boom"), "pre-request script")
	cmd := model.handleResponseMessage(responseMsg{err: err})
	if cmd != nil {
		collectMsgs(cmd)
	}

	if model.showErrorModal {
		t.Fatalf("expected error modal to stay closed for script errors")
	}
	if model.statusMessage.text != "Request failed ✗" ||
		model.statusMessage.level != statusError {
		t.Fatalf("unexpected script error status: %+v", model.statusMessage)
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
}

func TestSendActiveRequestHardFailsOnParseError(t *testing.T) {
	var calls int32
	fakeClient := newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Fatalf("request should not be sent after parse error")
			return nil, nil
		})}, nil
	})

	content := strings.Join([]string{
		"# @rts pre",
		`> request.setHeader("X-Test", "1")`,
		"GET https://example.com",
		"",
	}, "\n")
	model := New(Config{
		FilePath:       "sample.http",
		InitialContent: content,
		Client:         fakeClient,
	})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	if cmd := model.sendActiveRequest(); cmd != nil {
		collectMsgs(cmd)
	}

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected no HTTP client creation, got %d", got)
	}
	if model.statusMessage.text != "Request failed ✗" ||
		model.statusMessage.level != statusError {
		t.Fatalf("unexpected parse error status: %+v", model.statusMessage)
	}
	if model.lastError == nil || diag.ClassOf(model.lastError) != diag.ClassParse {
		t.Fatalf("expected parse lastError, got %v", model.lastError)
	}
	if model.responseLatest == nil || !model.responseLatest.ready {
		t.Fatalf("expected ready parse-error response snapshot")
	}
	plain := stripANSIEscape(model.responseLatest.pretty)
	for _, want := range []string{
		"error[parse]: @rts supports only pre-request mode",
		"@rts supports only pre-request mode",
		"--> sample.http:1:1",
		"   1 | # @rts pre",
		"     | ^",
		"note: Fix the request file parse error before running.",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected parse view to contain %q, got %q", want, plain)
		}
	}
}

func TestStartConfigCompareHardFailsOnParseError(t *testing.T) {
	content := strings.Join([]string{
		"# @rts pre",
		`> request.setHeader("X-Test", "1")`,
		"GET https://example.com",
		"",
	}, "\n")
	model := New(Config{
		FilePath:       "sample.http",
		InitialContent: content,
		Compare:        engine.CompareConfig{Targets: []string{"dev", "prod"}},
	})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	if cmd := model.startConfigCompareFromEditor(); cmd != nil {
		collectMsgs(cmd)
	}

	if model.compareRun != nil {
		t.Fatalf("expected compare run not to start")
	}
	if model.responseLatest == nil || !strings.Contains(
		stripANSIEscape(model.responseLatest.pretty),
		"@rts supports only pre-request mode",
	) {
		t.Fatalf("expected parse error in response pane, got %#v", model.responseLatest)
	}
}

func TestHandleResponseMsgRendersRTSStack(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 100
	model.height = 30
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	err := diag.WrapAs(diag.ClassScript, &rts.StackError{
		Err: &rts.RuntimeError{
			Pos: rts.Pos{Path: "hook.rts", Line: 3, Col: 7},
			Msg: "boom",
		},
		Frames: []rts.Frame{{
			Kind: rts.FrameFn,
			Pos:  rts.Pos{Path: "hook.rts", Line: 2, Col: 1},
			Name: "sign",
		}},
	},
		"pre-request rts script",
	)

	cmd := model.handleResponseMessage(responseMsg{err: err})
	if cmd != nil {
		collectMsgs(cmd)
	}

	if model.responseLatest == nil || !model.responseLatest.ready {
		t.Fatalf("expected ready script-error response snapshot")
	}
	plain := stripANSIEscape(model.responseLatest.pretty)
	for _, want := range []string{
		"error[script]: boom",
		"--> hook.rts:3:7",
		"pre-request rts script",
		"Stack:",
		"at hook.rts:2:1 in sign",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected RTS stack view to contain %q, got %q", want, plain)
		}
	}
}

func TestStyleLinesUsesSourceStyleForGutterBar(t *testing.T) {
	model := New(Config{})
	st := model.errSty()
	line := diag.Line{Kind: diag.LineBar, Text: "     |"}
	if got, want := st.line(line), st.src.Render(line.Text); got != want {
		t.Fatalf("LineBar style mismatch\ngot:  %q\nwant: %q", got, want)
	}

	mark := "     |   ^"
	if got, want := st.mark(mark), st.src.Render("     |   ")+st.title.Render("^"); got != want {
		t.Fatalf("LineMark style mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExecuteRequestRunsScriptsForSSE(t *testing.T) {
	fakeClient := newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
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
				Scope:      directive.ScopeRequest,
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

	cmd, _ := model.startRun(runSpec{doc: doc, req: req, opts: model.cfg.HTTPOptions, sel: testSelection("")})
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
	var seenHeader string
	fakeClient := newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
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

	model := New(Config{Env: vars.Config{Selection: testSelection("dev")}, Client: fakeClient})
	model.globalsStore().Set(testEnv("dev").Scope(), "token", "global-token", false)

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

	msg, ok := runCmd(model.startRun(runSpec{doc: nil, req: req, opts: httpx.Options{}, sel: testSelection("")}))().(responseMsg)
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
	model := New(Config{Env: vars.Config{Selection: testSelection("dev")}})
	model.globalsStore().Set(testEnv("dev").Scope(), "token", "global-token", false)

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

	msg, ok := runCmd(model.startRun(runSpec{doc: nil, req: req, opts: httpx.Options{}, sel: testSelection(""), mode: rqeng.ExecModePreview}))().(responseMsg)
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

func TestBuildHTTPRequestUsesInheritedFileAuth(t *testing.T) {
	model := Model{
		ws: workspace{sel: testSelection("dev")},
	}
	doc := &restfile.Document{
		Path: "/tmp/inherited-auth.http",
		Auth: []restfile.AuthProfile{{
			Scope: directive.ScopeFile,
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

	model.syncRegistry(doc)
	model.requestSvc(httpx.Options{}).ResolveInheritedAuth(doc, req)
	client := httpx.NewClient(nil)
	httpReq, _, _, err := client.BuildHTTPRequest(
		context.Background(),
		req,
		vars.NewResolver(),
		httpx.Options{},
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
		ws: workspace{sel: testSelection("dev")},
	}
	model.registryIndex().Sync(&restfile.Document{
		Path: "/tmp/other.http",
		Auth: []restfile.AuthProfile{{
			Scope: directive.ScopeGlobal,
			Spec: restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "global-token"},
			},
			Line: 1,
		}},
	})

	req := &restfile.Request{}
	model.requestSvc(httpx.Options{}).ResolveInheritedAuth(
		&restfile.Document{Path: "/tmp/current.http"},
		req,
	)

	if req.Metadata.Auth == nil {
		t.Fatalf("expected inherited global auth")
	}
	if got := req.Metadata.Auth.Params["token"]; got != "global-token" {
		t.Fatalf("expected global token, got %q", got)
	}
}

func TestRunPreRequestScriptsApplyCanClearInheritedAuth(t *testing.T) {
	model := Model{
		ws: workspace{sel: testSelection("dev")},
	}
	doc := &restfile.Document{
		Path: "/tmp/inherited-auth-apply.http",
		Auth: []restfile.AuthProfile{{
			Scope: directive.ScopeFile,
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

	res, err := model.requestSvc(httpx.Options{}).ExecuteWith(
		doc,
		req, testEnv(

			""),

		rqeng.ExecOptions{Mode: rqeng.ExecModePreview})

	if err != nil {
		t.Fatalf("ExecuteWith: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("preview error: %v", res.Err)
	}
	if res.Executed.Metadata.Auth != nil {
		t.Fatalf("expected @apply to clear inherited auth")
	}
	if !res.Executed.Metadata.AuthDisabled {
		t.Fatalf("expected cleared inherited auth to stay disabled for this execution")
	}
}

func TestEnsureOAuthSetsAuthorizationHeader(t *testing.T) {
	var calls int32
	var lastAuth string
	var lastForm url.Values

	model := Model{
		ws: workspace{sel: testSelection("dev")},
	}

	model.runtimeSvc().OAuth().SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpx.Options) (*httpx.Response, error) {
			atomic.AddInt32(&calls, 1)
			values, err := url.ParseQuery(req.Body.Text)
			if err != nil {
				t.Fatalf("parse form: %v", err)
			}
			lastForm = copyValues(values)
			lastAuth = req.Headers.Get("Authorization")
			return &httpx.Response{
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
	if err := model.requestSvc(httpx.Options{}).EnsureOAuth(
		context.Background(),
		req,
		resolver,
		httpx.Options{},
		testEnv(""),
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
	if err := model.requestSvc(httpx.Options{}).EnsureOAuth(
		context.Background(),
		req2,
		resolver,
		httpx.Options{},
		testEnv(""),
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
		ws: workspace{sel: testSelection("dev")},
	}
	model.runtimeSvc().OAuth().SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpx.Options) (*httpx.Response, error) {
			atomic.AddInt32(&called, 1)
			return &httpx.Response{
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
	if err := model.requestSvc(httpx.Options{}).EnsureOAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		httpx.Options{},
		testEnv(""),
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

func testEnsureCommandAuth(
	m *Model,
	ctx context.Context,
	req *restfile.Request,
	res *vars.Resolver,
	env string,
	timeout time.Duration,
) (authcmd.Result, error) {
	return m.requestSvc(httpx.Options{}).EnsureCommandAuth(
		ctx,
		nil,
		req,
		res,
		testEnv(env),
		timeout,
	)
}

func testBuildCommandAuthConfig(
	m *Model,
	auth *restfile.AuthSpec,
	res *vars.Resolver,
	timeout time.Duration,
) (authcmd.Config, error) {
	return m.requestSvc(httpx.Options{}).
		BuildCommandAuthConfig(nil, auth, res, timeout)
}

func testPrepareExplainAuthPreview(
	m *Model,
	req *restfile.Request,
	res *vars.Resolver,
	env string,
) (rqeng.ExplainAuthPreviewResult, error) {
	return m.requestSvc(httpx.Options{}).
		PrepareExplainAuthPreview(nil, req, res, testEnv(env))
}

func TestEnsureOAuthUsesEnvironmentOverride(t *testing.T) {
	var requests int32
	model := Model{
		ws: workspace{sel: testSelection("dev")},
	}
	model.runtimeSvc().OAuth().SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpx.Options) (*httpx.Response, error) {
			atomic.AddInt32(&requests, 1)
			return &httpx.Response{
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

	if err := model.requestSvc(httpx.Options{}).EnsureOAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		httpx.Options{},
		testEnv("stage"),
		time.Second,
	); err != nil {
		t.Fatalf("ensureOAuth stage: %v", err)
	}
	req.Headers = nil
	if err := model.requestSvc(httpx.Options{}).EnsureOAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		httpx.Options{},
		testEnv("stage"),
		time.Second,
	); err != nil {
		t.Fatalf("ensureOAuth stage cached: %v", err)
	}
	if atomic.LoadInt32(&requests) != 1 {
		t.Fatalf("expected cached token for repeated stage env, got %d", requests)
	}

	req.Headers = nil
	if err := model.requestSvc(httpx.Options{}).EnsureOAuth(
		context.Background(),
		req,
		vars.NewResolver(),
		httpx.Options{},
		testEnv("dev"),
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
		ws: workspace{sel: testSelection("dev")},
	}

	model.runtimeSvc().OAuth().SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpx.Options) (*httpx.Response, error) {
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
		if err := model.requestSvc(httpx.Options{}).EnsureOAuth(
			ctx,
			req,
			resolver,
			httpx.Options{},
			testEnv(""),
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
		ws:          workspace{sel: testSelection("dev")},
		currentFile: "/tmp/example.http",
	}
	model.runtimeSvc().AuthCmd().SetExecFunc(func(_ context.Context, cfg authcmd.Config) ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		seen = cfg
		return []byte("token-basic"), nil
	})

	auth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"argv":      `["gh","auth","token"]`,
		"cache_key": "github",
	}}
	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: auth}}

	res, err := testEnsureCommandAuth(&model,
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
	if _, err := testEnsureCommandAuth(&model,
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
		ws: workspace{sel: testSelection("dev")},
	}
	model.runtimeSvc().AuthCmd().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
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

	if _, err := testEnsureCommandAuth(&model,
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
		ws:          workspace{sel: testSelection("dev")},
		currentFile: "/tmp/example.http",
	}
	model.runtimeSvc().AuthCmd().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
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
	if _, err := testEnsureCommandAuth(&model,
		context.Background(),
		seedReq,
		vars.NewResolver(),
		"",
		time.Second,
	); err != nil {
		t.Fatalf("ensureCommandAuth seed: %v", err)
	}

	req := &restfile.Request{Metadata: restfile.RequestMetadata{Auth: cacheOnlyAuth}}
	res, err := testEnsureCommandAuth(&model,
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
		ws:          workspace{sel: testSelection("dev")},
		currentFile: "/tmp/example.http",
	}

	auth := &restfile.AuthSpec{Type: "command", Params: map[string]string{
		"argv": `["aws","--profile","{{aws.profile}}","ecr","get-login-password"]`,
	}}
	resolver := vars.NewResolver(vars.NewMapProvider("aws", map[string]string{
		"profile": `qa"blue\team`,
	}))

	cfg, err := testBuildCommandAuthConfig(&model, auth, resolver, 5*time.Second)
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
		ws:          workspace{sel: testSelection("dev")},
		currentFile: "/tmp/example.http",
	}
	model.runtimeSvc().AuthCmd().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
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
	if _, err := testEnsureCommandAuth(&model,
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
	preview, err := testPrepareExplainAuthPreview(&model, req, vars.NewResolver(), "")
	if err != nil {
		t.Fatalf("prepareExplainAuthPreview: %v", err)
	}
	if preview.Status != xplain.StageOK {
		t.Fatalf("expected preview ok, got %v", preview.Status)
	}
	if preview.Summary != explainSummaryAuthPrepared {
		t.Fatalf("expected auth prepared summary, got %q", preview.Summary)
	}
	if got := req.Headers.Get("X-Registry-Token"); got != "Token token-basic" {
		t.Fatalf("expected cached auth header, got %q", got)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected preview to reuse cache only, got %d executions", calls)
	}
	if len(preview.ExtraSecrets) == 0 {
		t.Fatalf("expected preview secrets to include cached auth values")
	}
}

func TestPrepareExplainAuthPreviewCommandCacheOnlyRequiresSeed(t *testing.T) {
	model := Model{
		ws: workspace{sel: testSelection("dev")},
	}

	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{Type: "command", Params: map[string]string{
				"cache_key": "github",
			}},
		},
	}

	_, err := testPrepareExplainAuthPreview(&model, req, vars.NewResolver(), "")
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
		ws: workspace{sel: testSelection("dev")},
	}
	model.runtimeSvc().AuthCmd().SetExecFunc(func(_ context.Context, _ authcmd.Config) ([]byte, error) {
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

	preview, err := testPrepareExplainAuthPreview(&model, req, vars.NewResolver(), "")
	if err != nil {
		t.Fatalf("prepareExplainAuthPreview: %v", err)
	}
	if preview.Status != xplain.StageSkipped {
		t.Fatalf("expected preview skip, got %v", preview.Status)
	}
	if preview.Summary != explainSummaryCommandAuthExecutionSkipped {
		t.Fatalf("expected command skip summary, got %q", preview.Summary)
	}
	if req.Headers.Get("Authorization") != "" {
		t.Fatalf("expected no auth header injection without cache")
	}
	if atomic.LoadInt32(&called) != 0 {
		t.Fatalf("expected preview to skip command execution, got %d calls", called)
	}
}

func TestExecuteRequestCancelsBeforePreRequest(t *testing.T) {
	model := Model{
		ws: workspace{sel: testSelection("dev")},
	}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
	}

	cmd, _ := model.startRun(runSpec{doc: nil, req: req, opts: httpx.Options{}, sel: testSelection("")})
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

	cmd, _ := model.startRun(runSpec{doc: nil, req: req, opts: httpx.Options{}, sel: testSelection("")})
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
	if got := resp.response.Headers.Get(httpx.StreamHeaderType); got != "websocket" {
		t.Fatalf("expected websocket placeholder header, got %q", got)
	}
	applyQueuedStreamAttach(t, &model)

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

func TestRunRequestServiceAttachesInteractiveWebSocket(t *testing.T) {
	srv, cleanup := startUIWebSocketServer(t)
	defer cleanup()

	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/ws/chat"
	model := New(Config{})
	req := &restfile.Request{
		Method:    "GET",
		URL:       wsURL,
		WebSocket: &restfile.WebSocketRequest{},
	}
	svc := model.runRequestSvc(httpx.Options{})
	if svc == nil {
		t.Fatal("expected run request service")
	}

	res, err := svc.ExecuteWith(
		nil,
		req, testEnv(

			""),

		rqeng.ExecOptions{Ctx: context.Background()})

	if err != nil {
		t.Fatalf("ExecuteWith: %v", err)
	}
	if res.Response == nil {
		t.Fatal("expected placeholder websocket response")
	}
	if got := res.Response.Headers.Get(httpx.StreamHeaderType); got != "websocket" {
		t.Fatalf("expected websocket placeholder header, got %q", got)
	}
	if res.StreamID == "" {
		t.Fatal("expected websocket result to carry its stream ID")
	}
	applyQueuedStreamAttach(t, &model)

	sessionID := model.sessionIDForRequest(req)
	session := model.sessionHandles[sessionID]
	if session == nil {
		t.Fatalf("expected websocket session handle for %q", sessionID)
	}
	select {
	case <-session.Done():
		t.Fatal("core websocket session canceled immediately after request returned")
	case <-time.After(150 * time.Millisecond):
	}
	session.Cancel()
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket session shutdown")
	}
}

func applyQueuedStreamAttach(t *testing.T, model *Model) {
	t.Helper()
	select {
	case msg := <-model.streamMsgChan:
		attach, ok := msg.(streamAttachMsg)
		if !ok {
			t.Fatalf("expected queued stream attachment, got %T", msg)
		}
		model.handleStreamAttach(attach)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream attachment")
	}
}

func TestExecuteRequestRejectsNilRequest(t *testing.T) {
	model := Model{
		ws: workspace{sel: testSelection("dev")},
	}

	cmd, _ := model.startRun(runSpec{doc: nil, req: nil, opts: httpx.Options{}, sel: testSelection("")})
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
		ws: workspace{sel: testSelection("dev")},
	}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		SSH:    &restfile.SSHSpec{},
		K8s:    &restfile.K8sSpec{},
	}

	cmd, _ := model.startRun(runSpec{doc: nil, req: req, opts: httpx.Options{}, sel: testSelection("")})
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
	model.sendingOverlayBase = responseExplainPreviewBase
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
	if model.sendingOverlayBase != "" {
		t.Fatalf("expected sending overlay label to reset, got %q", model.sendingOverlayBase)
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

func TestShowGlobalSummary(t *testing.T) {
	model := Model{
		ws: workspace{sel: testSelection("dev"), active: testEnv("dev")},
		doc: &restfile.Document{
			Globals: []restfile.Variable{
				{Name: "docVar", Value: "visible"},
				{Name: "secretDoc", Value: "hidden", Secret: true},
			},
		},
	}
	model.globalsStore().Set(testEnv("dev").Scope(), "token", "secretValue", true)
	model.globalsStore().Set(testEnv("dev").Scope(), "refresh", "refresh-token", false)

	model.showGlobalSummary()

	expected := "Globals: refresh=refresh-token, token=••• | Doc: docVar=visible, secretDoc=•••"
	if model.statusMessage.text != expected {
		t.Fatalf("expected summary %q, got %q", expected, model.statusMessage.text)
	}
	if model.statusMessage.level != statusInfo {
		t.Fatalf("expected info status, got %v", model.statusMessage.level)
	}
}

func TestClearGlobalValues(t *testing.T) {
	model := Model{
		ws: workspace{sel: testSelection("dev"), active: testEnv("dev")},
	}
	model.globalsStore().Set(testEnv("dev").Scope(), "token", "value", false)
	model.cookieStore().Jar(testEnv("dev").Scope())
	if snap := model.globalsStore().Snapshot(testEnv("dev").Scope()); len(snap) == 0 {
		t.Fatalf("expected snapshot to contain entries before clearing")
	}
	model.clearGlobalValues()
	if snap := model.globalsStore().Snapshot(testEnv("dev").Scope()); len(snap) != 0 {
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
		ws: workspace{sel: testSelection("dev"), active: testEnv("dev")},
	}
	u, err := url.Parse("https://example.com")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	model.cookieStore().Jar(testEnv("dev").Scope()).SetCookies(u, []*http.Cookie{{Name: "session", Value: "dev123"}})
	model.cookieStore().
		Jar("prod").
		SetCookies(u, []*http.Cookie{{Name: "session", Value: "prod456"}})

	model.clearGlobalValues()

	if got := model.cookieStore().Jar(testEnv("dev").Scope()).Cookies(u); len(got) != 0 {
		t.Fatalf("expected dev cookies to be cleared, got %+v", got)
	}
	if got := model.cookieStore().
		Jar("prod").
		Cookies(u); len(got) != 1 ||
		got[0].Value != "prod456" {
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

	model := New(Config{Env: vars.Config{Selection: testSelection("dev")}})

	setReq := &restfile.Request{Method: http.MethodGet, URL: srv.URL + "/set"}
	echoReq := &restfile.Request{Method: http.MethodGet, URL: srv.URL + "/echo"}

	msg, ok := runCmd(model.startRun(runSpec{doc: nil, req: setReq, opts: httpx.Options{}, sel: testSelection("dev")}))().(responseMsg)
	if !ok || msg.err != nil {
		t.Fatalf("unexpected set response: %#v", msg)
	}

	msg, ok = runCmd(model.startRun(runSpec{doc: nil, req: echoReq, opts: httpx.Options{}, sel: testSelection("dev")}))().(responseMsg)
	if !ok || msg.err != nil {
		t.Fatalf("unexpected dev echo response: %#v", msg)
	}
	if got := strings.TrimSpace(string(msg.response.Body)); got != "session=dev123" {
		t.Fatalf("expected dev cookie, got %q", got)
	}

	msg, ok = runCmd(model.startRun(runSpec{doc: nil, req: echoReq, opts: httpx.Options{}, sel: testSelection("prod")}))().(responseMsg)
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

	model := New(Config{Env: vars.Config{Selection: testSelection("dev")}})
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	model.cookieStore().Jar(testEnv("dev").Scope()).SetCookies(u, []*http.Cookie{{Name: "session", Value: "dev123"}})

	req := &restfile.Request{
		Method:   http.MethodGet,
		URL:      srv.URL + "/echo",
		Settings: map[string]string{"no-cookies": "true"},
	}
	msg, ok := runCmd(model.startRun(runSpec{doc: nil, req: req, opts: httpx.Options{}, sel: testSelection("dev")}))().(responseMsg)
	if !ok || msg.err != nil {
		t.Fatalf("unexpected response: %#v", msg)
	}
	if got := strings.TrimSpace(string(msg.response.Body)); got != "" {
		t.Fatalf("expected no cookie to be sent, got %q", got)
	}
}

func TestExecuteRequestWithTraceSpecPopulatesTimeline(t *testing.T) {
	client := newHTTPClientWithFactory(func(opts httpx.Options) (*http.Client, error) {
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
	model := New(Config{Client: client})

	content := "### Trace\n# @trace total<=1s\nGET https://example.com\n\n"
	doc := parser.Parse("trace.http", []byte(content))
	if len(doc.Requests) != 1 {
		t.Fatalf("expected single request")
	}
	req := doc.Requests[0]
	cmd, _ := model.startRun(runSpec{doc: doc, req: req, opts: model.cfg.HTTPOptions, sel: testSelection("")})
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
		ws: workspace{sel: testSelection("dev")},
	}

	// Prepare the cookie jar
	u, _ := url.Parse(srv.URL)
	model.cookieStore().Jar(testEnv("dev").Scope()).SetCookies(u, []*http.Cookie{
		{Name: "session", Value: "active"},
	})

	// First request to check the cookie is set by default
	req := &restfile.Request{
		Method: "GET",
		URL:    srv.URL,
	}

	cmd, _ := model.startRun(
		runSpec{doc: nil, req: req, opts: httpx.Options{NoFallback: true}, sel: testSelection("dev")},
	)
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
	if respBodyString != "session=active" {
		t.Fatalf("expected cookie session=active in dev env, got %q", respBodyString)
	}

	// Second request with setting to skip cookies
	reqWithSetting := &restfile.Request{
		Method:   "GET",
		URL:      srv.URL,
		Settings: map[string]string{"no-cookies": "true"},
	}

	cmd, _ = model.startRun(runSpec{
		req:  reqWithSetting,
		opts: httpx.Options{NoFallback: true},
		sel:  testSelection("dev"),
	})

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
