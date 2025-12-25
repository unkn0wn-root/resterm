package httpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	streamHeaderType    = "X-Resterm-Stream-Type"
	streamHeaderSummary = "X-Resterm-Stream-Summary"

	sseMetaReason = "resterm.summary.reason"
	sseMetaBytes  = "resterm.summary.bytes"
	sseMetaEvents = "resterm.summary.events"
)

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
		return nil, nil, errdef.New(errdef.CodeHTTP, "sse metadata missing")
	}

	streamOpts := req.SSE.Options
	var (
		streamCtx context.Context
		cancel    context.CancelFunc
	)
	if streamOpts.TotalTimeout > 0 {
		streamCtx, cancel = context.WithTimeout(ctx, streamOpts.TotalTimeout)
	} else {
		streamCtx, cancel = context.WithCancel(ctx)
	}

	httpReq, effectiveOpts, err := c.prepareHTTPRequest(streamCtx, req, resolver, opts)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if httpReq.Header.Get("Accept") == "" {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	factory := c.resolveHTTPFactory()
	if factory == nil {
		cancel()
		return nil, nil, errdef.New(errdef.CodeHTTP, "http client factory unavailable")
	}
	client, err := factory(effectiveOpts)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	client.Timeout = 0

	start := time.Now()
	httpResp, err := client.Do(httpReq)
	if err != nil {
		cancel()
		return nil, nil, errdef.Wrap(errdef.CodeHTTP, err, "perform sse request")
	}

	effURL := ""
	if httpResp.Request != nil && httpResp.Request.URL != nil {
		effURL = httpResp.Request.URL.String()
	} else if httpReq.URL != nil {
		effURL = httpReq.URL.String()
	}

	contentType := strings.ToLower(httpResp.Header.Get("Content-Type"))
	if httpResp.StatusCode >= 400 || !strings.Contains(contentType, "text/event-stream") {
		body, readErr := io.ReadAll(httpResp.Body)
		closeErr := httpResp.Body.Close()
		cancel()
		if readErr != nil {
			return nil, nil, errdef.Wrap(errdef.CodeHTTP, readErr, "read response body")
		}
		if closeErr != nil {
			return nil, nil, errdef.Wrap(errdef.CodeHTTP, closeErr, "close response body")
		}
		meta := captureReqMeta(httpReq, httpResp)
		return nil, &Response{
			Status:         httpResp.Status,
			StatusCode:     httpResp.StatusCode,
			Proto:          httpResp.Proto,
			Headers:        httpResp.Header.Clone(),
			ReqMethod:      meta.method,
			RequestHeaders: meta.headers,
			ReqHost:        meta.host,
			ReqLen:         meta.length,
			ReqTE:          meta.te,
			Body:           body,
			Duration:       time.Since(start),
			EffectiveURL:   effURL,
			Request:        req,
		}, nil
	}

	reqMeta := captureReqMeta(httpReq, httpResp)

	meta := StreamMeta{
		Status:         httpResp.Status,
		StatusCode:     httpResp.StatusCode,
		Proto:          httpResp.Proto,
		Headers:        httpResp.Header.Clone(),
		RequestHeaders: reqMeta.headers,
		RequestMethod:  reqMeta.method,
		RequestHost:    reqMeta.host,
		RequestLength:  reqMeta.length,
		RequestTE:      reqMeta.te,
		EffectiveURL:   effURL,
		ConnectedAt:    time.Now(),
		Request:        req,
		BaseDir:        effectiveOpts.BaseDir,
	}

	session := stream.NewSession(streamCtx, stream.KindSSE, stream.Config{})
	session.MarkOpen()

	go func() {
		defer cancel()
		defer func() {
			_ = httpResp.Body.Close()
		}()
		runSSESession(session, httpResp.Body, streamOpts)
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
		return nil, errdef.New(errdef.CodeHTTP, "sse session not available")
	}

	session := handle.Session
	<-session.Done()

	acc := newSSEAccumulator()
	for _, evt := range session.EventsSnapshot() {
		acc.consume(evt)
	}

	stats := session.StatsSnapshot()
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
	if acc.summary.Reason == "" {
		if state, serr := session.State(); serr != nil {
			acc.summary.Reason = serr.Error()
		} else if state == stream.StateFailed {
			acc.summary.Reason = "error"
		} else {
			acc.summary.Reason = "eof"
		}
	}

	transcript := SSETranscript{Events: acc.events, Summary: acc.summary}
	body, err := json.MarshalIndent(transcript, "", "  ")
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "encode sse transcript")
	}

	headers := handle.Meta.Headers.Clone()
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("Content-Type", "application/json; charset=utf-8")
	headers.Set(streamHeaderType, "sse")
	headers.Set(
		streamHeaderSummary,
		fmt.Sprintf(
			"events=%d bytes=%d reason=%s",
			transcript.Summary.EventCount,
			transcript.Summary.ByteCount,
			transcript.Summary.Reason,
		),
	)

	var reqHeaders http.Header
	if handle.Meta.RequestHeaders != nil {
		reqHeaders = handle.Meta.RequestHeaders.Clone()
	}
	return &Response{
		Status:         handle.Meta.Status,
		StatusCode:     handle.Meta.StatusCode,
		Proto:          handle.Meta.Proto,
		Headers:        headers,
		ReqMethod:      handle.Meta.RequestMethod,
		RequestHeaders: reqHeaders,
		ReqHost:        handle.Meta.RequestHost,
		ReqLen:         handle.Meta.RequestLength,
		ReqTE:          append([]string(nil), handle.Meta.RequestTE...),
		Body:           body,
		Duration:       acc.summary.Duration,
		EffectiveURL:   handle.Meta.EffectiveURL,
		Request:        handle.Meta.Request,
	}, nil
}

// Idle timer watches for activity resets - each incoming byte triggers a reset.
// The drain logic after Stop() handles the race where the timer fires just before we reset.
func runSSESession(session *stream.Session, body io.ReadCloser, opts restfile.SSEOptions) {
	ctx := session.Context()
	reader := bufio.NewReader(body)
	summary := SSESummary{Reason: "eof"}

	var (
		builder    sseEventBuilder
		index      int
		byteCount  int64
		eventCount int
	)

	idleReset := make(chan struct{}, 1)
	var idleTimer *time.Timer
	idleEnabled := opts.IdleTimeout > 0
	if idleEnabled {
		idleTimer = time.NewTimer(opts.IdleTimeout)
		go func() {
			defer idleTimer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-idleTimer.C:
					summary.Reason = "timeout:idle"
					session.Cancel()
					return
				case _, ok := <-idleReset:
					if !ok {
						return
					}
					if !idleTimer.Stop() {
						select {
						case <-idleTimer.C:
						default:
						}
					}
					idleTimer.Reset(opts.IdleTimeout)
				}
			}
		}()
	} else {
		close(idleReset)
	}

	defer func() {
		if idleEnabled {
			close(idleReset)
		}
	}()

	for {
		if opts.MaxBytes > 0 && byteCount >= opts.MaxBytes {
			if summary.Reason == "" || summary.Reason == "eof" {
				summary.Reason = "limit:max_bytes"
			}
			break
		}

		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			byteCount += int64(len(line))
			if idleEnabled {
				select {
				case idleReset <- struct{}{}:
				default:
				}
			}
		}

		limitReached := opts.MaxBytes > 0 && byteCount >= opts.MaxBytes

		if err != nil && !errors.Is(err, io.EOF) {
			session.Close(errdef.Wrap(errdef.CodeHTTP, err, "read sse stream"))
			return
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if evt, ok := builder.finalize(index); ok {
				publishSSEEvent(session, evt)
				index++
				eventCount++
				if opts.MaxEvents > 0 && eventCount >= opts.MaxEvents {
					summary.Reason = "limit:max_events"
					break
				}
			}
		} else {
			if err := builder.consume(trimmed); err != nil {
				session.Close(err)
				return
			}
		}

		if limitReached {
			if summary.Reason == "" || summary.Reason == "eof" {
				summary.Reason = "limit:max_bytes"
			}
			break
		}

		if errors.Is(err, io.EOF) {
			if evt, ok := builder.finalize(index); ok {
				publishSSEEvent(session, evt)
				eventCount++
			}
			break
		}

		if ctx.Err() != nil {
			if summary.Reason == "" || summary.Reason == "eof" {
				summary.Reason = "context_canceled"
			}
			break
		}
	}

	summary.EventCount = eventCount
	summary.ByteCount = byteCount

	metadata := map[string]string{
		sseMetaReason: summary.Reason,
		sseMetaBytes:  strconv.FormatInt(summary.ByteCount, 10),
		sseMetaEvents: strconv.Itoa(summary.EventCount),
	}
	session.Publish(&stream.Event{
		Kind:      stream.KindSSE,
		Direction: stream.DirNA,
		Timestamp: time.Now(),
		Metadata:  metadata,
	})

	var closeErr error
	if ctx.Err() != nil && summary.Reason == "context_canceled" {
		closeErr = ctx.Err()
	}
	session.Close(closeErr)
}

type sseAccumulator struct {
	events  []SSEEvent
	summary SSESummary
}

func newSSEAccumulator() *sseAccumulator {
	return &sseAccumulator{
		events:  make([]SSEEvent, 0, 16),
		summary: SSESummary{Reason: "eof"},
	}
}

func (a *sseAccumulator) consume(evt *stream.Event) {
	if evt == nil {
		return
	}
	switch evt.Direction {
	case stream.DirReceive:
		data := string(evt.Payload)
		item := SSEEvent{
			Index:     len(a.events),
			ID:        evt.SSE.ID,
			Event:     evt.SSE.Name,
			Data:      data,
			Comment:   evt.SSE.Comment,
			Retry:     evt.SSE.Retry,
			Timestamp: evt.Timestamp,
		}
		a.events = append(a.events, item)
	case stream.DirNA:
		if evt.Metadata == nil {
			return
		}
		if reason, ok := evt.Metadata[sseMetaReason]; ok {
			a.summary.Reason = reason
		}
		if bytesStr, ok := evt.Metadata[sseMetaBytes]; ok {
			if bytesParsed, err := strconv.ParseInt(bytesStr, 10, 64); err == nil {
				a.summary.ByteCount = bytesParsed
			}
		}
		if eventsStr, ok := evt.Metadata[sseMetaEvents]; ok {
			if count, err := strconv.Atoi(eventsStr); err == nil {
				a.summary.EventCount = count
			}
		}
	}
}

func publishSSEEvent(session *stream.Session, evt SSEEvent) {
	payload := []byte(evt.Data)
	metadata := make(map[string]string)
	if evt.Event != "" {
		metadata["sse.event"] = evt.Event
	}
	if evt.ID != "" {
		metadata["sse.id"] = evt.ID
	}
	if evt.Comment != "" {
		metadata["sse.comment"] = evt.Comment
	}
	if evt.Retry > 0 {
		metadata["sse.retry"] = strconv.Itoa(evt.Retry)
	}
	session.Publish(&stream.Event{
		Kind:      stream.KindSSE,
		Direction: stream.DirReceive,
		Timestamp: evt.Timestamp,
		Metadata:  metadata,
		Payload:   payload,
		SSE: stream.SSEMetadata{
			Name:    evt.Event,
			ID:      evt.ID,
			Comment: evt.Comment,
			Retry:   evt.Retry,
		},
	})
}

type sseEventBuilder struct {
	id       string
	event    string
	comment  []string
	data     []string
	retry    int
	hasRetry bool
}

func (b *sseEventBuilder) consume(line string) error {
	switch {
	case strings.HasPrefix(line, "data:"):
		b.data = append(b.data, strings.TrimLeft(line[5:], " \t"))
	case strings.HasPrefix(line, "event:"):
		b.event = strings.TrimLeft(line[6:], " \t")
	case strings.HasPrefix(line, "id:"):
		b.id = strings.TrimLeft(line[3:], " \t")
	case strings.HasPrefix(line, "retry:"):
		value := strings.TrimLeft(line[6:], " \t")
		if value == "" {
			b.retry = 0
			b.hasRetry = false
			return nil
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "parse retry directive")
		}
		if n < 0 {
			return errdef.New(errdef.CodeHTTP, "retry directive must be non-negative")
		}
		b.retry = n
		b.hasRetry = true
	case strings.HasPrefix(line, ":"):
		b.comment = append(b.comment, strings.TrimLeft(line[1:], " \t"))
	default:
		// Ignore unrecognised fields per SSE spec.
	}
	return nil
}

func (b *sseEventBuilder) finalize(index int) (SSEEvent, bool) {
	hasContent := len(b.data) > 0 || len(b.comment) > 0 || b.event != "" || b.id != "" || b.hasRetry
	if !hasContent {
		return SSEEvent{}, false
	}
	evt := SSEEvent{
		Index:     index,
		ID:        b.id,
		Event:     b.event,
		Data:      strings.Join(b.data, "\n"),
		Comment:   strings.Join(b.comment, "\n"),
		Timestamp: time.Now(),
	}
	if b.hasRetry {
		evt.Retry = b.retry
	}
	*b = sseEventBuilder{}
	return evt, true
}
