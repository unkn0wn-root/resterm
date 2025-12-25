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
	if len(ls.visibleEvents()) != 1 {
		t.Fatalf("expected visible events while running")
	}
	ls.setPaused(true)
	if !ls.paused {
		t.Fatalf("expected paused flag to set")
	}
	if len(ls.visibleEvents()) != 1 {
		t.Fatalf("expected existing events visible when paused")
	}
	ls.append(
		[]*stream.Event{
			{Kind: stream.KindSSE, Direction: stream.DirReceive, Payload: []byte("two")},
		},
	)
	if len(ls.visibleEvents()) != 1 {
		t.Fatalf("expected new events hidden while paused")
	}
	ls.setPaused(false)
	if len(ls.visibleEvents()) != 2 {
		t.Fatalf("expected all events visible after resume")
	}
}

func TestBookmarkLabelFallback(t *testing.T) {
	bm := streamBookmark{Label: "", Created: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
	label := bookmarkLabel(bm)
	if label == "" {
		t.Fatalf("expected fallback label")
	}
}
