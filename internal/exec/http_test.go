package exec

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRunnerRunHTTPSSE(t *testing.T) {
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
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
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{{
				Kind: "test",
				Body: `{% tests.assert(response.json().summary.eventCount === 1, "event count"); %}`,
			}},
		},
	}

	seenStream := (*struct {
		kind  string
		count int
	})(nil)
	run := Runner{
		Hooks: HTTPHooks{
			ApplyCaptures: func(in CaptureInput) error {
				seenStream = &struct {
					kind  string
					count int
				}{
					kind:  in.Stream.Kind,
					count: in.Stream.Summary["eventCount"].(int),
				}
				return nil
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
	if res.Stream == nil || res.Stream.Kind != "sse" {
		t.Fatalf("expected sse stream info, got %+v", res.Stream)
	}
	if len(res.Tests) != 1 {
		t.Fatalf("expected 1 test result, got %d", len(res.Tests))
	}
	if !res.Tests[0].Passed {
		t.Fatalf("expected test to pass, got %+v", res.Tests[0])
	}
	if seenStream == nil {
		t.Fatalf("expected capture hook to observe stream info")
	}
	if seenStream.kind != "sse" || seenStream.count != 1 {
		t.Fatalf("unexpected stream info %+v", seenStream)
	}
}

func TestRunnerRunHTTPRejectsInteractiveWebSocket(t *testing.T) {
	run := Runner{}
	res := run.RunHTTP(HTTPInput{
		Client:           httpclient.NewClient(nil),
		Context:          context.Background(),
		Req:              &restfile.Request{URL: "wss://example.com", WebSocket: &restfile.WebSocketRequest{}},
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
