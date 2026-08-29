package httpx

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	sseMetaReason = "resterm.summary.reason"
	sseMetaBytes  = "resterm.summary.bytes"
	sseMetaEvents = "resterm.summary.events"
	sseMetaError  = "resterm.summary.error"
)

const (
	sseReasonEOF        = "eof"
	sseReasonErr        = "error"
	sseReasonIdle       = "timeout:idle"
	sseReasonMaxBytes   = "limit:max_bytes"
	sseReasonMaxEvents  = "limit:max_events"
	sseReasonLineBytes  = "limit:line_bytes"
	sseReasonEventBytes = "limit:event_bytes"
	sseReasonTotal      = "timeout:total"
	sseReasonCanceled   = "context_canceled"
)

const (
	DefaultSSEMaxLineBytes  = 4 << 20
	DefaultSSEMaxEventBytes = 8 << 20
	DefaultSSESessionBytes  = 16 << 20
)

var (
	errSSELineTooLong   = errors.New("sse line exceeds the line limit")
	errSSEEventTooLarge = errors.New("sse event exceeds the event limit")
)

type sseLimits struct {
	stream int64
	line   int64
	event  int64
}

func sseLimitsFor(opts restfile.SSEOptions) sseLimits {
	return sseLimits{
		stream: opts.MaxBytes,
		line:   cmp.Or(opts.MaxLineBytes, DefaultSSEMaxLineBytes),
		event:  cmp.Or(opts.MaxEventBytes, DefaultSSEMaxEventBytes),
	}
}

func (l sseLimits) sessionBytes() bytesize.Budget {
	// Keep room for a full event and the summary event that follows it.
	return bytesize.Of(max(int64(DefaultSSESessionBytes), 2*l.event))
}

func (l sseLimits) lineBudget(read int64) int {
	budget := l.line
	if l.stream > 0 {
		budget = min(budget, l.stream-read+1)
	}
	return int(budget)
}

func sseOverrun(subject string, limit int64, option string) error {
	return diag.Newf(
		diag.ClassProtocol,
		"sse %s exceeds %d bytes, raise it with @sse %s",
		subject,
		limit,
		option,
	)
}

type SSEEvent struct {
	Index     int       `json:"index"`
	ID        string    `json:"id,omitempty"`
	Event     string    `json:"event,omitempty"`
	Data      string    `json:"data,omitempty"`
	Comment   string    `json:"comment,omitempty"`
	Retry     int       `json:"retry,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type SSESummary struct {
	EventCount int           `json:"eventCount"`
	ByteCount  int64         `json:"byteCount"`
	Duration   time.Duration `json:"duration"`
	Reason     string        `json:"reason"`
	Dropped    int64         `json:"dropped,omitempty"`
	Error      string        `json:"error,omitempty"`
}

type SSETranscript struct {
	Events  []SSEEvent `json:"events"`
	Summary SSESummary `json:"summary"`
}

func (c *Client) StartSSE(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*StreamHandle, *Response, error) {
	if req == nil || req.SSE == nil {
		return nil, nil, diag.New(diag.ClassProtocol, "sse metadata missing")
	}

	streamOpts := req.SSE.Options
	streamCtx, cancel := ctxWithTimeout(ctx, streamOpts.TotalTimeout)

	httpReq, effectiveOpts, err := c.prepareHTTPRequest(streamCtx, req, resolver, opts)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if httpReq.Header.Get("Accept") == "" {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	client, err := c.streamClient(effectiveOpts)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	var k8sDiag *k8s.RequestDiag
	if effectiveOpts.K8s != nil && effectiveOpts.K8s.Active() {
		reqCtx, diag := k8s.BindRequestContext(httpReq.Context())
		httpReq = httpReq.WithContext(reqCtx)
		k8sDiag = diag
	}

	start := time.Now()
	httpResp, err := client.Do(httpReq)
	if err != nil {
		if k8sDiag != nil {
			err = k8s.AnnotateRequestError(err, start, k8sDiag)
		}
		cancel()
		return nil, nil, diag.WrapAs(diag.ClassProtocol, err, "perform sse request")
	}
	if verErr := checkHTTPVersion(httpResp, effectiveOpts.HTTPVersion); verErr != nil {
		_ = httpResp.Body.Close()
		cancel()
		return nil, nil, verErr
	}

	contentType := strings.ToLower(httpResp.Header.Get("Content-Type"))
	if httpResp.StatusCode >= 400 || !strings.Contains(contentType, "text/event-stream") {
		body, readErr := effectiveOpts.readBody(httpResp.Body)
		closeErr := httpResp.Body.Close()
		cancel()
		if readErr != nil {
			return nil, nil, readErr
		}
		if closeErr != nil {
			return nil, nil, diag.WrapAs(diag.ClassProtocol, closeErr, "close response body")
		}
		return nil, respFromHTTP(httpReq, httpResp, req, body, time.Since(start)), nil
	}

	meta := buildStreamMeta(req, httpReq, httpResp, effectiveOpts.BaseDir, metaDefaults{})

	session := stream.NewSession(streamCtx, stream.KindSSE, stream.Config{
		MaxBytes: sseLimitsFor(streamOpts).sessionBytes(),
	})
	session.MarkOpen()

	go func() {
		defer cancel()
		defer func() {
			_ = httpResp.Body.Close()
		}()
		runSSESession(session, httpResp.Body, streamOpts, cancel)
	}()

	return &StreamHandle{Session: session, Meta: meta}, nil, nil
}

func (c *Client) ExecuteSSE(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*Response, error) {
	handle, httpResp, err := c.StartSSE(ctx, req, resolver, opts)
	if err != nil {
		return nil, err
	}
	if httpResp != nil {
		return httpResp, nil
	}

	return CompleteSSE(handle)
}

func CompleteSSE(handle *StreamHandle) (*Response, error) {
	if handle == nil || handle.Session == nil {
		return nil, diag.New(diag.ClassProtocol, "sse session not available")
	}

	session := handle.Session
	<-session.Done()

	acc := newSSEAccumulator()
	for _, evt := range session.EventsSnapshot() {
		acc.consume(evt)
	}

	stats := session.StatsSnapshot()
	acc.summary.Dropped = int64(stats.Evicted)
	if !stats.EndedAt.IsZero() {
		acc.summary.Duration = stats.EndedAt.Sub(stats.StartedAt)
	} else {
		acc.summary.Duration = time.Since(handle.Meta.ConnectedAt)
	}
	if acc.summary.ByteCount == 0 {
		acc.summary.ByteCount = int64(stats.BytesTotal)
	}
	if acc.summary.EventCount == 0 {
		acc.summary.EventCount = len(acc.events)
	}
	state, serr := session.State()
	if acc.summary.Error == "" && serr != nil {
		acc.summary.Error = serr.Error()
	}
	if acc.summary.Reason == "" {
		if serr != nil || state == stream.StateFailed {
			acc.summary.Reason = sseReasonErr
		} else {
			acc.summary.Reason = sseReasonEOF
		}
	} else if acc.summary.Reason == sseReasonEOF && (state == stream.StateFailed || serr != nil) {
		acc.summary.Reason = sseReasonErr
	}

	transcript := SSETranscript{Events: acc.events, Summary: acc.summary}
	body, err := json.MarshalIndent(transcript, "", "  ")
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "encode sse transcript")
	}

	headers := handle.Meta.Headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("Content-Type", streamContentTypeJSON)
	headers.Set(StreamHeaderType, "sse")
	headers.Set(StreamHeaderSummary, sseSummaryLine(transcript.Summary))

	return streamResp(handle.Meta, headers, body, acc.summary.Duration), nil
}

// Idle timer watches for activity resets - each incoming byte triggers a reset.
// The drain logic after Stop() handles the race where the timer fires just before we reset.
func runSSESession(
	session *stream.Session,
	body io.ReadCloser,
	opts restfile.SSEOptions,
	stopRead context.CancelFunc,
) {
	limits := sseLimitsFor(opts)
	run := &sseRun{
		session: session,
		reader:  bufio.NewReader(body),
		opts:    opts,
		limits:  limits,
		builder: sseEventBuilder{limit: limits.event},
		summary: SSESummary{Reason: sseReasonEOF},
	}

	ctx := session.Context()
	var stopIdle func()
	run.idle, stopIdle = startIdleWatch(ctx, opts.IdleTimeout, func() {
		run.idled.Store(true)
		// Cancel the request because stopping the session does not unblock the read.
		stopRead()
	})
	defer stopIdle()

	run.finish(ctx, run.loop(ctx))
}

type sseRun struct {
	session *stream.Session
	reader  *bufio.Reader
	opts    restfile.SSEOptions
	limits  sseLimits
	builder sseEventBuilder
	summary SSESummary
	failure error
	idle    chan<- struct{}
	idled   atomic.Bool
	index   int
	events  int
	bytes   int64
}

func (r *sseRun) loop(ctx context.Context) error {
	for {
		if r.capped() {
			r.stop(sseReasonMaxBytes)
			return nil
		}

		line, err := r.next()

		if errors.Is(err, errSSELineTooLong) {
			if r.capped() {
				r.summary.Reason = sseReasonMaxBytes
				return nil
			}
			r.summary.Reason = sseReasonLineBytes
			r.failure = sseOverrun("line", r.limits.line, "max-line-bytes")
			return nil
		}

		capped := r.capped()

		if err != nil && !errors.Is(err, io.EOF) {
			if ctx.Err() != nil {
				r.stop(r.ended(ctx))
				return nil
			}
			return diag.WrapAs(diag.ClassProtocol, err, "read sse stream")
		}

		if trimmed := strings.TrimRight(line, "\r\n"); trimmed == "" {
			if r.flush() && r.opts.MaxEvents > 0 && r.events >= r.opts.MaxEvents {
				r.summary.Reason = sseReasonMaxEvents
				return nil
			}
		} else if cerr := r.builder.consume(trimmed); cerr != nil {
			if !errors.Is(cerr, errSSEEventTooLarge) {
				return cerr
			}
			r.summary.Reason = sseReasonEventBytes
			r.failure = sseOverrun("event", r.limits.event, "max-event-bytes")
			return nil
		}

		if capped {
			r.stop(sseReasonMaxBytes)
			return nil
		}

		if errors.Is(err, io.EOF) {
			r.flush()
			return nil
		}

		if ctx.Err() != nil {
			r.stop(r.ended(ctx))
			return nil
		}
	}
}

func (r *sseRun) next() (string, error) {
	line, err := readSSELine(r.reader, r.limits.lineBudget(r.bytes))
	if len(line) > 0 {
		r.bytes += int64(len(line))
		select {
		case r.idle <- struct{}{}:
		default:
		}
	}
	return line, err
}

func (r *sseRun) capped() bool {
	return r.limits.stream > 0 && r.bytes >= r.limits.stream
}

func (r *sseRun) ended(ctx context.Context) string {
	switch {
	case r.idled.Load():
		return sseReasonIdle
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return sseReasonTotal
	default:
		return sseReasonCanceled
	}
}

func (r *sseRun) stop(reason string) {
	if r.summary.Reason == "" || r.summary.Reason == sseReasonEOF {
		r.summary.Reason = reason
	}
}

func (r *sseRun) flush() bool {
	evt, ok := r.builder.finalize(r.index)
	if !ok {
		return false
	}
	publishSSEEvent(r.session, evt)
	r.index++
	r.events++
	return true
}

func (r *sseRun) finish(ctx context.Context, err error) {
	if err != nil {
		r.session.Close(err)
		return
	}

	// The transport may report a cancelled read as EOF, so check the context.
	if ctx.Err() != nil {
		r.stop(r.ended(ctx))
	}

	r.summary.EventCount = r.events
	r.summary.ByteCount = r.bytes
	metadata := map[string]string{
		sseMetaReason: r.summary.Reason,
		sseMetaBytes:  strconv.FormatInt(r.summary.ByteCount, 10),
		sseMetaEvents: strconv.Itoa(r.summary.EventCount),
	}
	if r.failure != nil {
		metadata[sseMetaError] = r.failure.Error()
	}
	r.session.Publish(&stream.Event{
		Kind:      stream.KindSSE,
		Direction: stream.DirNA,
		Timestamp: time.Now(),
		Metadata:  metadata,
	})

	closeErr := r.failure
	if closeErr == nil && ctx.Err() != nil && r.summary.Reason == sseReasonCanceled {
		closeErr = ctx.Err()
	}
	r.session.Close(closeErr)
}

func readSSELine(r *bufio.Reader, limit int) (string, error) {
	var line strings.Builder
	for {
		chunk, err := r.ReadSlice('\n')
		if line.Len()+len(chunk) > limit {
			line.Write(chunk[:limit-line.Len()])
			return line.String(), errSSELineTooLong
		}
		line.Write(chunk)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line.String(), err
		}
	}
}

func sseSummaryLine(sum SSESummary) string {
	line := fmt.Sprintf(
		"events=%d bytes=%d reason=%s",
		sum.EventCount,
		sum.ByteCount,
		sum.Reason,
	)
	if sum.Dropped > 0 {
		line += fmt.Sprintf(" dropped=%d", sum.Dropped)
	}
	return line
}
