package httpx

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

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

func TestResponseLimitCountsDecompressedBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/plain")
		zw := gzip.NewWriter(w)
		defer func() { _ = zw.Close() }()
		chunk := make([]byte, 1<<16)
		for sent := 0; sent < 8<<20; sent += len(chunk) {
			if _, err := zw.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	_, err := NewClient(nil).Execute(
		t.Context(),
		&restfile.Request{Method: "GET", URL: srv.URL},
		nil,
		Options{MaxResponseBytes: bytesize.Of(1 << 20), Timeout: 30 * time.Second},
	)
	tooLarge(t, err)
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

func TestSSEStopsOnAnUnterminatedLine(t *testing.T) {
	var written atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: "))
		chunk := []byte(strings.Repeat("A", 1<<16))
		for range 512 {
			n, err := w.Write(chunk)
			written.Add(int64(n))
			if err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	req := &restfile.Request{
		Method: "GET",
		URL:    srv.URL,
		SSE: &restfile.SSERequest{Options: restfile.SSEOptions{
			MaxBytes:     1 << 10,
			TotalTimeout: 30 * time.Second,
		}},
	}
	if _, err := NewClient(nil).ExecuteSSE(t.Context(), req, nil, Options{}); err != nil {
		t.Fatalf("ExecuteSSE: %v", err)
	}
	if n := written.Load(); n > 1<<20 {
		t.Fatalf("the reader consumed %d bytes of one line against a 1KiB stream limit", n)
	}
}

func TestSSELineBudgetTracksTheStreamLimit(t *testing.T) {
	tests := []struct {
		name string
		opts restfile.SSEOptions
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sseLimitsFor(tt.opts).lineBudget(tt.read); got != tt.want {
				t.Fatalf("lineBudget() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWebSocketReadLimit(t *testing.T) {
	if got := webSocketReadLimit(0); got != defaultWebSocketMessageBytes {
		t.Fatalf("webSocketReadLimit(0) = %d, want %d", got, defaultWebSocketMessageBytes)
	}
	if got := webSocketReadLimit(4096); got != 4096 {
		t.Fatalf("webSocketReadLimit(4096) = %d, want 4096", got)
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
			Metadata:  map[string]string{wsMetaType: "text"},
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

func TestWebSocketAccumulatorKeepsTheCloseEvent(t *testing.T) {
	acc := newWSAccumulator()
	acc.limit = 16

	acc.consume(&stream.Event{
		Direction: stream.DirReceive,
		Payload:   []byte(strings.Repeat("A", 64)),
		Metadata:  map[string]string{wsMetaType: "text"},
	})
	acc.consume(&stream.Event{
		Direction: stream.DirReceive,
		Metadata: map[string]string{
			wsMetaType:        "close",
			wsMetaClosedBy:    "server",
			wsMetaCloseReason: "bye",
		},
	})

	if acc.summary.ClosedBy != "server" || acc.summary.CloseReason != "bye" {
		t.Fatalf("summary lost the close details: %+v", acc.summary)
	}
}
