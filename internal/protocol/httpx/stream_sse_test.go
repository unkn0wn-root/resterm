package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
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
	limit, cancelLimit := ctxWithTimeout(context.Background(), time.Nanosecond, errStreamLimit)
	defer cancelLimit()
	<-limit.Done()

	caller, cancelCaller := context.WithDeadline(
		context.Background(),
		time.Now().Add(-time.Second),
	)
	defer cancelCaller()
	outlived, cancelOutlived := ctxWithTimeout(caller, time.Hour, errStreamLimit)
	defer cancelOutlived()

	stopped, stop := context.WithCancel(context.Background())
	stop()

	tests := []struct {
		name  string
		ctx   context.Context
		idled bool
		want  string
	}{
		{name: "the idle watcher", ctx: stopped, idled: true, want: sseReasonIdle},
		{name: "the duration the request asked for", ctx: limit, want: sseReasonTotal},
		{name: "a deadline the caller set", ctx: caller, want: sseReasonDeadline},
		{name: "a caller deadline inside a longer duration", ctx: outlived, want: sseReasonDeadline},
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

func TestSSEIgnoresAMalformedRetry(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		_, _ = io.WriteString(w, "data: one\n\nretry: abc\ndata: two\n\n")
		flush()
	})

	transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{})
	if sum := transcript.Summary; sum.Reason != sseReasonEOF || sum.Error != "" {
		t.Fatalf("summary = %+v, want a clean EOF", sum)
	}
	if len(transcript.Events) != 2 || transcript.Events[1].Data != "two" || transcript.Events[1].Retry != 0 {
		t.Fatalf("events = %+v, want both events without a retry", transcript.Events)
	}
}

func TestSSETranscriptSplitsLinesAtCROrLF(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		data   []string
	}{
		{name: "CR", chunks: []string{"data: a\rdata: b\r\r"}, data: []string{"a\nb"}},
		{name: "CR inside a line", chunks: []string{"data: a\rb\n\n"}, data: []string{"a"}},
		{name: "CR then CRLF", chunks: []string{"data: a\r\r\n"}, data: []string{"a"}},
		{name: "CRLF across writes", chunks: []string{"data: a\r", "\ndata: b\r\n\r\n"}, data: []string{"a\nb"}},
		{name: "leading BOM", chunks: []string{"\ufeffdata: x\n\n"}, data: []string{"x"}},
		{name: "second BOM", chunks: []string{"\ufeff\ufeffdata: x\n\ndata: y\n\n"}, data: []string{"y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
				for _, chunk := range tt.chunks {
					_, _ = io.WriteString(w, chunk)
					flush()
				}
			})
			transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{})
			var data []string
			for _, evt := range transcript.Events {
				data = append(data, evt.Data)
			}
			if !slices.Equal(data, tt.data) {
				t.Fatalf("data = %q, want %q", data, tt.data)
			}
			sum := transcript.Summary
			if sum.Reason != sseReasonEOF || sum.Error != "" {
				t.Fatalf("summary = %+v, want a clean EOF", sum)
			}
			if want := len(strings.Join(tt.chunks, "")); sum.ByteCount != int64(want) {
				t.Fatalf("ByteCount = %d, want %d", sum.ByteCount, want)
			}
		})
	}
}

func TestSSEDispatchesACREventWithoutWaitingForMore(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		_, _ = io.WriteString(w, "data: x\r\r")
		flush()
		<-t.Context().Done()
	})

	transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{IdleTimeout: 200 * time.Millisecond})
	if len(transcript.Events) != 1 || transcript.Events[0].Data != "x" {
		t.Fatalf("events = %+v, want the event before the idle timeout", transcript.Events)
	}
}

func TestSSEIdleCountsDataInsideALine(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		_, _ = io.WriteString(w, "data: ")
		flush()
		for range 6 {
			time.Sleep(50 * time.Millisecond)
			_, _ = io.WriteString(w, strings.Repeat("x", 100))
			flush()
		}
		_, _ = io.WriteString(w, "\n\n")
		flush()
	})

	transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{IdleTimeout: 150 * time.Millisecond})
	if sum := transcript.Summary; sum.Reason != sseReasonEOF {
		t.Fatalf("Reason = %q after %d bytes, want %q", sum.Reason, sum.ByteCount, sseReasonEOF)
	}
	if len(transcript.Events) != 1 || transcript.Events[0].Data != strings.Repeat("x", 600) {
		t.Fatalf("events = %d, want the one line that kept arriving", len(transcript.Events))
	}
}

func TestSSETranscriptKeepsFieldWhitespace(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		data  string
		event string
		id    string
	}{
		{
			name: "multiline data",
			raw:  "data:  first\ndata: \tsecond\ndata: \t third\n\n",
			data: " first\n\tsecond\n\t third",
		},
		{
			name:  "metadata overwrite",
			raw:   "event: old\nid: old\nevent:  update\nid: \t42\ndata: payload\n\n",
			data:  "payload",
			event: " update",
			id:    "\t42",
		},
		{
			name: "whitespace only data",
			raw:  "data:   \ndata: \t\n\n",
			data: "  \n\t",
		},
		{
			name:  "trailing whitespace",
			raw:   "event:  update \t\nid:  42 \t\ndata:  payload \t\n\n",
			data:  " payload \t",
			event: " update \t",
			id:    " 42 \t",
		},
		{
			name: "empty data line",
			raw:  "data:\ndata:  tail\n\n",
			data: "\n tail",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
				_, _ = io.WriteString(w, tt.raw)
				flush()
			})
			transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{})
			if len(transcript.Events) != 1 {
				t.Fatalf("events = %+v, want one event", transcript.Events)
			}
			evt := transcript.Events[0]
			if evt.Data != tt.data || evt.Event != tt.event || evt.ID != tt.id {
				t.Fatalf("event = %+v, want data %q, event %q, id %q", evt, tt.data, tt.event, tt.id)
			}
			if transcript.Summary.Reason != sseReasonEOF || transcript.Summary.Error != "" {
				t.Fatalf("summary = %+v, want a clean EOF", transcript.Summary)
			}
			if transcript.Summary.ByteCount != int64(len(tt.raw)) {
				t.Fatalf("ByteCount = %d, want %d", transcript.Summary.ByteCount, len(tt.raw))
			}
		})
	}
}

func TestSSEIndentedDataCountsTowardEventLimit(t *testing.T) {
	const line = "data:  \tpayload"
	for _, limit := range []int64{int64(len(line)), int64(len(line) - 1)} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
				_, _ = io.WriteString(w, line+"\n\n")
				flush()
			})
			transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{MaxEventBytes: limit})
			if limit < int64(len(line)) {
				if transcript.Summary.Reason != sseReasonEventBytes || len(transcript.Events) != 0 {
					t.Fatalf("transcript = %+v, want the event byte limit", transcript)
				}
				return
			}
			if len(transcript.Events) != 1 || transcript.Events[0].Data != " \tpayload" {
				t.Fatalf("events = %+v, want the indented payload", transcript.Events)
			}
		})
	}
}

func quietSSE(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "data: ping\n\n")
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
			_, _ = fmt.Fprintf(w, "data: %s\n", strings.Repeat("x", each))
		}
		_, _ = fmt.Fprint(w, "\n")
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
			_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.Repeat("x", 1<<20))
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
			_, _ = fmt.Fprintf(w, "data: %s\n\n", strings.Repeat("x", each))
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
			_, _ = fmt.Fprintf(w, ": %s\n\n", strings.Repeat("x", each))
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

func TestSSESummaryErr(t *testing.T) {
	tests := []struct {
		name    string
		summary SSESummary
		class   diag.Class
	}{
		{name: "server closed the stream", summary: SSESummary{Reason: sseReasonEOF}},
		{name: "stopped at max events", summary: SSESummary{Reason: sseReasonMaxEvents}},
		{name: "stopped at max bytes", summary: SSESummary{Reason: sseReasonMaxBytes}},
		{name: "went quiet", summary: SSESummary{Reason: sseReasonIdle}},
		{name: "ran out of time", summary: SSESummary{Reason: sseReasonTotal}},
		{
			name:    "read failed",
			summary: SSESummary{Reason: sseReasonErr, Error: "read sse stream: boom"},
			class:   diag.ClassProtocol,
		},
		{
			name:    "line over the limit",
			summary: SSESummary{Reason: sseReasonLineBytes, Error: "sse line exceeds 16 bytes"},
			class:   diag.ClassProtocol,
		},
		{
			name:    "event over the limit",
			summary: SSESummary{Reason: sseReasonEventBytes, Error: "sse event exceeds 16 bytes"},
			class:   diag.ClassProtocol,
		},
		{
			name:    "canceled",
			summary: SSESummary{Reason: sseReasonCanceled, Error: "context canceled"},
			class:   diag.ClassCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.summary.Err()
			if tt.class == "" {
				if err != nil {
					t.Fatalf("Err() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Err() = nil, want a %s failure", tt.class)
			}
			if got := diag.Classes(err); !slices.Contains(got, tt.class) {
				t.Fatalf("Err() classes = %v, want %s", got, tt.class)
			}
			if tt.summary.Error != "" && err.Error() != tt.summary.Error {
				t.Fatalf("Err() = %q, want %q", err, tt.summary.Error)
			}
		})
	}
}

func TestSSEAcceptsTheLargestByteLimit(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		_, _ = w.Write([]byte("data: hello\n\n"))
		flush()
	})

	transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{MaxBytes: math.MaxInt64})
	if len(transcript.Events) != 1 || transcript.Events[0].Data != "hello" {
		t.Fatalf("events = %+v, want the one event the server sent", transcript.Events)
	}
}

func TestSSEFailureSummaryCountsWhatTheRunRead(t *testing.T) {
	const events = 5

	var raw strings.Builder
	for i := range events {
		_, _ = fmt.Fprintf(&raw, "data: event-%d\n\n", i)
	}
	reset := &net.OpError{Op: "read", Err: errors.New("connection reset by peer")}

	req := &restfile.Request{Method: "GET", URL: "https://example.com/events", SSE: &restfile.SSERequest{}}
	resp, err := failingSSEClient(t, raw.String(), reset).ExecuteSSE(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("ExecuteSSE: %v", err)
	}
	transcript, err := DecodeSSETranscript(resp.Body)
	if err != nil {
		t.Fatalf("decode transcript: %v", err)
	}

	sum := transcript.Summary
	if sum.Reason != sseReasonErr {
		t.Fatalf("Reason = %q, want %q", sum.Reason, sseReasonErr)
	}
	if !strings.Contains(sum.Error, "read sse stream") {
		t.Fatalf("Error = %q, want the read failure", sum.Error)
	}
	if sum.ErrorClass != diag.ClassNetwork {
		t.Fatalf("ErrorClass = %q, want %q", sum.ErrorClass, diag.ClassNetwork)
	}
	if sum.EventCount != events {
		t.Fatalf("EventCount = %d, want the %d events the run read", sum.EventCount, events)
	}
	if sum.ByteCount != int64(raw.Len()) {
		t.Fatalf("ByteCount = %d, want the %d bytes the run read", sum.ByteCount, raw.Len())
	}
}

func TestSSEEventsKeepTheirStreamIndex(t *testing.T) {
	const sent = 1200

	srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
		for i := range sent {
			_, _ = fmt.Fprintf(w, "data: event-%d\n\n", i)
		}
		flush()
	})

	transcript := sseTranscript(t, srv.URL, restfile.SSEOptions{})
	if transcript.Summary.EventCount != sent {
		t.Fatalf("EventCount = %d, want every event read", transcript.Summary.EventCount)
	}
	if len(transcript.Events) >= sent {
		t.Fatalf("kept %d events, want the buffer to have discarded some", len(transcript.Events))
	}
	for _, evt := range transcript.Events {
		if want := fmt.Sprintf("event-%d", evt.Index); evt.Data != want {
			t.Fatalf("event at index %d holds %q, want %q", evt.Index, evt.Data, want)
		}
	}
}

func TestSSEStreamWithoutEventsCountsNothing(t *testing.T) {
	srv := sseServer(t, func(w http.ResponseWriter, flush func()) { flush() })

	sum := sseTranscript(t, srv.URL, restfile.SSEOptions{}).Summary
	if sum.Reason != sseReasonEOF {
		t.Fatalf("Reason = %q, want %q", sum.Reason, sseReasonEOF)
	}
	if sum.EventCount != 0 || sum.ByteCount != 0 {
		t.Fatalf("summary = %d events and %d bytes, want an empty stream", sum.EventCount, sum.ByteCount)
	}
}

// sseBody serves its data and then fails, so the run ends on a read error.
type sseBody struct {
	data []byte
	err  error
}

func (b *sseBody) Read(p []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, b.err
	}
	n := copy(p, b.data)
	b.data = b.data[n:]
	return n, nil
}

func (b *sseBody) Close() error { return nil }

func failingSSEClient(t *testing.T, data string, err error) *Client {
	t.Helper()
	return newTestClientWithHTTPFactory(func(Options) (*http.Client, error) {
		rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Proto:      "HTTP/1.1",
				Header:     make(http.Header),
				Body:       &sseBody{data: []byte(data), err: err},
				Request:    req,
			}
			resp.Header.Set("Content-Type", "text/event-stream")
			return resp, nil
		})
		return &http.Client{Transport: rt}, nil
	})
}

func TestSSEReadFailureKeepsItsClass(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want diag.Class
	}{
		{
			name: "the connection dropped",
			err:  &net.OpError{Op: "read", Err: errors.New("connection reset by peer")},
			want: diag.ClassNetwork,
		},
		{
			name: "a read that names nothing",
			err:  errors.New("the stream broke"),
			want: diag.ClassProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{
				Method: "GET",
				URL:    "https://example.com/events",
				SSE:    &restfile.SSERequest{},
			}
			resp, err := failingSSEClient(t, "data: ping\n\n", tt.err).ExecuteSSE(t.Context(), req, nil, Options{})
			if err != nil {
				t.Fatalf("ExecuteSSE: %v", err)
			}

			transcript, err := DecodeSSETranscript(resp.Body)
			if err != nil {
				t.Fatalf("decode transcript: %v", err)
			}
			if transcript.Summary.Reason != sseReasonErr {
				t.Fatalf("Reason = %q, want %q", transcript.Summary.Reason, sseReasonErr)
			}
			if len(transcript.Events) != 1 {
				t.Fatalf("kept %d events, want the one read before the failure", len(transcript.Events))
			}
			if got := diag.ClassOf(transcript.Summary.Err()); got != tt.want {
				t.Fatalf("Err() class = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSSETellsACallerDeadlineFromTheDurationLimit(t *testing.T) {
	tests := []struct {
		name     string
		total    time.Duration
		deadline time.Duration
		reason   string
		want     diag.Class
	}{
		{
			name:   "the duration the request asked for",
			total:  250 * time.Millisecond,
			reason: sseReasonTotal,
		},
		{
			name:     "a deadline the caller set",
			deadline: 250 * time.Millisecond,
			reason:   sseReasonDeadline,
			want:     diag.ClassTimeout,
		},
		{
			name:     "a caller deadline inside a longer duration",
			total:    time.Minute,
			deadline: 250 * time.Millisecond,
			reason:   sseReasonDeadline,
			want:     diag.ClassTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := sseServer(t, func(w http.ResponseWriter, flush func()) {
				_, _ = w.Write([]byte("data: ping\n\n"))
				flush()
				<-t.Context().Done()
			})

			ctx := t.Context()
			if tt.deadline > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.deadline)
				defer cancel()
			}

			req := &restfile.Request{
				Method: "GET",
				URL:    srv.URL,
				SSE:    &restfile.SSERequest{Options: restfile.SSEOptions{TotalTimeout: tt.total}},
			}
			resp, err := NewClient(nil).ExecuteSSE(ctx, req, nil, Options{})
			if err != nil {
				t.Fatalf("ExecuteSSE: %v", err)
			}

			transcript, err := DecodeSSETranscript(resp.Body)
			if err != nil {
				t.Fatalf("decode transcript: %v", err)
			}
			sum := transcript.Summary
			if sum.Reason != tt.reason {
				t.Fatalf("Reason = %q, want %q", sum.Reason, tt.reason)
			}
			if len(transcript.Events) != 1 {
				t.Fatalf("kept %d events, want the one read before the stop", len(transcript.Events))
			}
			if tt.want == "" {
				if err := sum.Err(); err != nil {
					t.Fatalf("a reached duration limit ended as a failure: %v", err)
				}
				return
			}
			if got := diag.ClassOf(sum.Err()); got != tt.want {
				t.Fatalf("Err() class = %q, want %q", got, tt.want)
			}
		})
	}
}

func BenchmarkSSERun(b *testing.B) {
	limits := sseLimitsFor(restfile.SSEOptions{}, Options{})
	for _, bc := range []struct{ name, body string }{
		{"one 3MiB line", "data: " + strings.Repeat("x", 3<<20) + "\n\n"},
		{"small LF events", strings.Repeat("id: 1\ndata: hello\n\n", 1<<12)},
		{"small CRLF events", strings.Repeat("id: 1\r\ndata: hello\r\n\r\n", 1<<12)},
	} {
		b.Run(bc.name, func(b *testing.B) {
			b.SetBytes(int64(len(bc.body)))
			for b.Loop() {
				s := stream.NewSession(b.Context(), stream.KindSSE, stream.Config{MaxBytes: limits.sessionBytes()})
				s.MarkOpen()
				runSSESession(
					s.Context(),
					s,
					io.NopCloser(strings.NewReader(bc.body)),
					restfile.SSEOptions{},
					limits,
					func() {},
				)
			}
		})
	}
}

func TestSSESessionCancelStopsAWaitingRead(t *testing.T) {
	hold := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: ping\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-hold:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(hold) })

	req := &restfile.Request{Method: http.MethodGet, URL: srv.URL, SSE: &restfile.SSERequest{}}
	handle, _, err := NewClient(nil).StartSSE(t.Context(), req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	listener := handle.Session.Subscribe()
	defer listener.Cancel()
	if len(listener.Snapshot.Events) == 0 {
		<-listener.C
	}
	handle.Session.Cancel()
	select {
	case <-handle.Session.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("canceling the session did not stop the read")
	}
	if got := publishedReason(t, handle.Session); got != sseReasonCanceled {
		t.Fatalf("reason = %q, want %q", got, sseReasonCanceled)
	}
}
