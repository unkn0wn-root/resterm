package httpx

import (
	"strconv"

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
