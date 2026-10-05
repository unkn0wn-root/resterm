package httpx

import (
	"strconv"
	"strings"
	"time"

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
			Index:     evt.SSE.Index,
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
		if failure, ok := evt.Metadata[sseMetaError]; ok {
			a.summary.Error = failure
		}
	}
}

func publishSSEEvent(session *stream.Session, evt SSEEvent) {
	session.Publish(&stream.Event{
		Kind:      stream.KindSSE,
		Direction: stream.DirReceive,
		Timestamp: evt.Timestamp,
		Payload:   []byte(evt.Data),
		SSE: stream.SSEMetadata{
			Index:   evt.Index,
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

// A line without a colon is a field with an empty value, and unknown fields
// are ignored, as the SSE spec says.
func (b *sseEventBuilder) consume(line string) {
	name, value, _ := strings.Cut(line, ":")
	value = strings.TrimPrefix(value, " ")
	switch name {
	case "": // a leading colon makes a comment
		b.comment = append(b.comment, value)
	case "data":
		b.data = append(b.data, value)
	case "event":
		b.event = value
	case "id":
		if !strings.Contains(value, "\x00") {
			b.id = value
		}
	case "retry":
		// ParseUint rejects a sign, so only ASCII digits pass, and 31 bits fit an int everywhere.
		if n, err := strconv.ParseUint(value, 10, 31); err == nil {
			b.retry, b.hasRetry = int(n), true
		}
	}
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
