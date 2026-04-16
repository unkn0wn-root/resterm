package headless

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
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

	res, err := eng.ExecuteRequest(doc, req, "")
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
	}, "")
	if err != nil {
		t.Fatalf("ExecuteCompare: %v", err)
	}
	if compare == nil || len(compare.Rows) != 2 {
		t.Fatalf("unexpected compare result: %+v", compare)
	}

	docProfile, profileReq := testDocumentRequest(srv.URL)
	profileReq.Metadata.Profile = &restfile.ProfileSpec{Count: 2}
	profile, err := eng.ExecuteProfile(docProfile, profileReq, "")
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
	out, err := eng.ExecuteWorkflow(doc, wf, "")
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

	if _, err := eng.ExecuteRequest(doc, setDev, "dev"); err != nil {
		t.Fatalf("set dev cookie: %v", err)
	}
	res, err := eng.ExecuteRequest(doc, echo, "dev")
	if err != nil {
		t.Fatalf("echo dev cookie: %v", err)
	}
	if got := strings.TrimSpace(string(res.Response.Body)); got != "session=dev123" {
		t.Fatalf("expected dev cookie, got %q", got)
	}

	res, err = eng.ExecuteRequest(doc, echo, "prod")
	if err != nil {
		t.Fatalf("echo prod cookie before set: %v", err)
	}
	if got := strings.TrimSpace(string(res.Response.Body)); got != "" {
		t.Fatalf("expected no prod cookie before set, got %q", got)
	}

	if _, err := eng.ExecuteRequest(doc, setProd, "prod"); err != nil {
		t.Fatalf("set prod cookie: %v", err)
	}
	res, err = eng.ExecuteRequest(doc, echo, "prod")
	if err != nil {
		t.Fatalf("echo prod cookie: %v", err)
	}
	if got := strings.TrimSpace(string(res.Response.Body)); got != "session=prod456" {
		t.Fatalf("expected prod cookie, got %q", got)
	}

	res, err = eng.ExecuteRequest(doc, echo, "dev")
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

	reqRes, err := eng.ExecuteRequest(doc, req, "")
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

	out, err := eng.ExecuteWorkflow(doc, wf, "")
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
				Scope:      restfile.CaptureScopeRequest,
				Name:       "ws.reply",
				Expression: "{{stream.events[1].text}}",
			}},
		},
	}
	doc := &restfile.Document{Path: "ws.http", Requests: []*restfile.Request{req}}

	eng := New(engine.Config{})
	res, err := eng.ExecuteRequest(doc, req, "")
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
			Plaintext:     true,
			PlaintextSet:  true,
		},
		Metadata: restfile.RequestMetadata{
			Asserts: []restfile.AssertSpec{{
				Expression: "stream.summary().receivedCount == 2",
			}},
			Captures: []restfile.CaptureSpec{{
				Scope:      restfile.CaptureScopeRequest,
				Name:       "grpc.count",
				Expression: "{{stream.summary.receivedCount}}",
			}},
		},
	}
	doc := &restfile.Document{Path: "grpc.http", Requests: []*restfile.Request{req}}

	eng := New(engine.Config{
		GRPCOptions: grpcclient.Options{
			DefaultPlaintext:    true,
			DefaultPlaintextSet: true,
			DialTimeout:         time.Second,
		},
	})
	res, err := eng.ExecuteRequest(doc, req, "")
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
