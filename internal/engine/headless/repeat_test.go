package headless

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	xexec "github.com/unkn0wn-root/resterm/internal/exec"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
)

type repeatRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn repeatRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestExecuteRequestRetryInsidePollUsesRTSPredicatesAndTerminalCapture(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		jobResponses := []struct {
			status int
			body   string
		}{
			{status: http.StatusServiceUnavailable, body: `{"status":"busy"}`},
			{status: http.StatusOK, body: `{"status":"pending"}`},
			{status: http.StatusServiceUnavailable, body: `{"status":"busy"}`},
			{status: http.StatusOK, body: `{"status":"completed"}`},
		}
		jobAttempts := 0
		client := newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
			return &http.Client{Transport: repeatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				status := http.StatusOK
				body := `{"status":"previous"}`
				if req.URL.Path == "/jobs/123" {
					item := jobResponses[jobAttempts]
					jobAttempts++
					status, body = item.status, item.body
				}
				return &http.Response{
					Status:        http.StatusText(status),
					StatusCode:    status,
					Proto:         "HTTP/1.1",
					Header:        make(http.Header),
					Body:          io.NopCloser(strings.NewReader(body)),
					ContentLength: int64(len(body)),
					Request:       req,
				}, nil
			})}, nil
		})
		source := `### Seed
# @name seed
GET https://example.com/seed

### Wait for job
# @name wait
# @retry count=1
# @retry-when response.statusCode == 503
# @retry-backoff exponential(1ms, 1ms) jitter=0%
# @poll every=1ms timeout=1s until=last.json().status == "previous" && response.json().status == "completed"
# @capture request job.status response.json().status
GET https://example.com/jobs/123
`
		doc := parser.Parse("jobs.http", []byte(source))
		if err := parser.Check(doc); err != nil {
			t.Fatalf("parse request: %v", err)
		}
		history := &memHistory{}
		eng := New(engine.Config{
			Client:      client,
			HTTPOptions: httpx.Options{Timeout: time.Second},
			History:     history,
		})
		t.Cleanup(func() { _ = eng.Close() })
		if seed, err := eng.ExecuteRequestContext(
			t.Context(),
			doc,
			doc.Requests[0],
			testSelection(""),
		); err != nil ||
			seed.Err != nil {
			t.Fatalf("seed request = err %v, result error %v", err, seed.Err)
		}
		result, err := eng.ExecuteRequestContext(t.Context(), doc, doc.Requests[1], testSelection(""))
		if err != nil {
			t.Fatalf("ExecuteRequestContext() error = %v", err)
		}
		if result.Err != nil {
			t.Fatalf("request result error = %v", result.Err)
		}
		if jobAttempts != 4 {
			t.Fatalf("job attempts = %d, want 4", jobAttempts)
		}
		if result.Response == nil || !strings.Contains(string(result.Response.Body), "completed") {
			t.Fatalf("terminal response = %+v", result.Response)
		}
		captured, ok := findReqVar(result.Executed, "job.status")
		if !ok || captured.Value != "completed" {
			t.Fatalf("terminal capture = %+v, %v", captured, ok)
		}
		if len(history.entries) != 2 {
			t.Fatalf("history entries = %d, want seed and one logical job entry", len(history.entries))
		}
		if got := history.entries[1].Duration; got != 3*time.Millisecond {
			t.Fatalf("logical history duration = %s, want 3ms", got)
		}
	})
}

func TestExecuteRequestRejectsNonBooleanPollPredicate(t *testing.T) {
	client := newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
		return &http.Client{Transport: repeatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"status":"pending"}`)),
				Request:    req,
			}, nil
		})}, nil
	})
	doc := parser.Parse(
		"jobs.http",
		[]byte("# @poll timeout=1s until=response.json().status\nGET https://example.com/jobs/123\n"),
	)
	if err := parser.Check(doc); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	result, err := New(engine.Config{Client: client, SourceDiagnostics: true}).ExecuteRequest(
		doc,
		doc.Requests[0],
		testSelection(""),
	)
	if err != nil {
		t.Fatalf("ExecuteRequest() error = %v", err)
	}
	if result.Err == nil || !strings.Contains(result.Err.Error(), "must evaluate to a boolean") {
		t.Fatalf("request result error = %v", result.Err)
	}
}

func TestPollTimeoutDoesNotReplaceLastResponse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		probeCalls := 0
		client := newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
			return &http.Client{Transport: repeatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				body := `{"status":"pending"}`
				switch req.URL.Path {
				case "/seed":
					body = `{"status":"previous"}`
				case "/probe":
					probeCalls++
					body = `{"status":"probe"}`
				}
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			})}, nil
		})
		doc := parser.Parse("last.http", []byte(`### Seed
GET https://example.com/seed

### Poll
# @poll every=1ms timeout=2ms until=response.json().status == "completed"
GET https://example.com/poll

### Probe
# @when last.json().status == "previous"
GET https://example.com/probe
`))
		if err := parser.Check(doc); err != nil {
			t.Fatalf("parse request: %v", err)
		}
		history := &memHistory{}
		eng := New(engine.Config{
			Client:      client,
			HTTPOptions: httpx.Options{Timeout: time.Second},
			History:     history,
		})
		t.Cleanup(func() { _ = eng.Close() })
		seed, err := eng.ExecuteRequestContext(t.Context(), doc, doc.Requests[0], testSelection(""))
		if err != nil || seed.Err != nil {
			t.Fatalf("seed request = err %v, result error %v", err, seed.Err)
		}
		poll, err := eng.ExecuteRequestContext(t.Context(), doc, doc.Requests[1], testSelection(""))
		if err != nil {
			t.Fatalf("poll request error = %v", err)
		}
		if !errors.Is(poll.Err, xexec.ErrPollTimeout) {
			t.Fatalf("poll result error = %v", poll.Err)
		}
		if poll.Response == nil || !strings.Contains(string(poll.Response.Body), "pending") {
			t.Fatalf("poll timeout response = %+v", poll.Response)
		}
		if len(history.entries) != 2 {
			t.Fatalf("history entries after timeout = %d, want seed and one poll entry", len(history.entries))
		}
		probe, err := eng.ExecuteRequestContext(t.Context(), doc, doc.Requests[2], testSelection(""))
		if err != nil || probe.Err != nil {
			t.Fatalf("probe request = err %v, result error %v", err, probe.Err)
		}
		if probe.Skipped || probeCalls != 1 {
			t.Fatalf("probe skipped=%v calls=%d; poll timeout replaced last", probe.Skipped, probeCalls)
		}
	})
}
