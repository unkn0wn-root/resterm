package stream

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
)

type DropPolicy int

const (
	DropNewest DropPolicy = iota
	DropOldest
	DropListener
)

const DefaultMaxBytes = 8 << 20

type Config struct {
	BufferSize     int
	ListenerBuffer int
	DropPolicy     DropPolicy
	MaxBytes       bytesize.Budget
}

func defaultConfig(cfg Config) Config {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1024
	}
	if cfg.ListenerBuffer <= 0 {
		cfg.ListenerBuffer = 64
	}
	switch cfg.DropPolicy {
	case DropNewest, DropOldest, DropListener:
	default:
		cfg.DropPolicy = DropOldest
	}
	return cfg
}

type Session struct {
	id   string
	kind Kind

	ctx    context.Context
	cancel context.CancelFunc

	cfg Config

	mu        sync.RWMutex
	state     State
	err       error
	events    *ringBuffer
	listeners map[int]*listener
	nextLID   int

	done chan struct{}

	stats Stats

	closeOnce sync.Once
}

type Stats struct {
	StartedAt   time.Time
	EndedAt     time.Time
	EventsTotal uint64
	BytesTotal  uint64
	Dropped     uint64
	Evicted     uint64
}

type listener struct {
	mu      sync.Mutex
	ch      chan *Event
	dropCnt uint64
	policy  DropPolicy
	closed  bool
}

type Listener struct {
	C        <-chan *Event
	Cancel   func()
	Snapshot Snapshot
	dropped  func() uint64
}

func (l Listener) Dropped() uint64 {
	if l.dropped == nil {
		return 0
	}
	return l.dropped()
}

type Snapshot struct {
	Events  []*Event
	State   State
	Err     error
	Evicted uint64
}

var sessionCounter uint64

// closedEvents is handed to subscribers that arrive after a session ended.
var closedEvents = func() chan *Event {
	ch := make(chan *Event)
	close(ch)
	return ch
}()

func NewSession(parent context.Context, kind Kind, cfg Config) *Session {
	cfg = defaultConfig(cfg)
	ctx, cancel := context.WithCancel(parent)
	return &Session{
		id:        buildSessionID(kind),
		kind:      kind,
		ctx:       ctx,
		cancel:    cancel,
		cfg:       cfg,
		state:     StateConnecting,
		events:    newRingBuffer(cfg.BufferSize, cfg.MaxBytes.Or(DefaultMaxBytes)),
		listeners: make(map[int]*listener),
		done:      make(chan struct{}),
		stats: Stats{
			StartedAt: time.Now(),
		},
	}
}

func buildSessionID(kind Kind) string {
	prefix := "stream"
	switch kind {
	case KindSSE:
		prefix = "sse"
	case KindWebSocket:
		prefix = "ws"
	case KindGRPC:
		prefix = "grpc"
	}
	seq := atomic.AddUint64(&sessionCounter, 1)
	return prefix + "-" + time.Now().UTC().Format("20060102T150405.000000Z") + "-" + itoa(seq)
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) Kind() Kind {
	return s.kind
}

func (s *Session) Context() context.Context {
	return s.ctx
}

func (s *Session) Cancel() {
	s.cancel()
}

func (s *Session) State() (State, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state, s.err
}

func (s *Session) StatsSnapshot() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

func (s *Session) EventsSnapshot() []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.events.snapshot()
}

// Subscribe returns the events still to come plus a snapshot of the ones
// already published. A session that has ended hands back a closed channel, so
// a late subscriber reads the transcript and stops instead of waiting on a
// listener that nothing will ever write to.
func (s *Session) Subscribe() Listener {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := Snapshot{
		Events:  s.events.snapshot(),
		State:   s.state,
		Err:     s.err,
		Evicted: s.stats.Evicted,
	}
	if s.ended() {
		return Listener{
			C:        closedEvents,
			Cancel:   func() {},
			Snapshot: snapshot,
		}
	}

	id := s.nextLID
	s.nextLID++
	l := &listener{
		ch:     make(chan *Event, s.cfg.ListenerBuffer),
		policy: s.cfg.DropPolicy,
	}
	s.listeners[id] = l

	return Listener{
		C: l.ch,
		Cancel: func() {
			s.removeListener(id)
		},
		Snapshot: snapshot,
		dropped:  l.dropped,
	}
}

// ended reports whether Close already ran. Callers must hold s.mu: EndedAt is
// stamped under the lock, while s.done closes just after it is released.
func (s *Session) ended() bool {
	return !s.stats.EndedAt.IsZero()
}

func (s *Session) removeListener(id int) {
	s.mu.Lock()
	l, ok := s.listeners[id]
	if ok {
		delete(s.listeners, id)
	}
	s.mu.Unlock()
	if ok {
		l.close()
	}
}

func (l *listener) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	close(l.ch)
}

func (s *Session) Publish(evt *Event) {
	if evt == nil {
		return
	}
	evt.Sequence = nextSequence()
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}

	s.mu.Lock()
	s.events.append(evt)
	s.stats.Evicted = s.events.evicted
	s.stats.EventsTotal++
	s.stats.BytesTotal += uint64(evt.Size())
	listeners := make([]*listener, 0, len(s.listeners))
	for _, l := range s.listeners {
		listeners = append(listeners, l)
	}
	s.mu.Unlock()

	var dropped uint64
	for _, l := range listeners {
		dropped += l.emit(evt)
	}
	if dropped > 0 {
		s.mu.Lock()
		s.stats.Dropped += dropped
		s.mu.Unlock()
	}
}

func (l *listener) emit(evt *Event) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		l.dropCnt++
		return 1
	}

	switch l.policy {
	case DropNewest:
		select {
		case l.ch <- evt:
			return 0
		default:
			l.dropCnt++
			return 1
		}
	case DropListener:
		select {
		case l.ch <- evt:
			return 0
		default:
			l.dropCnt++
			l.closed = true
			close(l.ch)
			return 1
		}
	default: // DropOldest - when buffer is full, try to discard one old event to make room
		select {
		case l.ch <- evt:
			return 0
		default:
			var dropped uint64
			select {
			case <-l.ch:
				l.dropCnt++
				dropped++
			default:
			}
			select {
			case l.ch <- evt:
				return dropped
			default:
				l.dropCnt++
				return dropped + 1
			}
		}
	}
}

func (l *listener) dropped() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dropCnt
}

func (s *Session) MarkOpen() {
	s.setState(StateOpen, nil)
}

func (s *Session) MarkClosing() {
	s.setState(StateClosing, nil)
}

func (s *Session) Close(err error) {
	s.cancel()
	s.mu.Lock()
	if !s.ended() {
		if err != nil {
			s.state = StateFailed
			s.err = err
		} else {
			s.state = StateClosed
			s.err = nil
		}
		s.stats.EndedAt = time.Now()
	}

	listeners := make([]*listener, 0, len(s.listeners))
	for id, l := range s.listeners {
		listeners = append(listeners, l)
		delete(s.listeners, id)
	}

	s.mu.Unlock()
	for _, l := range listeners {
		l.close()
	}
	s.closeOnce.Do(func() {
		close(s.done)
	})
}

func (s *Session) setState(state State, err error) {
	s.mu.Lock()
	s.state = state
	if err != nil {
		s.err = err
	} else if state == StateClosed {
		s.err = nil
	}
	s.mu.Unlock()
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

func (s *Session) Err() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func itoa(v uint64) string {
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for v > 0 {
		buf = append([]byte{digits[v%10]}, buf...)
		v /= 10
	}
	return string(buf)
}
