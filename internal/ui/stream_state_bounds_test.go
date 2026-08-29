package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/stream"
)

func streamEvents(n, size int) []*stream.Event {
	out := make([]*stream.Event, n)
	for i := range out {
		out[i] = &stream.Event{Payload: make([]byte, size)}
	}
	return out
}

func TestLiveSessionTrimsOnItsByteBudget(t *testing.T) {
	ls := newLiveSession("s", 5000)
	ls.maxBytes = 4096

	ls.append(streamEvents(64, 1024))

	if len(ls.events) > 4 {
		t.Fatalf("kept %d events, want the byte budget to have trimmed them", len(ls.events))
	}
	if ls.bytes > ls.maxBytes {
		t.Fatalf("bytes = %d, want at most %d", ls.bytes, ls.maxBytes)
	}
}

func TestLiveSessionKeepsTheNewestEventWhateverItsSize(t *testing.T) {
	ls := newLiveSession("s", 5000)
	ls.maxBytes = 64

	ls.append(streamEvents(1, 4096))

	if len(ls.events) != 1 {
		t.Fatalf("kept %d events, want the newest one", len(ls.events))
	}
}

func TestLiveSessionStillTrimsOnItsEventCount(t *testing.T) {
	ls := newLiveSession("s", 10)

	ls.append(streamEvents(64, 8))

	if len(ls.events) != 10 {
		t.Fatalf("kept %d events, want 10", len(ls.events))
	}
}

func TestLiveSessionTrimMovesBookmarksAndThePausedPosition(t *testing.T) {
	ls := newLiveSession("s", 4)

	ls.append(streamEvents(4, 8))
	ls.addBookmark("first")
	ls.setPaused(true)
	ls.append(streamEvents(4, 8))

	if ls.pausedIndex < 0 || ls.pausedIndex > len(ls.events) {
		t.Fatalf("pausedIndex = %d, outside a buffer of %d", ls.pausedIndex, len(ls.events))
	}
	for _, bm := range ls.bookmarks {
		if bm.Index < 0 || bm.Index > len(ls.events) {
			t.Fatalf("bookmark index = %d, outside a buffer of %d", bm.Index, len(ls.events))
		}
	}
}

func TestLiveSessionResetClearsItsByteCount(t *testing.T) {
	ls := newLiveSession("s", 5000)
	ls.append(streamEvents(4, 1024))
	ls.reset()

	if ls.bytes != 0 {
		t.Fatalf("bytes = %d after reset, want 0", ls.bytes)
	}
}

func TestLiveSessionTrimsEventsThatCarryOnlyAComment(t *testing.T) {
	comments := make([]*stream.Event, 64)
	for i := range comments {
		comments[i] = &stream.Event{
			Kind: stream.KindSSE,
			SSE:  stream.SSEMetadata{Comment: strings.Repeat("x", 1024)},
		}
	}

	ls := newLiveSession("s", 5000)
	ls.maxBytes = 4096
	ls.append(comments)

	if len(ls.events) > 4 {
		t.Fatalf("kept %d events, want the byte budget to have trimmed them", len(ls.events))
	}
	if ls.bytes > 4096 {
		t.Fatalf("bytes = %d, want at most the 4096 byte budget", ls.bytes)
	}
}
