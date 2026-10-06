package stream

import (
	"sync/atomic"
	"time"
)

type Kind int

const (
	KindSSE Kind = iota
	KindWebSocket
	KindGRPC
)

type Direction int

const (
	DirNA Direction = iota
	DirSend
	DirReceive
)

type State int

const (
	StateConnecting State = iota
	StateOpen
	StateClosing
	StateClosed
	StateFailed
)

type Event struct {
	Kind      Kind
	Direction Direction
	Timestamp time.Time
	Sequence  uint64

	// Metadata carries SSE and gRPC details. WebSocket events use WS.
	Metadata map[string]string
	Payload  []byte

	SSE SSEMetadata
	WS  WSMetadata
}

func (e *Event) Size() int64 {
	if e == nil {
		return 0
	}
	size := len(e.Payload) + e.SSE.size() + e.WS.size()
	for key, value := range e.Metadata {
		size += len(key) + len(value)
	}
	return int64(size)
}

type SSEMetadata struct {
	Index   int
	Name    string
	ID      string
	Comment string
	Retry   int
}

func (m SSEMetadata) size() int {
	return len(m.Name) + len(m.ID) + len(m.Comment)
}

var seqCounter uint64

func nextSequence() uint64 {
	return atomic.AddUint64(&seqCounter, 1)
}
