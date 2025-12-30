package stream

import (
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
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

	Metadata map[string]string
	Payload  []byte

	SSE SSEMetadata
	WS  WSMetadata
}

type SSEMetadata struct {
	Name    string
	ID      string
	Comment string
	Retry   int
}

type WSMetadata struct {
	Opcode int
	Code   websocket.StatusCode
	Reason string
}

var seqCounter uint64

func nextSequence() uint64 {
	return atomic.AddUint64(&seqCounter, 1)
}
