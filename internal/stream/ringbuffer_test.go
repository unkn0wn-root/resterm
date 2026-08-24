package stream

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
)

func payload(n int) *Event {
	return &Event{Payload: make([]byte, n)}
}

func TestRingBufferEvictsToStayWithinItsByteBudget(t *testing.T) {
	r := newRingBuffer(16, 100)
	for range 8 {
		r.append(payload(30))
	}

	if got := r.count; got != 3 {
		t.Fatalf("count = %d, want 3", got)
	}
	if r.bytes > 100 {
		t.Fatalf("bytes = %d, want at most 100", r.bytes)
	}
	if r.evicted != 5 {
		t.Fatalf("evicted = %d, want 5", r.evicted)
	}
}

func TestRingBufferKeepsAnEventLargerThanTheBudget(t *testing.T) {
	r := newRingBuffer(16, 100)
	r.append(payload(50))
	r.append(payload(4096))

	if got := r.count; got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}
	if got := len(r.snapshot()[0].Payload); got != 4096 {
		t.Fatalf("kept an event of %d bytes, want the newest", got)
	}
}

func TestRingBufferWithoutAByteBudgetKeepsEverythingItHolds(t *testing.T) {
	r := newRingBuffer(4, 0)
	for range 4 {
		r.append(payload(1 << 20))
	}
	if got := r.count; got != 4 {
		t.Fatalf("count = %d, want 4", got)
	}
}

func TestRingBufferStillEvictsOnCount(t *testing.T) {
	r := newRingBuffer(2, 0)
	for i := range 5 {
		r.append(&Event{Payload: []byte{byte(i)}})
	}
	snap := r.snapshot()
	if len(snap) != 2 || snap[0].Payload[0] != 3 || snap[1].Payload[0] != 4 {
		t.Fatalf("snapshot = %v, want the last two events", snap)
	}
}

func TestSessionReportsEvictedEvents(t *testing.T) {
	s := NewSession(context.Background(), KindSSE, Config{BufferSize: 8, MaxBytes: bytesize.Of(100)})
	for range 8 {
		s.Publish(payload(40))
	}
	if got := s.StatsSnapshot().Evicted; got == 0 {
		t.Fatal("Evicted = 0, want the discarded events to be counted")
	}
}

func TestSessionByteBudgetDefaultsAndOptsOut(t *testing.T) {
	tests := []struct {
		name string
		raw  bytesize.Budget
		want int64
	}{
		{name: "unset takes the default", want: DefaultMaxBytes},
		{name: "explicit budget", raw: bytesize.Of(4096), want: 4096},
		{name: "an unlimited budget removes it", raw: bytesize.Unlimited(), want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.raw.Or(DefaultMaxBytes); got != tt.want {
				t.Fatalf("MaxBytes = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRingBufferKeepsAnOversizedEventBehindAWeightlessOne(t *testing.T) {
	r := newRingBuffer(16, 100)
	r.append(payload(4096))
	r.append(&Event{Kind: KindSSE})

	snap := r.snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot holds %d events, want both", len(snap))
	}
	if got := len(snap[0].Payload); got != 4096 {
		t.Fatalf("the oversized event kept %d bytes, want 4096", got)
	}
	if r.evicted != 0 {
		t.Fatalf("evicted = %d, want a payload-free event to evict nothing", r.evicted)
	}
}

func TestRingBufferCountsEventsThatCarryNoPayload(t *testing.T) {
	comment := func(n int) *Event {
		return &Event{Kind: KindSSE, SSE: SSEMetadata{Comment: strings.Repeat("x", n)}}
	}

	r := newRingBuffer(64, 100)
	for range 8 {
		r.append(comment(30))
	}

	if r.count != 3 {
		t.Fatalf("count = %d, want the buffer to evict down to its budget", r.count)
	}
	if r.bytes > 100 {
		t.Fatalf("bytes = %d, want at most 100", r.bytes)
	}
	if r.evicted != 5 {
		t.Fatalf("evicted = %d, want 5", r.evicted)
	}
}

func TestEventSizeCountsEveryRetainedField(t *testing.T) {
	evt := &Event{
		Payload:  []byte("ab"),
		Metadata: map[string]string{"k": "vv"},
		SSE:      SSEMetadata{Name: "nnn", ID: "i", Comment: "cccc"},
		WS:       WSMetadata{Reason: "rr"},
	}
	if got := evt.Size(); got != 15 {
		t.Fatalf("Size() = %d, want 15", got)
	}
	if got := (*Event)(nil).Size(); got != 0 {
		t.Fatalf("nil Size() = %d, want 0", got)
	}
}
