package httpx

import (
	"bufio"
	"bytes"
	"errors"
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

var errSSELineTooLong = errors.New("sse line exceeds the line limit")

type sseReader struct {
	br      *bufio.Reader
	cr      bool
	full    bool
	started bool
}

// readLine returns the next line without its end, which is CRLF, LF or CR.
// n counts every byte the call read: the line, its end, a BOM before the first
// line, and the LF of a CRLF whose CR ended the previous line. It never reads
// past a CR, so a stream that ends lines with CR alone does not wait for more
// data, and the result does not depend on how the bytes arrive. A line that
// needs more than limit bytes, its end included, returns its first limit bytes
// with errSSELineTooLong. The LF of a CRLF counts toward the line it ends, so
// when the CR already used the whole limit, that LF is reported as
// errSSELineTooLong on the next call without being read.
func (r *sseReader) readLine(limit int) (string, int, error) {
	var (
		line strings.Builder
		n    int
		size int
		err  error
	)
	for {
		if r.br.Buffered() == 0 {
			if _, err = r.br.Peek(1); err != nil {
				break
			}
		}
		buf, _ := r.br.Peek(r.br.Buffered())
		if r.cr && buf[0] == '\n' {
			if r.full {
				return "", 0, errSSELineTooLong
			}
			_, _ = r.br.Discard(1)
			n++
			buf = buf[1:]
		}
		r.cr = false
		if len(buf) == 0 {
			continue
		}
		end := bytes.IndexByte(buf, '\n')
		if end < 0 {
			end = len(buf)
		}
		if i := bytes.IndexByte(buf[:end], '\r'); i >= 0 {
			end = i
		}
		found := end < len(buf)
		take := min(end+1, len(buf))
		if size+take > limit {
			take, err = limit-size, errSSELineTooLong
		}
		line.Write(buf[:min(end, take)])
		r.cr = found && err == nil && buf[end] == '\r'
		_, _ = r.br.Discard(take)
		n += take
		size += take
		if found || err != nil {
			break
		}
	}
	r.full = r.cr && size == limit
	s := line.String()
	if !r.started {
		r.started = true
		s = strings.TrimPrefix(s, "\ufeff")
	}
	return s, n, err
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
