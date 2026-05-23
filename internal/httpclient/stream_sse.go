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
)

const (
	sseReasonEOF       = "eof"
	sseReasonErr       = "error"
	sseReasonIdle      = "timeout:idle"
	sseReasonMaxBytes  = "limit:max_bytes"
	sseReasonMaxEvents = "limit:max_events"
	sseReasonCanceled  = "context_canceled"
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
		body, readErr := io.ReadAll(httpResp.Body)
		closeErr := httpResp.Body.Close()
		cancel()
		if readErr != nil {
			return nil, nil, diag.WrapAs(diag.ClassProtocol, readErr, "read response body")
		}
		if closeErr != nil {
			return nil, nil, diag.WrapAs(diag.ClassProtocol, closeErr, "close response body")
		}
		return nil, respFromHTTP(httpReq, httpResp, req, body, time.Since(start)), nil
	}

	meta := buildStreamMeta(req, httpReq, httpResp, effectiveOpts.BaseDir, metaDefaults{})

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
		return nil, diag.New(diag.ClassProtocol, "sse session not available")
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
	state, serr := session.State()
	if acc.summary.Reason == "" {
		if serr != nil {
			acc.summary.Reason = serr.Error()
		} else if state == stream.StateFailed {
			acc.summary.Reason = sseReasonErr
		} else {
			acc.summary.Reason = sseReasonEOF
		}
	} else if acc.summary.Reason == sseReasonEOF && (state == stream.StateFailed || serr != nil) {
		if serr != nil {
			acc.summary.Reason = serr.Error()
		} else {
			acc.summary.Reason = sseReasonErr
		}
	}

	transcript := SSETranscript{Events: acc.events, Summary: acc.summary}
	body, err := json.MarshalIndent(transcript, "", "  ")
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "encode sse transcript")
	}

	headers := cloneHdr(handle.Meta.Headers)
	if headers == nil {
		headers = make(http.Header)
	}
	headers.Set("Content-Type", streamContentTypeJSON)
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

	return streamResp(handle.Meta, headers, body, acc.summary.Duration), nil
}

// Idle timer watches for activity resets - each incoming byte triggers a reset.
// The drain logic after Stop() handles the race where the timer fires just before we reset.
func runSSESession(session *stream.Session, body io.ReadCloser, opts restfile.SSEOptions) {
	ctx := session.Context()
	reader := bufio.NewReader(body)
	summary := SSESummary{Reason: sseReasonEOF}

	var (
		builder    sseEventBuilder
		index      int
		byteCount  int64
		eventCount int
	)

	idleReset, stopIdle := startIdleWatch(ctx, opts.IdleTimeout, func() {
		summary.Reason = sseReasonIdle
		session.Cancel()
	})
	defer stopIdle()

	for {
		if opts.MaxBytes > 0 && byteCount >= opts.MaxBytes {
			if summary.Reason == "" || summary.Reason == sseReasonEOF {
				summary.Reason = sseReasonMaxBytes
			}
			break
		}

		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			byteCount += int64(len(line))
			if idleReset != nil {
				select {
				case idleReset <- struct{}{}:
				default:
				}
			}
		}

		limitReached := opts.MaxBytes > 0 && byteCount >= opts.MaxBytes

		if err != nil && !errors.Is(err, io.EOF) {
			session.Close(diag.WrapAs(diag.ClassProtocol, err, "read sse stream"))
			return
		}

		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if evt, ok := builder.finalize(index); ok {
				publishSSEEvent(session, evt)
				index++
				eventCount++
				if opts.MaxEvents > 0 && eventCount >= opts.MaxEvents {
					summary.Reason = sseReasonMaxEvents
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
			if summary.Reason == "" || summary.Reason == sseReasonEOF {
				summary.Reason = sseReasonMaxBytes
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
			if summary.Reason == "" || summary.Reason == sseReasonEOF {
				summary.Reason = sseReasonCanceled
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
	if ctx.Err() != nil && summary.Reason == sseReasonCanceled {
		closeErr = ctx.Err()
	}
	session.Close(closeErr)
}
