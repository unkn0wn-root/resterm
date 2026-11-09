package httpx

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/http/coding"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	if _, err := z.Write(data); err != nil || z.Close() != nil {
		t.Fatalf("gzip: %v", err)
	}
	return b.Bytes()
}

func serveGzip(t *testing.T, status int, contentType string, body []byte) *httptest.Server {
	t.Helper()
	gz := gzipBytes(t, body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(status)
		_, _ = w.Write(gz)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func serveBytes(t *testing.T, contentType string, size int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		chunk := make([]byte, 1<<16)
		for sent := 0; sent < size; sent += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func tooLarge(t *testing.T, err error) int64 {
	t.Helper()
	var limit *ResponseTooLargeError
	if !errors.As(err, &limit) {
		t.Fatalf("error = %v, want a ResponseTooLargeError", err)
	}
	return limit.Limit
}

func TestResponseBodyStopsAtTheLimit(t *testing.T) {
	srv := serveBytes(t, "application/octet-stream", 4<<20)

	_, err := NewClient(nil).Execute(
		t.Context(),
		&restfile.Request{Method: "GET", URL: srv.URL},
		nil,
		Options{MaxResponseBytes: bytesize.Of(1 << 20), Timeout: 30 * time.Second},
	)
	if got := tooLarge(t, err); got != 1<<20 {
		t.Fatalf("Limit = %d, want %d", got, 1<<20)
	}
}

func TestResponseBodyAtTheLimitIsKept(t *testing.T) {
	const size = 1 << 16
	srv := serveBytes(t, "application/octet-stream", size)

	resp, err := NewClient(nil).Execute(
		t.Context(),
		&restfile.Request{Method: "GET", URL: srv.URL},
		nil,
		Options{MaxResponseBytes: bytesize.Of(size), Timeout: 30 * time.Second},
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(resp.Body) != size {
		t.Fatalf("len(Body) = %d, want %d", len(resp.Body), size)
	}
}

func TestResponseBodyAcceptsTheLargestLimit(t *testing.T) {
	const body = "hello world"

	got, err := readResponseBody(strings.NewReader(body), math.MaxInt64)
	if err != nil {
		t.Fatalf("readResponseBody: %v", err)
	}
	if string(got) != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

func TestResponseLimitCountsDecompressedBytes(t *testing.T) {
	const limit = 1 << 20
	noise := make([]byte, limit)
	_, _ = rand.NewChaCha8([32]byte{}).Read(noise)
	if n := len(gzipBytes(t, noise)); n <= limit {
		t.Fatalf("gzip of the noise is %d bytes, want more than the %d byte limit", n, limit)
	}

	zeros := make([]byte, 8<<20)
	tests := []struct {
		name     string
		ask      bool
		body     []byte
		tooLarge bool
	}{
		{name: "transport asked", body: zeros, tooLarge: true},
		{name: "request asked", ask: true, body: zeros, tooLarge: true},
		{name: "near limit", ask: true, body: noise},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Content-Type", "text/plain")
				zw := gzip.NewWriter(w)
				_, _ = zw.Write(tt.body)
				_ = zw.Close()
			}))
			defer srv.Close()

			req := &restfile.Request{Method: "GET", URL: srv.URL}
			if tt.ask {
				req.Headers = http.Header{"Accept-Encoding": {"gzip"}}
			}
			resp, err := NewClient(nil).Execute(
				t.Context(),
				req,
				nil,
				Options{MaxResponseBytes: bytesize.Of(limit), Timeout: 30 * time.Second},
			)
			if tt.tooLarge {
				if got := tooLarge(t, err); got != limit {
					t.Fatalf("Limit = %d, want %d", got, limit)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !bytes.Equal(resp.Body, tt.body) {
				t.Fatalf("len(Body) = %d, want the %d decoded bytes", len(resp.Body), len(tt.body))
			}
		})
	}
}

func TestResponseBodyDecodesWithoutALimit(t *testing.T) {
	srv := serveGzip(t, http.StatusOK, "text/plain", []byte("hello"))
	tests := []struct {
		name  string
		limit bytesize.Budget
	}{
		{name: "unlimited", limit: bytesize.Unlimited()},
		{name: "largest limit", limit: bytesize.Of(math.MaxInt64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := NewClient(nil).Execute(
				t.Context(),
				&restfile.Request{Method: "GET", URL: srv.URL, Headers: http.Header{"Accept-Encoding": {"gzip"}}},
				nil,
				Options{MaxResponseBytes: tt.limit, Timeout: 30 * time.Second},
			)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if string(resp.Body) != "hello" {
				t.Fatalf("Body = %q, want the decoded body", resp.Body)
			}
		})
	}
}

func TestResponseBodyDecodesGzipTheRequestAskedFor(t *testing.T) {
	gz := gzipBytes(t, []byte("hello"))
	tests := []struct {
		name     string
		method   string
		status   int
		enc      []string
		body     []byte
		flush    bool
		noHeader bool
		want     string
		decoded  bool
		err      error
	}{
		{name: "gzip", enc: []string{"gzip"}, body: gz, want: "hello", decoded: true},
		{name: "gzip in capitals", enc: []string{"GZIP"}, body: gz, want: "hello", decoded: true},
		{
			name:    "two members",
			enc:     []string{"gzip"},
			body:    slices.Concat(gzipBytes(t, []byte("hel")), gzipBytes(t, []byte("lo"))),
			want:    "hello",
			decoded: true,
		},
		{name: "chunked empty", enc: []string{"gzip"}, flush: true, decoded: true},
		{name: "transport asked", enc: []string{"gzip"}, body: gz, noHeader: true, want: "hello", decoded: true},
		{name: "invalid", enc: []string{"gzip"}, body: []byte("hello"), err: coding.ErrHeader},
		{name: "truncated", enc: []string{"gzip"}, body: gz[:len(gz)-4], err: io.ErrUnexpectedEOF},
		{
			name: "trailing garbage",
			enc:  []string{"gzip"},
			body: slices.Concat(gz, []byte("not a gzip member")),
			err:  gzip.ErrHeader,
		},
		{name: "identity", enc: []string{"identity"}, body: []byte("hello"), want: "hello"},
		{name: "none", enc: []string{"none"}, body: []byte("hello"), want: "hello"},
		{name: "br", enc: []string{"br"}, body: gz, want: string(gz)},
		{name: "deflate", enc: []string{"deflate"}, body: gz, want: string(gz)},
		{name: "zstd", enc: []string{"zstd"}, body: gz, want: string(gz)},
		{name: "x-gzip", enc: []string{"x-gzip"}, body: gz, want: string(gz)},
		{name: "gzip twice in one line", enc: []string{"gzip, gzip"}, body: gz, want: string(gz)},
		{name: "gzip on two lines", enc: []string{"gzip", "gzip"}, body: gz, want: string(gz)},
		{name: "identity then gzip", enc: []string{"identity", "gzip"}, body: gz, want: string(gz)},
		{name: "partial content", status: http.StatusPartialContent, enc: []string{"gzip"}, body: gz, want: string(gz)},
		{name: "head", method: http.MethodHead, enc: []string{"gzip"}},
		{name: "no content", status: http.StatusNoContent, enc: []string{"gzip"}},
		{name: "not modified", status: http.StatusNotModified, enc: []string{"gzip"}},
		{name: "zero length", enc: []string{"gzip"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header()["Content-Encoding"] = tt.enc
				w.WriteHeader(cmp.Or(tt.status, http.StatusOK))
				if tt.flush {
					w.(http.Flusher).Flush()
				}
				_, _ = w.Write(tt.body)
			}))
			defer srv.Close()

			req := &restfile.Request{Method: cmp.Or(tt.method, http.MethodGet), URL: srv.URL}
			if !tt.noHeader {
				req.Headers = http.Header{"Accept-Encoding": {"gzip"}}
			}
			resp, err := NewClient(nil).Execute(t.Context(), req, nil, Options{Timeout: 30 * time.Second})
			if tt.err != nil {
				if !errors.Is(err, tt.err) || diag.ClassOf(err) != diag.ClassProtocol {
					t.Fatalf("Execute() error = %v, want a protocol error matching %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if string(resp.Body) != tt.want {
				t.Fatalf("Body = %q, want %q", resp.Body, tt.want)
			}
			ce, cl := resp.Headers.Values("Content-Encoding"), resp.Headers.Get("Content-Length")
			switch {
			case tt.decoded && (ce != nil || cl != ""):
				t.Fatalf("headers = %v, want no Content-Encoding or Content-Length once decoded", resp.Headers)
			case !tt.decoded && !slices.Equal(ce, tt.enc):
				t.Fatalf("Content-Encoding = %q, want %q as sent", ce, tt.enc)
			case !tt.decoded && len(tt.body) > 0 && cl != strconv.Itoa(len(tt.body)):
				t.Fatalf("Content-Length = %q, want %d as sent", cl, len(tt.body))
			}
		})
	}
}

func TestResponseLimitDefaultsAndOverrides(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want int64
	}{
		{name: "unset takes the default", want: DefaultMaxResponseBytes},
		{name: "explicit size", opts: Options{MaxResponseBytes: bytesize.Of(1 << 20)}, want: 1 << 20},
		{
			name: "an unlimited budget removes the bound",
			opts: Options{MaxResponseBytes: bytesize.Unlimited()},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.responseLimit(); got != tt.want {
				t.Fatalf("responseLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMaxResponseSizeSetting(t *testing.T) {
	var opts Options
	if err := ApplyOptionSettings(&opts, map[string]string{"max-response-size": "2mb"}); err != nil {
		t.Fatalf("ApplyOptionSettings: %v", err)
	}
	if got := opts.responseLimit(); got != 2<<20 {
		t.Fatalf("responseLimit() = %d, want %d", got, 2<<20)
	}
	if err := ApplyOptionSettings(&opts, map[string]string{"max-response-size": "none"}); err != nil {
		t.Fatalf("ApplyOptionSettings: %v", err)
	}
	if got := opts.responseLimit(); got != 0 {
		t.Fatalf("responseLimit() = %d, want no limit", got)
	}
	if err := ApplyOptionSettings(&opts, map[string]string{"max-response-size": "soon"}); err == nil {
		t.Fatal("ApplyOptionSettings() = nil, want an invalid size error")
	}
}

func TestSSEFallbackBodyStopsAtTheLimit(t *testing.T) {
	srv := serveBytes(t, "application/json", 4<<20)

	req := &restfile.Request{
		Method: "GET",
		URL:    srv.URL,
		SSE:    &restfile.SSERequest{Options: restfile.SSEOptions{TotalTimeout: 30 * time.Second}},
	}
	_, err := NewClient(nil).ExecuteSSE(t.Context(), req, nil, Options{MaxResponseBytes: bytesize.Of(1 << 20)})
	tooLarge(t, err)
}

func TestSSEFallbackDecodesGzip(t *testing.T) {
	srv := serveGzip(t, http.StatusOK, "application/json", []byte(`{"ok":true}`))
	req := &restfile.Request{
		Method:  "GET",
		URL:     srv.URL,
		Headers: http.Header{"Accept-Encoding": {"gzip"}},
		SSE:     &restfile.SSERequest{Options: restfile.SSEOptions{TotalTimeout: 30 * time.Second}},
	}
	resp, err := NewClient(nil).ExecuteSSE(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("ExecuteSSE: %v", err)
	}
	if string(resp.Body) != `{"ok":true}` || resp.Headers.Get("Content-Encoding") != "" {
		t.Fatalf(
			"fallback = %q with headers %v, want the decoded body without Content-Encoding",
			resp.Body,
			resp.Headers,
		)
	}
}

// countingConn records how much the client pulls off the socket. Counting what
// the server managed to write instead would measure kernel socket buffers,
// which hold megabytes on some machines and almost nothing on others.
type countingConn struct {
	net.Conn
	read *atomic.Int64
}

func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.read.Add(int64(n))
	return n, err
}

func countingDialClient(read *atomic.Int64) *Client {
	return NewClientWithOptions(WithHTTPFactory(func(Options) (*http.Client, error) {
		return &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return &countingConn{Conn: conn, read: read}, nil
			},
		}}, nil
	}))
}

func TestSSEStopsOnAnUnterminatedLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: "))
		chunk := []byte(strings.Repeat("A", 1<<16))
		for range 512 {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	var read atomic.Int64
	req := &restfile.Request{
		Method: "GET",
		URL:    srv.URL,
		SSE: &restfile.SSERequest{Options: restfile.SSEOptions{
			MaxBytes:     1 << 10,
			TotalTimeout: 30 * time.Second,
		}},
	}
	if _, err := countingDialClient(&read).ExecuteSSE(t.Context(), req, nil, Options{}); err != nil {
		t.Fatalf("ExecuteSSE: %v", err)
	}
	// The reader stops within one buffered fill of the limit. The transport has
	// a buffer of its own, so allow for both plus the response headers.
	if n := read.Load(); n > 64<<10 {
		t.Fatalf("the reader consumed %d bytes of one line against a 1KiB stream limit", n)
	}
}

func TestSSELineBudgetTracksTheStreamLimit(t *testing.T) {
	tests := []struct {
		name string
		opts restfile.SSEOptions
		set  Options
		read int64
		want int
	}{
		{name: "no stream limit", want: DefaultSSEMaxLineBytes},
		{
			name: "stream limit is larger",
			opts: restfile.SSEOptions{MaxBytes: 8 << 20},
			want: DefaultSSEMaxLineBytes,
		},
		{name: "stream limit is smaller", opts: restfile.SSEOptions{MaxBytes: 1024}, want: 1025},
		{
			name: "the largest stream limit still budgets a line",
			opts: restfile.SSEOptions{MaxBytes: math.MaxInt64},
			want: DefaultSSEMaxLineBytes,
		},
		{
			name: "part of the stream is read",
			opts: restfile.SSEOptions{MaxBytes: 1024},
			read: 1000,
			want: 25,
		},
		{
			name: "line limit is configured",
			opts: restfile.SSEOptions{MaxLineBytes: 4096},
			want: 4096,
		},
		{
			name: "the setting supplies the line limit",
			set:  Options{SSEMaxLineBytes: 2048},
			want: 2048,
		},
		{
			name: "the directive beats the setting",
			opts: restfile.SSEOptions{MaxLineBytes: 4096},
			set:  Options{SSEMaxLineBytes: 2048},
			want: 4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sseLimitsFor(tt.opts, tt.set).lineBudget(tt.read); got != tt.want {
				t.Fatalf("lineBudget() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSSEEventLimitFallsBackToTheSetting(t *testing.T) {
	tests := []struct {
		name string
		opts restfile.SSEOptions
		set  Options
		want int64
	}{
		{name: "default", want: DefaultSSEMaxEventBytes},
		{name: "setting", set: Options{SSEMaxEventBytes: 2048}, want: 2048},
		{
			name: "directive wins",
			opts: restfile.SSEOptions{MaxEventBytes: 4096},
			set:  Options{SSEMaxEventBytes: 2048},
			want: 4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sseLimitsFor(tt.opts, tt.set).event; got != tt.want {
				t.Fatalf("event limit = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWebSocketReadLimit(t *testing.T) {
	tests := []struct {
		name       string
		configured int64
		setting    int64
		want       int64
	}{
		{name: "default", want: defaultWebSocketMessageBytes},
		{name: "directive", configured: 4096, want: 4096},
		{name: "setting", setting: 2048, want: 2048},
		{name: "directive wins", configured: 4096, setting: 2048, want: 4096},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := webSocketReadLimit(tt.configured, tt.setting); got != tt.want {
				t.Fatalf("webSocketReadLimit(%d, %d) = %d, want %d",
					tt.configured, tt.setting, got, tt.want)
			}
		})
	}
}

func sseServer(t *testing.T, write func(w http.ResponseWriter, flush func())) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		flush := func() {
			if flusher != nil {
				flusher.Flush()
			}
		}
		write(w, flush)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sseTranscript(t *testing.T, url string, opts restfile.SSEOptions) SSETranscript {
	t.Helper()
	if opts.TotalTimeout == 0 {
		opts.TotalTimeout = 30 * time.Second
	}
	req := &restfile.Request{
		Method: "GET",
		URL:    url,
		SSE:    &restfile.SSERequest{Options: opts},
	}
	resp, err := NewClient(nil).ExecuteSSE(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("ExecuteSSE: %v", err)
	}

	var out SSETranscript
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("decode transcript: %v", err)
	}
	return out
}

func TestSSELineOverrunIsReported(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		_, _ = w.Write([]byte("data: " + strings.Repeat("A", 4096)))
		flush()
		<-t.Context().Done()
	})

	sum := sseTranscript(t, srv.URL, restfile.SSEOptions{MaxLineBytes: 512}).Summary
	if sum.Reason != sseReasonLineBytes {
		t.Fatalf("Reason = %q, want %q", sum.Reason, sseReasonLineBytes)
	}
	if !strings.Contains(sum.Error, "max-line-bytes") {
		t.Fatalf("Error = %q, want the limit and how to raise it", sum.Error)
	}
}

func TestSSEEventOverrunIsReported(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		for range 64 {
			_, _ = w.Write([]byte("data: " + strings.Repeat("A", 256) + "\n"))
			flush()
		}
		<-t.Context().Done()
	})

	sum := sseTranscript(t, srv.URL, restfile.SSEOptions{MaxEventBytes: 1024}).Summary
	if sum.Reason != sseReasonEventBytes {
		t.Fatalf("Reason = %q, want %q", sum.Reason, sseReasonEventBytes)
	}
	if !strings.Contains(sum.Error, "max-event-bytes") {
		t.Fatalf("Error = %q, want the limit and how to raise it", sum.Error)
	}
}

func TestSSEEventLimitCountsEachBlock(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		_, _ = w.Write([]byte(strings.Repeat("foo: 0123456789\n\n", 20) + "data: done\n\n"))
		flush()
	})

	transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{MaxEventBytes: 64})
	if sum := transcript.Summary; sum.Reason != sseReasonEOF || sum.Error != "" {
		t.Fatalf("summary = %+v, want a clean EOF", sum)
	}
	if len(transcript.Events) != 1 || transcript.Events[0].Data != "done" {
		t.Fatalf("events = %+v, want the one event after the ignored blocks", transcript.Events)
	}
}

func TestSSEStreamLimitEndsCleanly(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		for range 64 {
			_, _ = w.Write([]byte("data: " + strings.Repeat("A", 256) + "\n\n"))
			flush()
		}
		<-t.Context().Done()
	})

	sum := sseTranscript(t, srv.URL, restfile.SSEOptions{MaxBytes: 1024}).Summary
	if sum.Reason != sseReasonMaxBytes {
		t.Fatalf("Reason = %q, want %q", sum.Reason, sseReasonMaxBytes)
	}
}

func TestSSETranscriptReportsDiscardedEvents(t *testing.T) {
	const sent = 2000
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		for range sent {
			_, _ = w.Write([]byte("data: " + strings.Repeat("A", 512) + "\n\n"))
		}
		flush()
	})

	transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{})
	if transcript.Summary.Dropped == 0 {
		t.Fatal("Dropped = 0, want the discarded events to be reported")
	}
	if len(transcript.Events) >= sent {
		t.Fatalf("kept %d events, want the buffer to have discarded some", len(transcript.Events))
	}
}

func TestWebSocketAccumulatorStopsKeepingPayloadAtItsBudget(t *testing.T) {
	message := func() *stream.Event {
		return &stream.Event{
			Kind:      stream.KindWebSocket,
			Direction: stream.DirReceive,
			Payload:   []byte(strings.Repeat("A", 256)),
			WS:        stream.WSMetadata{Type: stream.WSText},
		}
	}

	acc := newWSAccumulator()
	acc.limit = 4 * message().Size()

	for range 8 {
		acc.consume(message())
	}

	if acc.summary.ReceivedCount != 8 {
		t.Fatalf("ReceivedCount = %d, want every message counted", acc.summary.ReceivedCount)
	}
	if len(acc.events) != 4 {
		t.Fatalf("kept %d events, want 4 within the budget", len(acc.events))
	}
	if acc.summary.Dropped != 4 {
		t.Fatalf("Dropped = %d, want 4", acc.summary.Dropped)
	}
}
