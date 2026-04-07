package ui

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/stream"
)

func TestMatchesFilterSSE(t *testing.T) {
	evt := &stream.Event{
		Kind:      stream.KindSSE,
		Direction: stream.DirReceive,
		Payload:   []byte("hello world"),
		SSE: stream.SSEMetadata{
			Name:    "greeting",
			Comment: "friendly",
		},
	}
	if !matchesFilter("hello", evt) {
		t.Fatalf("expected filter to match payload")
	}
	if !matchesFilter("greet", evt) {
		t.Fatalf("expected filter to match event name")
	}
	if matchesFilter("bye", evt) {
		t.Fatalf("did not expect filter to match")
	}
}

func TestLiveSessionPause(t *testing.T) {
	ls := newLiveSession("s", 10)
	evt := &stream.Event{Kind: stream.KindSSE, Direction: stream.DirReceive, Payload: []byte("one")}
	ls.append([]*stream.Event{evt})
	if len(ls.events) != 1 {
		t.Fatalf("expected one event while running")
	}
	ls.setPaused(true)
	if !ls.paused {
		t.Fatalf("expected paused flag to set")
	}
	if ls.pausedIndex != 1 {
		t.Fatalf("expected paused index to capture current position, got %d", ls.pausedIndex)
	}
	ls.append(
		[]*stream.Event{
			{Kind: stream.KindSSE, Direction: stream.DirReceive, Payload: []byte("two")},
		},
	)
	if len(ls.events) != 2 {
		t.Fatalf("expected buffered events to grow while paused")
	}
	if ls.pausedIndex != 1 {
		t.Fatalf("expected pause boundary to stay fixed while paused, got %d", ls.pausedIndex)
	}
	ls.setPaused(false)
	if ls.pausedIndex != -1 {
		t.Fatalf("expected paused index reset after resume, got %d", ls.pausedIndex)
	}
	if len(ls.events) != 2 {
		t.Fatalf("expected all events available after resume")
	}
}

func TestBookmarkLabelFallback(t *testing.T) {
	bm := streamBookmark{Label: "", Created: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
	label := bookmarkLabel(bm)
	if label == "" {
		t.Fatalf("expected fallback label")
	}
}
