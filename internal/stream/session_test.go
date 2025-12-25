package stream

import (
	"context"
	"testing"
	"time"
)

func TestSessionPublishAndSubscribe(t *testing.T) {
	s := NewSession(context.Background(), KindSSE, Config{BufferSize: 4, ListenerBuffer: 2})
	s.MarkOpen()
	listener := s.Subscribe()

	evt := &Event{Kind: KindSSE, Direction: DirReceive, Payload: []byte("hello")}
	s.Publish(evt)

	select {
	case received := <-listener.C:
		if string(received.Payload) != "hello" {
			t.Fatalf("expected payload hello, got %q", string(received.Payload))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	s.Close(nil)
	listener.Cancel()
	select {
	case _, ok := <-listener.C:
		if ok {
			t.Fatalf("expected listener channel to be closed")
		}
	default:
	}
}

func TestSessionDropNewestPolicy(t *testing.T) {
	s := NewSession(
		context.Background(),
		KindSSE,
		Config{ListenerBuffer: 1, DropPolicy: DropNewest},
	)
	s.MarkOpen()
	listener := s.Subscribe()

	s.Publish(&Event{Kind: KindSSE, Direction: DirReceive, Payload: []byte("first")})
	s.Publish(&Event{Kind: KindSSE, Direction: DirReceive, Payload: []byte("second")})

	select {
	case evt := <-listener.C:
		if string(evt.Payload) != "first" {
			t.Fatalf("expected first event, got %q", evt.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first event")
	}

	select {
	case evt := <-listener.C:
		t.Fatalf("unexpected event %q", evt.Payload)
	default:
	}

	stats := s.StatsSnapshot()
	if stats.Dropped == 0 {
		t.Fatalf("expected dropped counter to increase")
	}

	s.Close(nil)
	listener.Cancel()
}
