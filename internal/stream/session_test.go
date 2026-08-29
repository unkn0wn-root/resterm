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

func TestSessionDropOldestPolicyReportsLoss(t *testing.T) {
	s := NewSession(
		t.Context(),
		KindSSE,
		Config{ListenerBuffer: 1, DropPolicy: DropOldest},
	)
	s.MarkOpen()
	listener := s.Subscribe()
	t.Cleanup(listener.Cancel)

	s.Publish(&Event{Kind: KindSSE, Direction: DirReceive, Payload: []byte("first")})
	s.Publish(&Event{Kind: KindSSE, Direction: DirReceive, Payload: []byte("second")})

	select {
	case evt := <-listener.C:
		if string(evt.Payload) != "second" {
			t.Fatalf("received %q, want the newest event", evt.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the newest event")
	}
	if got := listener.Dropped(); got != 1 {
		t.Fatalf("listener dropped %d events, want 1", got)
	}
	if got := s.StatsSnapshot().Dropped; got != 1 {
		t.Fatalf("session dropped %d events, want 1", got)
	}
}

func TestSessionSnapshotReportsEvictedEvents(t *testing.T) {
	s := NewSession(t.Context(), KindWebSocket, Config{BufferSize: 1})
	s.Publish(&Event{Kind: KindWebSocket, Payload: []byte("first")})
	s.Publish(&Event{Kind: KindWebSocket, Payload: []byte("second")})

	listener := s.Subscribe()
	t.Cleanup(listener.Cancel)
	if got := listener.Snapshot.Evicted; got != 1 {
		t.Fatalf("snapshot evicted %d events, want 1", got)
	}
}

func TestSessionSubscribeAfterClose(t *testing.T) {
	s := NewSession(context.Background(), KindGRPC, Config{})
	s.Publish(&Event{Kind: KindGRPC, Direction: DirReceive, Payload: []byte("last")})
	s.Close(context.DeadlineExceeded)

	listener := s.Subscribe()
	if got := len(listener.Snapshot.Events); got != 1 {
		t.Fatalf("expected one snapshot event, got %d", got)
	}
	if listener.Snapshot.State != StateFailed {
		t.Fatalf("expected failed snapshot, got %v", listener.Snapshot.State)
	}
	if listener.Snapshot.Err != context.DeadlineExceeded {
		t.Fatalf("expected deadline error, got %v", listener.Snapshot.Err)
	}
	select {
	case _, ok := <-listener.C:
		if ok {
			t.Fatal("expected closed listener channel")
		}
	default:
		t.Fatal("expected listener channel to be closed immediately")
	}
	listener.Cancel()
}
