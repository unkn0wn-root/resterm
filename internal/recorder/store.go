package recorder

import (
	"cmp"
	"net/http"
	"sync"
)

// mu guards all fields below it.
type store struct {
	cfg     Config
	updates chan struct{}

	// Add under mu so shutdown cannot start waiting while new captures begin.
	captures sync.WaitGroup

	mu       sync.Mutex
	entries  []Entry
	received uint64
	excluded uint64
	retained int64
	buffered int64
	active   int
	limit    Limit
	running  bool
}

func newStore(cfg Config) *store {
	return &store{cfg: cfg, updates: make(chan struct{}, 1), running: true}
}

func (s *store) signal() {
	select {
	case s.updates <- struct{}{}:
	default:
	}
}

func (s *store) stop() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	s.signal()
}

func (s *store) drain() { s.captures.Wait() }

func (s *store) captureDone() { s.captures.Done() }

func (s *store) stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{
		Received:       s.received,
		Entries:        len(s.entries),
		Excluded:       s.excluded,
		RetainedBytes:  s.retained,
		ActiveCaptures: s.active,
		Limit:          s.limit,
		Running:        s.running,
	}
}

// IDs start at 1 in append order, so after is also the next slice index.
func (s *store) snapshot(after uint64) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if after >= uint64(len(s.entries)) {
		return nil
	}
	rest := s.entries[after:]
	entries := make([]Entry, len(rest))
	for i, e := range rest {
		entries[i] = e.clone()
	}
	return entries
}

func (s *store) summaries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := make([]Entry, len(s.entries))
	for i, e := range s.entries {
		entries[i] = e.summary()
	}
	return entries
}

func (s *store) reserve(n int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > s.cfg.MaxBytes-s.buffered {
		return false
	}
	s.buffered += n
	return true
}

func (s *store) release(n int64) {
	s.mu.Lock()
	s.buffered -= n
	s.mu.Unlock()
}

func (s *store) admit(r *http.Request) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.received++
	if !s.admits(r) {
		s.excluded++
		s.signal()
		return false
	}
	s.active++
	s.captures.Add(1)
	return true
}

func (s *store) admits(r *http.Request) bool {
	switch {
	case !s.running, s.active >= s.cfg.CaptureConcurrency,
		len(r.RequestURI)+headerSize(r.Header) > maxMetadataBytes:
	case len(s.entries) >= s.cfg.MaxEntries:
		s.reach(LimitEntries)
	case s.retained >= s.cfg.MaxBytes:
		s.reach(LimitBytes)
	default:
		return true
	}
	return false
}

func (s *store) reach(limit Limit) { s.limit = cmp.Or(s.limit, limit) }

func (s *store) refuse() {
	s.mu.Lock()
	s.received++
	s.excluded++
	s.mu.Unlock()
	s.signal()
}

func (s *store) publish(e Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active--
	n := e.size()
	switch {
	case len(s.entries) >= s.cfg.MaxEntries:
		s.reach(LimitEntries)
		s.excluded++
	case n > s.cfg.MaxBytes-s.retained:
		s.reach(LimitBytes)
		s.excluded++
	default:
		e.ID = uint64(len(s.entries)) + 1
		s.entries = append(s.entries, e)
		s.retained += n
	}
	s.signal()
}
