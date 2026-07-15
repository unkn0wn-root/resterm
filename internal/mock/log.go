package mock

import "sync"

// ring is a fixed-capacity event log: fill to limit, then overwrite oldest.
type ring struct {
	mu     sync.RWMutex
	events []Event
	head   int
	limit  int
}

func (r *ring) add(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) < r.limit {
		r.events = append(r.events, e)
		return
	}
	r.events[r.head] = e
	r.head = (r.head + 1) % r.limit
}

func (r *ring) list() []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Event, 0, len(r.events))
	out = append(out, r.events[r.head:]...)
	return append(out, r.events[:r.head]...)
}

func (r *ring) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.events)
	r.events = r.events[:0]
	r.head = 0
}
