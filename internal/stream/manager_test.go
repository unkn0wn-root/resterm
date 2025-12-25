package stream

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestManagerRegisterAndList(t *testing.T) {
	mgr := NewManager()
	session := NewSession(context.Background(), KindSSE, Config{BufferSize: 8})
	session.MarkOpen()

	summary := mgr.Register(session)
	if summary.ID == "" {
		t.Fatal("expected session ID")
	}

	listed := mgr.List()
	if len(listed) != 1 {
		t.Fatalf("expected 1 session, got %d", len(listed))
	}
	if listed[0].ID != summary.ID {
		t.Fatalf("unexpected session ID %s", listed[0].ID)
	}

	if !mgr.Cancel(summary.ID) {
		t.Fatalf("expected cancel to succeed")
	}
}

func TestManagerCompletionHook(t *testing.T) {
	mgr := NewManager()
	session := NewSession(context.Background(), KindSSE, Config{BufferSize: 8})
	session.MarkOpen()

	summary := mgr.Register(session)

	var (
		wg              sync.WaitGroup
		hookCalled      bool
		receivedSummary SessionSummary
		receivedEvents  []*Event
	)
	wg.Add(1)

	mgr.AddCompletionHook(summary.ID, func(sum SessionSummary, events []*Event) {
		defer wg.Done()
		hookCalled = true
		receivedSummary = sum
		receivedEvents = events
	})

	event := &Event{
		Kind:      KindSSE,
		Direction: DirReceive,
		Payload:   []byte("test"),
		Timestamp: time.Now(),
	}
	session.Publish(event)
	session.Close(nil)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("completion hook did not fire")
	}

	if !hookCalled {
		t.Fatal("expected completion hook to be called")
	}
	if receivedSummary.State != StateClosed {
		t.Fatalf("expected closed state, got %v", receivedSummary.State)
	}
	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receivedEvents))
	}
}
