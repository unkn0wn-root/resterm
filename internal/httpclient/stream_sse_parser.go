package httpclient

import (
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

type sseAccumulator struct {
	events  []SSEEvent
	summary SSESummary
}

func newSSEAccumulator() *sseAccumulator {
	return &sseAccumulator{
		events:  make([]SSEEvent, 0, 16),
		summary: SSESummary{},
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
			return diag.WrapAs(diag.ClassProtocol, err, "parse retry directive")
		}
		if n < 0 {
			return diag.New(diag.ClassProtocol, "retry directive must be non-negative")
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
	if !b.hasContent() {
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

func (b *sseEventBuilder) hasContent() bool {
	if len(b.data) > 0 {
		return true
	}
	if len(b.comment) > 0 {
		return true
	}
	if b.event != "" {
		return true
	}
	if b.id != "" {
		return true
	}
	return b.hasRetry
}
