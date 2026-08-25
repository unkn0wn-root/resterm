package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func idledRun(t *testing.T, reason string) *sseRun {
	t.Helper()

	session := stream.NewSession(context.Background(), stream.KindSSE, stream.Config{})
	session.MarkOpen()
	t.Cleanup(func() { session.Close(nil) })

	run := &sseRun{session: session, summary: SSESummary{Reason: reason}}
	run.idled.Store(true)
	return run
}

func TestSSERunReportsAnIdleTimeoutOnEveryExit(t *testing.T) {
	stopped, stop := context.WithCancel(context.Background())
	stop()

	tests := []struct {
		name    string
		reached string
		want    string
	}{
		{name: "the read ended at eof", reached: sseReasonEOF, want: sseReasonIdle},
		{name: "the loop named nothing", want: sseReasonIdle},
		{name: "the loop saw the cancellation", reached: sseReasonIdle, want: sseReasonIdle},
		{name: "a limit was reached first", reached: sseReasonMaxEvents, want: sseReasonMaxEvents},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := idledRun(t, tt.reached)
			run.finish(stopped, nil)

			if run.summary.Reason != tt.want {
				t.Fatalf("Reason = %q, want %q", run.summary.Reason, tt.want)
			}
			if got := publishedReason(t, run.session); got != tt.want {
				t.Fatalf("published reason = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSSERunNamesWhatCancelledTheRead(t *testing.T) {
	deadline, cancelDeadline := context.WithDeadline(
		context.Background(),
		time.Now().Add(-time.Second),
	)
	defer cancelDeadline()

	stopped, stop := context.WithCancel(context.Background())
	stop()

	tests := []struct {
		name  string
		ctx   context.Context
		idled bool
		want  string
	}{
		{name: "the idle watcher", ctx: stopped, idled: true, want: sseReasonIdle},
		{name: "the total timeout", ctx: deadline, want: sseReasonTotal},
		{name: "the caller gave up", ctx: stopped, want: sseReasonCanceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := idledRun(t, sseReasonEOF)
			run.idled.Store(tt.idled)
			if got := run.ended(tt.ctx); got != tt.want {
				t.Fatalf("ended() = %q, want %q", got, tt.want)
			}
		})
	}
}

func publishedReason(t *testing.T, session *stream.Session) string {
	t.Helper()
	for _, evt := range session.EventsSnapshot() {
		if reason, ok := evt.Metadata[sseMetaReason]; ok {
			return reason
		}
	}
	t.Fatal("no summary event was published")
	return ""
}

func quietSSE(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: ping\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func readQuietSSE(t *testing.T, opts restfile.SSEOptions) (SSESummary, time.Duration) {
	t.Helper()

	req := &restfile.Request{
		Method: "GET",
		URL:    quietSSE(t),
		SSE:    &restfile.SSERequest{Options: opts},
	}
	start := time.Now()
	resp, err := NewClient(nil).ExecuteSSE(t.Context(), req, nil, Options{})
	took := time.Since(start)
	if err != nil {
		t.Fatalf("ExecuteSSE: %v", err)
	}

	var transcript SSETranscript
	if err := json.Unmarshal(resp.Body, &transcript); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	return transcript.Summary, took
}

func TestSSEIdleTimeoutEndsTheReadRatherThanWaitingForTheTotal(t *testing.T) {
	const idle = 100 * time.Millisecond

	tests := []struct {
		name  string
		total time.Duration
	}{
		{name: "a total timeout it must not wait for", total: 10 * time.Second},
		{name: "no total timeout to fall back on", total: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary, took := readQuietSSE(t, restfile.SSEOptions{
				IdleTimeout:  idle,
				TotalTimeout: tt.total,
			})

			if summary.Reason != sseReasonIdle {
				t.Fatalf("Reason = %q, want %q", summary.Reason, sseReasonIdle)
			}
			if took > 20*idle {
				t.Fatalf("took %s, want the idle timeout to end the read near %s", took, idle)
			}
			if summary.EventCount != 1 {
				t.Fatalf("EventCount = %d, want the event read before the stall", summary.EventCount)
			}
		})
	}
}

func TestSSETotalTimeoutEndsWithASummaryRatherThanAReadError(t *testing.T) {
	const total = 250 * time.Millisecond

	summary, took := readQuietSSE(t, restfile.SSEOptions{TotalTimeout: total})

	if summary.Reason != sseReasonTotal {
		t.Fatalf("Reason = %q, want %q", summary.Reason, sseReasonTotal)
	}
	if summary.Error != "" {
		t.Fatalf("Error = %q, want a reached limit to read as a clean stop", summary.Error)
	}
	if took < total {
		t.Fatalf("took %s, want it to run for the whole %s", took, total)
	}
	if summary.EventCount != 1 {
		t.Fatalf("EventCount = %d, want the event read before the stall", summary.EventCount)
	}
}

func eofOnCancel(t *testing.T) *Client {
	t.Helper()
	return newTestClientWithHTTPFactory(func(Options) (*http.Client, error) {
		rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			reader, writer := io.Pipe()
			go func() {
				_, _ = io.WriteString(writer, "data: ping\n\n")
				<-req.Context().Done()
				_ = writer.Close()
			}()
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Header:     make(http.Header),
				Body:       reader,
				Request:    req,
			}
			resp.Header.Set("Content-Type", "text/event-stream")
			return resp, nil
		})
		return &http.Client{Transport: rt}, nil
	})
}

func TestSSENamesTheLimitWhenACancelledReadEndsAtEOF(t *testing.T) {
	const limit = 150 * time.Millisecond

	tests := []struct {
		name        string
		opts        restfile.SSEOptions
		callerGives bool
		want        string
	}{
		{
			name: "the total timeout",
			opts: restfile.SSEOptions{TotalTimeout: limit},
			want: sseReasonTotal,
		},
		{
			name: "the idle timeout",
			opts: restfile.SSEOptions{IdleTimeout: limit, TotalTimeout: time.Minute},
			want: sseReasonIdle,
		},
		{
			name:        "the caller giving up",
			opts:        restfile.SSEOptions{TotalTimeout: time.Minute},
			callerGives: true,
			want:        sseReasonCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.callerGives {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				time.AfterFunc(limit, cancel)
				defer cancel()
			}

			req := &restfile.Request{
				Method: "GET",
				URL:    "https://example.com/events",
				SSE:    &restfile.SSERequest{Options: tt.opts},
			}
			resp, err := eofOnCancel(t).ExecuteSSE(ctx, req, vars.NewResolver(), Options{})
			if err != nil {
				t.Fatalf("ExecuteSSE: %v", err)
			}

			var transcript SSETranscript
			if err := json.Unmarshal(resp.Body, &transcript); err != nil {
				t.Fatalf("unmarshal transcript: %v", err)
			}
			if transcript.Summary.Reason != tt.want {
				t.Fatalf("Reason = %q, want %q", transcript.Summary.Reason, tt.want)
			}
			if transcript.Summary.EventCount != 1 {
				t.Fatalf("EventCount = %d, want the event read before the stall",
					transcript.Summary.EventCount)
			}
		})
	}
}

func multiLineSSE(t *testing.T, lines, each int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for range lines {
			fmt.Fprintf(w, "data: %s\n", strings.Repeat("x", each))
		}
		fmt.Fprint(w, "\n")
		w.(http.Flusher).Flush()
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func readSSE(t *testing.T, url string, opts restfile.SSEOptions) SSETranscript {
	t.Helper()
	opts.TotalTimeout = 30 * time.Second
	req := &restfile.Request{Method: "GET", URL: url, SSE: &restfile.SSERequest{Options: opts}}
	resp, err := NewClient(nil).ExecuteSSE(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("ExecuteSSE: %v", err)
	}
	var transcript SSETranscript
	if err := json.Unmarshal(resp.Body, &transcript); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	return transcript
}

func transcriptBytes(tr SSETranscript) int {
	total := 0
	for _, evt := range tr.Events {
		total += len(evt.Data)
	}
	return total
}

func TestSSEKeepsAnEventTheLimitsAllow(t *testing.T) {
	const each = 2 << 20

	tests := []struct {
		name  string
		lines int
		opts  restfile.SSEOptions
	}{
		{name: "a 6 MiB event on the defaults", lines: 3},
		{
			name:  "a 24 MiB event once the limits allow it",
			lines: 12,
			opts:  restfile.SSEOptions{MaxLineBytes: 4 << 20, MaxEventBytes: 32 << 20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := readSSE(t, multiLineSSE(t, tt.lines, each), tt.opts)

			if tr.Summary.Reason != sseReasonEOF {
				t.Fatalf("Reason = %q, want %q", tr.Summary.Reason, sseReasonEOF)
			}
			if tr.Summary.Dropped != 0 {
				t.Fatalf("Dropped = %d, want an allowed event to survive", tr.Summary.Dropped)
			}
			want := tt.lines*each + tt.lines - 1
			if got := transcriptBytes(tr); got != want {
				t.Fatalf("transcript holds %d bytes, want %d", got, want)
			}
		})
	}
}

func TestSSEReportsEventsTheBufferDiscarded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for range 24 {
			fmt.Fprintf(w, "data: %s\n\n", strings.Repeat("x", 1<<20))
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	tr := readSSE(t, srv.URL, restfile.SSEOptions{})

	if tr.Summary.EventCount != 24 {
		t.Fatalf("EventCount = %d, want every event to be read", tr.Summary.EventCount)
	}
	if tr.Summary.Dropped == 0 {
		t.Fatal("Dropped = 0, want the discarded events to be counted")
	}
	if got := int64(tr.Summary.EventCount - len(tr.Events)); got != tr.Summary.Dropped {
		t.Fatalf("Dropped = %d, want the %d events missing from the transcript",
			tr.Summary.Dropped, got)
	}
}

func TestSSEKeepsSeveralLargeEventsInTheTranscript(t *testing.T) {
	const (
		events = 4
		each   = 3 << 20
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for range events {
			fmt.Fprintf(w, "data: %s\n\n", strings.Repeat("x", each))
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	tr := readSSE(t, srv.URL, restfile.SSEOptions{})

	if tr.Summary.Dropped != 0 {
		t.Fatalf("Dropped = %d, want %d MiB of events to fit the buffer", tr.Summary.Dropped,
			(events*each)>>20)
	}
	if len(tr.Events) != events {
		t.Fatalf("transcript holds %d events, want %d", len(tr.Events), events)
	}
}

func TestSSEBudgetsEventsThatCarryOnlyAComment(t *testing.T) {
	const (
		count = 40
		each  = 1 << 20
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for range count {
			fmt.Fprintf(w, ": %s\n\n", strings.Repeat("x", each))
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()

	tr := readSSE(t, srv.URL, restfile.SSEOptions{})

	if tr.Summary.EventCount != count {
		t.Fatalf("EventCount = %d, want every event to be read", tr.Summary.EventCount)
	}

	retained := 0
	for _, evt := range tr.Events {
		retained += len(evt.Comment) + len(evt.Data)
	}
	if retained > DefaultSSESessionBytes {
		t.Fatalf("transcript retained %d bytes, want at most the %d byte budget",
			retained, DefaultSSESessionBytes)
	}
	if tr.Summary.Dropped == 0 {
		t.Fatal("Dropped = 0, want the comments the buffer discarded to be counted")
	}
}
