package history

import (
	"strconv"
	"sync/atomic"
	"time"
)

var lastID atomic.Int64

// NewID returns an entry ID that is unique within this process and never below the last one,
// even when the clock only moves in microseconds.
// Two processes sharing a history file can still get the same ID if they ask at the same instant.
func NewID() string {
	now := time.Now().UnixNano()
	for {
		last := lastID.Load()
		next := max(now, last+1)
		if lastID.CompareAndSwap(last, next) {
			return strconv.FormatInt(next, 10)
		}
	}
}
