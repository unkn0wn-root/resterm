package ui

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/stream"
)

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

func (ls *liveSession) visibleEvents() []*stream.Event {
	if !ls.paused || ls.pausedIndex < 0 || ls.pausedIndex > len(ls.events) {
		return ls.events
	}
	return ls.events[:ls.pausedIndex]
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
