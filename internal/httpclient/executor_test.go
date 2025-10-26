package httpclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestApplyRequestSettings(t *testing.T) {
	opts := Options{Timeout: 5 * time.Second, FollowRedirects: true}
	settings := map[string]string{
		"timeout":         "10s",
		"proxy":           "http://localhost:8080",
		"followredirects": "false",
		"insecure":        "true",
	}

	effective := applyRequestSettings(opts, settings)
	if effective.Timeout != 10*time.Second {
		t.Fatalf("expected timeout 10s, got %s", effective.Timeout)
	}
	if !effective.InsecureSkipVerify {
		t.Fatalf("expected insecure skip verify to be true")
	}
	if effective.FollowRedirects {
		t.Fatalf("expected redirects disabled")
	}
	if effective.ProxyURL != "http://localhost:8080" {
		t.Fatalf("unexpected proxy url: %s", effective.ProxyURL)
	}
}

func TestInjectBodyIncludes(t *testing.T) {
	client := &Client{fs: OSFileSystem{}}
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "payload.json")
	if err := os.WriteFile(path, []byte(`{"status":"ok"}`), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	body := "part1\n@payload.json\n@{notIncluded}\n"
	processed, err := client.injectBodyIncludes(body, baseDir)
	if err != nil {
		t.Fatalf("inject body includes: %v", err)
	}
	if !strings.Contains(processed, `{"status":"ok"}`) {
		t.Fatalf("expected file contents to be embedded, got %q", processed)
	}
	if !strings.Contains(processed, "@{notIncluded}") {
		t.Fatalf("expected handlebars directive to remain untouched")
	}
}

func TestApplyAuthenticationBasic(t *testing.T) {
	client := NewClient(nil)
	req := restfile.Request{Method: "GET", URL: "https://example.com"}
	httpReq, err := http.NewRequestWithContext(context.Background(), req.Method, req.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	auth := &restfile.AuthSpec{Type: "basic", Params: map[string]string{"username": "alice", "password": "secret"}}
	resolver := vars.NewResolver()
	client.applyAuthentication(httpReq, resolver, auth)
	if got := httpReq.Header.Get("Authorization"); !strings.HasPrefix(got, "Basic ") {
		t.Fatalf("expected basic auth header, got %s", got)
	}
}

func TestPrepareGraphQLPostBody(t *testing.T) {
	client := NewClient(nil)
	req := &restfile.Request{Method: "POST", URL: "https://example.com/graphql"}
	req.Body.GraphQL = &restfile.GraphQLBody{
		Query:     "query Ping($id: ID!){ ping(id: $id) }",
		Variables: "{ \"id\": \"{{id}}\" }",
	}
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"id": "123"}))
	reader, err := client.prepareBody(req, resolver, Options{})
	if err != nil {
		t.Fatalf("prepare graphQL body: %v", err)
	}
	if reader == nil {
		t.Fatalf("expected reader for POST graphQL body")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["query"] != "query Ping($id: ID!){ ping(id: $id) }" {
		t.Fatalf("unexpected query: %v", payload["query"])
	}
	varsField, ok := payload["variables"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected variables object, got %T", payload["variables"])
	}
	if varsField["id"] != "123" {
		t.Fatalf("expected variables to expand templates, got %v", varsField["id"])
	}
}

func TestPrepareGraphQLGetQueryParameters(t *testing.T) {
	client := NewClient(nil)
	req := &restfile.Request{Method: "GET", URL: "https://example.com/graphql?existing=1"}
	req.Body.GraphQL = &restfile.GraphQLBody{
		Query:         "query { ping }",
		Variables:     "{ \"flag\": true }",
		OperationName: "Ping",
	}
	reader, err := client.prepareBody(req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("prepare graphQL body (GET): %v", err)
	}
	if reader != nil {
		t.Fatalf("expected nil reader for GET graphql request")
	}
	parsed, err := url.Parse(req.URL)
	if err != nil {
		t.Fatalf("parse mutated url: %v", err)
	}
	values := parsed.Query()
	if values.Get("existing") != "1" {
		t.Fatalf("expected existing query param to persist, got %q", values.Get("existing"))
	}
	if values.Get("operationName") != "Ping" {
		t.Fatalf("expected operationName query param, got %q", values.Get("operationName"))
	}
	if values.Get("query") == "" {
		t.Fatalf("expected query parameter to be set")
	}
	if values.Get("variables") == "" {
		t.Fatalf("expected variables parameter to be set")
	}
}

func TestPrepareBodyFileExpandTemplates(t *testing.T) {
	fs := mapFS{
		"payload.json": []byte(`{"id":"{{env.id}}"}`),
	}
	client := NewClient(fs)
	req := &restfile.Request{Method: "POST", URL: "https://example.com"}
	req.Body.FilePath = "payload.json"
	req.Body.Options.ExpandTemplates = true
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"id": "123"}))
	reader, err := client.prepareBody(req, resolver, Options{})
	if err != nil {
		t.Fatalf("prepare body: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(data) != `{"id":"123"}` {
		t.Fatalf("unexpected expanded body: %s", string(data))
	}
}

func TestExecuteCapturesTraceTimeline(t *testing.T) {
	client := NewClient(nil)
	client.httpFactory = func(Options) (*http.Client, error) {
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			trace := httptrace.ContextClientTrace(req.Context())
			if trace != nil {
				if trace.DNSStart != nil {
					trace.DNSStart(httptrace.DNSStartInfo{Host: "example.com"})
				}
				time.Sleep(200 * time.Microsecond)
				if trace.DNSDone != nil {
					trace.DNSDone(httptrace.DNSDoneInfo{Addrs: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}})
				}
				time.Sleep(200 * time.Microsecond)
				if trace.ConnectStart != nil {
					trace.ConnectStart("tcp", "93.184.216.34:443")
				}
				time.Sleep(200 * time.Microsecond)
				if trace.ConnectDone != nil {
					trace.ConnectDone("tcp", "93.184.216.34:443", nil)
				}
				time.Sleep(200 * time.Microsecond)
				if trace.TLSHandshakeStart != nil {
					trace.TLSHandshakeStart()
				}
				time.Sleep(200 * time.Microsecond)
				if trace.TLSHandshakeDone != nil {
					trace.TLSHandshakeDone(tls.ConnectionState{}, nil)
				}
				time.Sleep(200 * time.Microsecond)
				if trace.WroteHeaders != nil {
					trace.WroteHeaders()
				}
				time.Sleep(200 * time.Microsecond)
				if trace.WroteRequest != nil {
					trace.WroteRequest(httptrace.WroteRequestInfo{})
				}
				time.Sleep(300 * time.Microsecond)
				if trace.GotFirstResponseByte != nil {
					trace.GotFirstResponseByte()
				}
			}

			body := &slowBody{data: []byte("ok"), delay: 300 * time.Microsecond}
			resp := &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Header:     make(http.Header),
				Body:       body,
				Request:    req,
			}
			return resp, nil
		})
		return &http.Client{Transport: transport}, nil
	}

	req := &restfile.Request{Method: http.MethodGet, URL: "https://example.com"}
	budget := nettrace.Budget{Total: 10 * time.Microsecond}
	resp, err := client.Execute(context.Background(), req, vars.NewResolver(), Options{Trace: true, TraceBudget: &budget})
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	if resp.Timeline == nil {
		t.Fatalf("expected timeline to be recorded")
	}
	if resp.TraceReport == nil {
		t.Fatalf("expected trace report to be populated")
	}
	if resp.TraceReport.Budget.Total != budget.Total {
		t.Fatalf("expected report to retain budget total, got %v", resp.TraceReport.Budget.Total)
	}
	if len(resp.TraceReport.BudgetReport.Breaches) == 0 {
		t.Fatalf("expected budget breaches to be recorded")
	}

	durations := make(map[nettrace.PhaseKind]time.Duration)
	for _, phase := range resp.Timeline.Phases {
		durations[phase.Kind] += phase.Duration
	}

	for _, kind := range []nettrace.PhaseKind{
		nettrace.PhaseDNS,
		nettrace.PhaseConnect,
		nettrace.PhaseTTFB,
		nettrace.PhaseTransfer,
	} {
		if durations[kind] <= 0 {
			t.Fatalf("expected duration for phase %s to be > 0, got %v", kind, durations[kind])
		}
	}
	if resp.Timeline.Duration <= 0 {
		t.Fatalf("expected overall timeline duration > 0, got %v", resp.Timeline.Duration)
	}
	if resp.Duration < resp.Timeline.Duration {
		t.Fatalf("expected response duration %v to be >= timeline duration %v", resp.Duration, resp.Timeline.Duration)
	}
	if diff := resp.Duration - resp.Timeline.Duration; diff > time.Millisecond || diff < -time.Millisecond {
		t.Fatalf("expected response duration and timeline duration to match within 1ms, got diff %v", diff)
	}
}

type mapFS map[string][]byte

func (m mapFS) ReadFile(name string) ([]byte, error) {
	if data, ok := m[name]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

type slowBody struct {
	data   []byte
	delay  time.Duration
	offset int
}

func (b *slowBody) Read(p []byte) (int, error) {
	if b.offset >= len(b.data) {
		time.Sleep(b.delay)
		return 0, io.EOF
	}
	time.Sleep(b.delay)
	n := copy(p, b.data[b.offset:])
	b.offset += n
	return n, nil
}

func (b *slowBody) Close() error { return nil }

func TestExecuteSSE(t *testing.T) {
	client := NewClient(nil)
	client.httpFactory = func(Options) (*http.Client, error) {
		stream := strings.Join([]string{
			":warmup",
			"",
			"id: 1",
			"event: greet",
			"data: hello world",
			"",
		}, "\n") + "\n"
		return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			resp := &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(stream)),
			}
			resp.Header.Set("Content-Type", "text/event-stream")
			finalReq := req.Clone(req.Context())
			parsed, err := url.Parse("https://final.example.com/events")
			if err != nil {
				return nil, err
			}
			finalReq.URL = parsed
			resp.Request = finalReq
			return resp, nil
		})}, nil
	}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/events",
		SSE:    &restfile.SSERequest{},
	}
	resp, err := client.ExecuteSSE(context.Background(), req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("execute sse: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.EffectiveURL != "https://final.example.com/events" {
		t.Fatalf("expected effective url to track redirect, got %s", resp.EffectiveURL)
	}

	var transcript struct {
		Events []struct {
			Event string `json:"event"`
			Data  string `json:"data"`
		}
		Summary struct {
			EventCount int `json:"eventCount"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(resp.Body, &transcript); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	if transcript.Summary.EventCount == 0 {
		t.Fatalf("expected at least one event, got %d", transcript.Summary.EventCount)
	}
	if transcript.Events[transcript.Summary.EventCount-1].Event != "greet" {
		t.Fatalf("expected final event to be greet, got %s", transcript.Events[len(transcript.Events)-1].Event)
	}
}

func TestExecuteSSEIdleTimeout(t *testing.T) {
	client := NewClient(nil)
	client.httpFactory = func(Options) (*http.Client, error) {
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			reader, writer := io.Pipe()
			go func() {
				defer func() {
					if err := writer.Close(); err != nil {
						t.Logf("close writer: %v", err)
					}
				}()
				_, _ = io.WriteString(writer, "data: ping\n\n")
				<-req.Context().Done()
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
	}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/events",
		SSE: &restfile.SSERequest{Options: restfile.SSEOptions{
			IdleTimeout:  25 * time.Millisecond,
			TotalTimeout: 500 * time.Millisecond,
		}},
	}

	resp, err := client.ExecuteSSE(context.Background(), req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("execute sse idle: %v", err)
	}
	if resp == nil {
		t.Fatalf("expected response")
	}

	var transcript struct {
		Summary struct {
			Reason string `json:"reason"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(resp.Body, &transcript); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	if transcript.Summary.Reason != "timeout:idle" {
		t.Fatalf("expected idle timeout reason, got %q", transcript.Summary.Reason)
	}
}

func TestExecuteSSEMaxBytes(t *testing.T) {
	client := NewClient(nil)
	first := "data: one\n\n"
	second := "data: two\n\n"
	maxBytes := len(first)
	client.httpFactory = func(Options) (*http.Client, error) {
		stream := first + second
		return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			resp := &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(stream)),
				Request:    req,
			}
			resp.Header.Set("Content-Type", "text/event-stream")
			return resp, nil
		})}, nil
	}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/events",
		SSE: &restfile.SSERequest{Options: restfile.SSEOptions{
			MaxBytes: int64(maxBytes),
		}},
	}

	resp, err := client.ExecuteSSE(context.Background(), req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("execute sse max bytes: %v", err)
	}

	var transcript SSETranscript
	if err := json.Unmarshal(resp.Body, &transcript); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	if transcript.Summary.Reason != "limit:max_bytes" {
		t.Fatalf("expected max bytes limit reason, got %q", transcript.Summary.Reason)
	}
	if transcript.Summary.EventCount != 1 {
		t.Fatalf("expected exactly one event before byte limit, got %d", transcript.Summary.EventCount)
	}
}

func TestStartSSEPublishesEvents(t *testing.T) {
	client := NewClient(nil)
	client.httpFactory = func(Options) (*http.Client, error) {
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			reader, writer := io.Pipe()
			go func() {
				defer func() {
					if err := writer.Close(); err != nil {
						t.Logf("close writer: %v", err)
					}
				}()
				_, _ = io.WriteString(writer, "data: first\n\n")
				time.Sleep(10 * time.Millisecond)
				_, _ = io.WriteString(writer, "data: second\n\n")
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
	}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/events",
		SSE:    &restfile.SSERequest{},
	}
	handle, fallback, err := client.StartSSE(context.Background(), req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("start sse: %v", err)
	}
	if fallback != nil {
		t.Fatalf("expected streaming session, got fallback response")
	}

	session := handle.Session
	listener := session.Subscribe()
	received := make([]string, 0, 2)
	for _, evt := range listener.Snapshot.Events {
		if evt.Direction == stream.DirReceive {
			received = append(received, string(evt.Payload))
		}
	}
	done := make(chan struct{})
	go func() {
		for evt := range listener.C {
			if evt.Direction != stream.DirReceive {
				continue
			}
			received = append(received, string(evt.Payload))
		}
		close(done)
	}()

	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("session did not complete")
	}

	listener.Cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("listener did not drain")
	}

	if len(received) < 2 {
		t.Fatalf("expected at least two events, got %d", len(received))
	}
	if received[0] != "first" || received[1] != "second" {
		t.Fatalf("unexpected events: %v", received)
	}
	state, serr := session.State()
	if serr != nil {
		t.Fatalf("unexpected session error: %v", serr)
	}
	if state != stream.StateClosed {
		t.Fatalf("expected session to close cleanly, got state %v", state)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
