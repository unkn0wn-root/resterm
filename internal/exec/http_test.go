package exec

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newHTTPClientWithFactory(factory httpx.HTTPClientFactory) *httpx.Client {
	return httpx.NewClientWithOptions(httpx.WithHTTPFactory(factory))
}

func TestRunnerRunHTTPSSE(t *testing.T) {
	client := newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
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

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/events",
		SSE:    &restfile.SSERequest{},
	}

	var capIn CaptureInput
	var testIn TestInput
	testErr := errors.New("boom")
	run := Runner{
		Hooks: HTTPHooks{
			ApplyCaptures: func(in CaptureInput) error {
				capIn = in
				return nil
			},
			RunTests: func(in TestInput) ([]scriptapi.TestResult, error) {
				testIn = in
				return []scriptapi.TestResult{{Name: "hook", Passed: true}}, testErr
			},
		},
	}

	res := run.RunHTTP(HTTPInput{
		Client:           client,
		Context:          context.Background(),
		Req:              req,
		EffectiveTimeout: 5 * time.Second,
	})
	if res.Err != nil {
		t.Fatalf("RunHTTP error: %v", res.Err)
	}
	if res.Response == nil {
		t.Fatalf("expected response")
	}
	if res.Stream == nil || res.Stream.Kind != "sse" || res.Stream.Summary["eventCount"] != 1 {
		t.Fatalf("expected sse stream info with one event, got %+v", res.Stream)
	}
	if capIn.Stream != res.Stream || testIn.Stream != res.Stream {
		t.Fatalf("hooks saw streams %p and %p, want %p", capIn.Stream, testIn.Stream, res.Stream)
	}
	if testIn.Response == nil || testIn.Response != capIn.Response {
		t.Fatalf("tests hook response = %p, want the capture response %p", testIn.Response, capIn.Response)
	}
	if testIn.HTTP != res.Response {
		t.Fatalf("tests hook response = %p, want %p", testIn.HTTP, res.Response)
	}
	if len(res.Tests) != 1 || res.Tests[0].Name != "hook" || !errors.Is(res.ScriptErr, testErr) {
		t.Fatalf("tests = %+v, script error = %v", res.Tests, res.ScriptErr)
	}
}

func TestConvertSSETranscriptExposesCompletionMetadata(t *testing.T) {
	info := convertSSETranscript(&httpx.SSETranscript{
		Summary: httpx.SSESummary{
			Reason:  "error",
			Dropped: 4,
			Error:   "read sse stream: boom",
		},
	})
	if got := info.Summary["dropped"]; got != int64(4) {
		t.Fatalf("summary.dropped = %#v, want 4", got)
	}
	if got := info.Summary["error"]; got != "read sse stream: boom" {
		t.Fatalf("summary.error = %#v, want the stream error", got)
	}
}

func TestConvertWebSocketTranscriptExposesDroppedEvents(t *testing.T) {
	info := convertWebSocketTranscript(&httpx.WebSocketTranscript{
		Summary: httpx.WebSocketSummary{Dropped: 3},
	})
	if got := info.Summary["dropped"]; got != int64(3) {
		t.Fatalf("summary.dropped = %#v, want 3", got)
	}
}

func TestRunnerRunHTTPRejectsInteractiveWebSocket(t *testing.T) {
	run := Runner{}
	res := run.RunHTTP(HTTPInput{
		Client:  httpx.NewClient(nil),
		Context: context.Background(),
		Req: &restfile.Request{
			URL:       "wss://example.com",
			WebSocket: &restfile.WebSocketRequest{},
		},
		EffectiveTimeout: 1,
	})
	if res.Err == nil {
		t.Fatalf("expected error for interactive websocket")
	}
	if !strings.Contains(res.Err.Error(), "caller-managed session handling") {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.Decision != "WebSocket request failed" {
		t.Fatalf("unexpected decision %q", res.Decision)
	}
}
