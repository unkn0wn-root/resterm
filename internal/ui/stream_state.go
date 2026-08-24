package ui

import (
	"slices"
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
	bytes       int64
	maxEvents   int
	maxBytes    int64
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

const defaultLiveSessionBytes = 16 << 20

func newLiveSession(id string, max int) *liveSession {
	if max <= 0 {
		max = 5000
	}
	return &liveSession{
		id:          id,
		maxEvents:   max,
		maxBytes:    defaultLiveSessionBytes,
		pausedIndex: -1,
		bookmarkIdx: -1,
	}
}

func (ls *liveSession) append(events []*stream.Event) {
	if len(events) == 0 {
		return
	}
	added := cloneEventSlice(events)
	ls.events = append(ls.events, added...)
	for _, evt := range added {
		ls.bytes += evt.Size()
	}
	ls.trimOverflow()
	if ls.paused && ls.pausedIndex == -1 {
		ls.pausedIndex = len(ls.events)
	}
}

func (ls *liveSession) trimOverflow() {
	trim := 0
	bytes := ls.bytes
	for trim < len(ls.events)-1 {
		if len(ls.events)-trim <= ls.maxEvents && (ls.maxBytes <= 0 || bytes <= ls.maxBytes) {
			break
		}
		bytes -= ls.events[trim].Size()
		trim++
	}
	if trim == 0 {
		return
	}

	ls.events = slices.Clone(ls.events[trim:])
	ls.bytes = bytes
	if ls.paused && ls.pausedIndex >= 0 {
		ls.pausedIndex = min(max(ls.pausedIndex-trim, 0), len(ls.events))
	}
	if len(ls.bookmarks) == 0 {
		return
	}

	filtered := ls.bookmarks[:0]
	for _, bm := range ls.bookmarks {
		idx := bm.Index - trim
		if idx < 0 {
			continue
		}
		bm.Index = min(idx, len(ls.events))
		filtered = append(filtered, bm)
	}
	ls.bookmarks = filtered
	if len(ls.bookmarks) == 0 {
		ls.bookmarkIdx = -1
	} else if ls.bookmarkIdx >= len(ls.bookmarks) {
		ls.bookmarkIdx = len(ls.bookmarks) - 1
	}
}

// reset empties the buffer and every view applied to it. The session itself
// keeps running, so new events land in a clean transcript.
func (ls *liveSession) reset() {
	if ls == nil {
		return
	}
	ls.events = nil
	ls.bytes = 0
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
