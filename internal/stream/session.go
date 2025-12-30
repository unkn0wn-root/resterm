package stream

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type DropPolicy int

const (
	DropNewest DropPolicy = iota
	DropOldest
	DropListener
)

type Config struct {
	BufferSize     int
	ListenerBuffer int
	DropPolicy     DropPolicy
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
}

type Stats struct {
	StartedAt   time.Time
	EndedAt     time.Time
	EventsTotal uint64
	BytesTotal  uint64
	Dropped     uint64
}

type listener struct {
	ch        chan *Event
	dropCnt   uint64
	policy    DropPolicy
	closed    int32
	closeOnce sync.Once
}

type Listener struct {
	C        <-chan *Event
	Cancel   func()
	Snapshot Snapshot
}

type Snapshot struct {
	Events []*Event
	State  State
	Err    error
}

var sessionCounter uint64

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
		events:    newRingBuffer(cfg.BufferSize),
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

func (s *Session) Subscribe() Listener {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextLID
	s.nextLID++
	l := &listener{
		ch:     make(chan *Event, s.cfg.ListenerBuffer),
		policy: s.cfg.DropPolicy,
	}
	s.listeners[id] = l

	snapshot := Snapshot{
		Events: s.events.snapshot(),
		State:  s.state,
		Err:    s.err,
	}

	return Listener{
		C: l.ch,
		Cancel: func() {
			s.removeListener(id)
		},
		Snapshot: snapshot,
	}
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
	l.closeOnce.Do(func() {
		atomic.StoreInt32(&l.closed, 1)
		close(l.ch)
	})
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
	s.stats.EventsTotal++
	s.stats.BytesTotal += uint64(len(evt.Payload))
	listeners := make([]*listener, 0, len(s.listeners))
	for _, l := range s.listeners {
		listeners = append(listeners, l)
	}
	s.mu.Unlock()

	dropped := 0
	for _, l := range listeners {
		if !l.emit(evt) {
			dropped++
		}
	}
	if dropped > 0 {
		s.mu.Lock()
		s.stats.Dropped += uint64(dropped)
		s.mu.Unlock()
	}
}

func (l *listener) emit(evt *Event) bool {
	if atomic.LoadInt32(&l.closed) == 1 {
		return false
	}

	send := func() bool {
		switch l.policy {
		case DropNewest:
			select {
			case l.ch <- evt:
				return true
			default:
				l.dropCnt++
				return false
			}
		case DropListener:
			select {
			case l.ch <- evt:
				return true
			default:
				l.close()
				return false
			}
		default: // DropOldest - when buffer is full, try to discard one old event to make room
			select {
			case l.ch <- evt:
				return true
			default:
				select {
				case <-l.ch:
				default:
				}
				select {
				case l.ch <- evt:
					return true
				default:
					l.dropCnt++
					return false
				}
			}
		}
	}
	defer func() {
		if r := recover(); r != nil {
			atomic.StoreInt32(&l.closed, 1)
			l.dropCnt++
		}
	}()
	return send()
}

func (s *Session) MarkOpen() {
	s.setState(StateOpen, nil)
}

func (s *Session) MarkClosing() {
	s.setState(StateClosing, nil)
}

func (s *Session) Close(err error) {
	if err != nil {
		s.setState(StateFailed, err)
	} else {
		s.setState(StateClosed, nil)
	}

	s.cancel()
	s.mu.Lock()
	if s.stats.EndedAt.IsZero() {
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
	select {
	case <-s.done:
	default:
		close(s.done)
	}
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
