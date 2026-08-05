package ui

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/stream"
)

// liveSession is the UI's copy of one stream: the events it has seen plus the
// view state (filter, pause, bookmarks) the user applied to them.
//
// bound and done drive its lifetime in Model.liveSessions. The registry is what
// a stream that has no response yet is looked up through, so an entry can only
// be dropped once a snapshot holds the session (bound) and no more events are
// coming (done). Either order works: a stream that ends before its response
// arrives stays in the registry until that response adopts it.
type liveSession struct {
	id          string
	events      []*stream.Event
	maxEvents   int
	state       stream.State
	err         error
	kind        stream.Kind
	filter      string
	paused      bool
	pausedIndex int
	bookmarks   []streamBookmark
	bookmarkIdx int
	bound       bool
	done        bool
}

func (ls *liveSession) failed() bool {
	return ls != nil && (ls.err != nil || ls.state == stream.StateFailed)
}

func newLiveSession(id string, max int) *liveSession {
	if max <= 0 {
		max = 5000
	}
	return &liveSession{id: id, maxEvents: max, pausedIndex: -1, bookmarkIdx: -1}
}

func (ls *liveSession) append(events []*stream.Event) {
	if len(events) == 0 {
		return
	}
	ls.events = append(ls.events, cloneEventSlice(events)...)
	if len(ls.events) > ls.maxEvents {
		trim := len(ls.events) - ls.maxEvents
		ls.events = append([]*stream.Event(nil), ls.events[trim:]...)
		if ls.paused && ls.pausedIndex >= 0 {
			ls.pausedIndex -= trim
			if ls.pausedIndex < 0 {
				ls.pausedIndex = 0
			}
			if ls.pausedIndex > len(ls.events) {
				ls.pausedIndex = len(ls.events)
			}
		}
		if len(ls.bookmarks) > 0 {
			filtered := ls.bookmarks[:0]
			for _, bm := range ls.bookmarks {
				idx := bm.Index - trim
				if idx < 0 {
					continue
				}
				if idx > len(ls.events) {
					idx = len(ls.events)
				}
				bm.Index = idx
				filtered = append(filtered, bm)
			}
			ls.bookmarks = filtered
			if len(ls.bookmarks) == 0 {
				ls.bookmarkIdx = -1
			} else if ls.bookmarkIdx >= len(ls.bookmarks) {
				ls.bookmarkIdx = len(ls.bookmarks) - 1
			}
		}
	}
	if ls.paused && ls.pausedIndex == -1 {
		ls.pausedIndex = len(ls.events)
	}
}

// reset empties the buffer and every view applied to it. The session itself
// keeps running, so new events land in a clean transcript.
func (ls *liveSession) reset() {
	if ls == nil {
		return
	}
	ls.events = nil
	ls.filter = ""
	ls.paused = false
	ls.pausedIndex = -1
	ls.bookmarks = nil
	ls.bookmarkIdx = -1
}

func (ls *liveSession) setState(state stream.State, err error) {
	ls.state = state
	ls.err = err
}

func (ls *liveSession) setPaused(paused bool) {
	ls.paused = paused
	if paused {
		ls.pausedIndex = len(ls.events)
	} else {
		ls.pausedIndex = -1
	}
}

func (ls *liveSession) addBookmark(label string) {
	idx := len(ls.events)
	if ls.paused && ls.pausedIndex >= 0 {
		idx = ls.pausedIndex
	}
	ls.bookmarks = append(
		ls.bookmarks,
		streamBookmark{Index: idx, Label: strings.TrimSpace(label), Created: time.Now()},
	)
	ls.bookmarkIdx = len(ls.bookmarks) - 1
}

func (ls *liveSession) bookmark(offset int) *streamBookmark {
	if offset < 0 || offset >= len(ls.bookmarks) {
		return nil
	}
	return &ls.bookmarks[offset]
}

func (ls *liveSession) nextBookmark(forward bool) *streamBookmark {
	if len(ls.bookmarks) == 0 {
		return nil
	}
	if forward {
		ls.bookmarkIdx++
		if ls.bookmarkIdx >= len(ls.bookmarks) {
			ls.bookmarkIdx = 0
		}
	} else {
		ls.bookmarkIdx--
		if ls.bookmarkIdx < 0 {
			ls.bookmarkIdx = len(ls.bookmarks) - 1
		}
	}
	return ls.bookmark(ls.bookmarkIdx)
}

type streamBookmark struct {
	Index   int
	Label   string
	Created time.Time
}

func cloneEventSlice(events []*stream.Event) []*stream.Event {
	if len(events) == 0 {
		return nil
	}
	out := make([]*stream.Event, len(events))
	copy(out, events)
	return out
}
