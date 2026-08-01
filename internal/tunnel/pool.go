package tunnel

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

var ErrPoolClosed = errors.New("tunnel: session pool closed")

type PoolSession interface {
	Alive() bool
	Close() error
}

type (
	OpenFunc[S PoolSession]     func(context.Context) (S, error)
	PoolDialFunc[S PoolSession] func(context.Context, S) (net.Conn, error)
)

type poolState uint8

const (
	poolReturn poolState = iota
	poolRetry
	poolOpen
)

// Pool caches live sessions by key and deduplicates concurrent opens.
// While a session for a key is being closed, the key's inflight slot stays
// reserved so a new open cannot race the release of local resources.
type Pool[K comparable, S PoolSession] struct {
	mu sync.Mutex

	entries  map[K]*poolEntry[S]
	inflight map[K]chan struct{}
	closed   bool

	ttl       time.Duration
	now       func() time.Time
	closedErr error
}

type poolEntry[S PoolSession] struct {
	ses  S
	used time.Time
}

type staleEntry[K comparable, S PoolSession] struct {
	key   K
	ses   S
	token chan struct{}
}

func NewPool[K comparable, S PoolSession](
	ttl time.Duration,
	now func() time.Time,
	closedErr error,
) *Pool[K, S] {
	if now == nil {
		now = time.Now
	}
	if closedErr == nil {
		closedErr = ErrPoolClosed
	}
	return &Pool[K, S]{
		entries:   make(map[K]*poolEntry[S]),
		inflight:  make(map[K]chan struct{}),
		ttl:       ttl,
		now:       now,
		closedErr: closedErr,
	}
}

func (p *Pool[K, S]) Dial(
	ctx context.Context,
	key K,
	open OpenFunc[S],
	dial PoolDialFunc[S],
) (net.Conn, error) {
	for {
		conn, state, err := p.tryCached(ctx, key, dial)
		switch state {
		case poolReturn:
			return conn, err
		case poolRetry:
			continue
		}

		conn, state, err = p.openNew(ctx, key, open, dial)
		switch state {
		case poolReturn:
			return conn, err
		case poolRetry:
			continue
		default:
			return nil, errors.New("tunnel: invalid pool dial state")
		}
	}
}

func (p *Pool[K, S]) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true

	entries := p.entries
	inflight := p.inflight
	p.entries = make(map[K]*poolEntry[S])
	p.inflight = make(map[K]chan struct{})
	p.mu.Unlock()

	for _, ch := range inflight {
		close(ch)
	}

	var errs []error
	for _, ent := range entries {
		if err := ent.ses.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *Pool[K, S]) Closed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *Pool[K, S]) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}

func (p *Pool[K, S]) Peek(key K) (S, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ent := p.entries[key]; ent != nil {
		return ent.ses, true
	}
	var zero S
	return zero, false
}

// Put stores an already open session under key, replacing any current entry
// without closing it. On a closed pool the session is closed instead.
func (p *Pool[K, S]) Put(key K, ses S) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = ses.Close()
		return
	}
	p.entries[key] = &poolEntry[S]{ses: ses, used: p.now()}
	p.mu.Unlock()
}

func (p *Pool[K, S]) tryCached(
	ctx context.Context,
	key K,
	dial PoolDialFunc[S],
) (net.Conn, poolState, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, poolReturn, p.closedErr
	}
	stale := p.purgeLocked()

	if ent := p.entries[key]; ent != nil {
		ent.used = p.now()
		ses := ent.ses
		p.mu.Unlock()
		_ = p.closeStale(stale)

		if ses.Alive() {
			conn, err := dial(ctx, ses)
			if err == nil {
				return conn, poolReturn, nil
			}
		}

		p.evict(key, ent)
		return nil, poolRetry, nil
	}

	ch, wait := p.inflight[key]
	p.mu.Unlock()
	_ = p.closeStale(stale)
	if !wait {
		return nil, poolOpen, nil
	}

	select {
	case <-ch:
		return nil, poolRetry, nil
	case <-ctx.Done():
		return nil, poolReturn, ctx.Err()
	}
}

func (p *Pool[K, S]) openNew(
	ctx context.Context,
	key K,
	open OpenFunc[S],
	dial PoolDialFunc[S],
) (net.Conn, poolState, error) {
	token, state, err := p.claim(key)
	if state != poolOpen {
		return nil, state, err
	}

	ses, err := open(ctx)
	if err != nil {
		p.release(key, token)
		return nil, poolReturn, err
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		p.release(key, token)
		err := p.closedErr
		if closeErr := ses.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return nil, poolReturn, err
	}
	if cur := p.entries[key]; cur != nil && cur.ses.Alive() {
		p.mu.Unlock()
		_ = ses.Close()
		p.release(key, token)

		conn, dialErr := dial(ctx, cur.ses)
		if dialErr == nil {
			return conn, poolReturn, nil
		}
		p.evict(key, cur)
		return nil, poolRetry, nil
	}

	ent := &poolEntry[S]{ses: ses, used: p.now()}
	p.entries[key] = ent
	p.mu.Unlock()
	p.release(key, token)

	conn, err := dial(ctx, ses)
	if err == nil {
		return conn, poolReturn, nil
	}

	p.mu.Lock()
	if cur := p.entries[key]; cur == ent {
		st := p.removeLocked(key, ent)
		p.mu.Unlock()
		if closeErr := p.closeStale([]staleEntry[K, S]{st}); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
		return nil, poolReturn, err
	}
	p.mu.Unlock()
	if closeErr := ses.Close(); closeErr != nil {
		err = errors.Join(err, closeErr)
	}
	return nil, poolReturn, err
}

func (p *Pool[K, S]) claim(key K) (chan struct{}, poolState, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, poolReturn, p.closedErr
	}
	stale := p.purgeLocked()
	if p.entries[key] != nil || p.inflight[key] != nil {
		p.mu.Unlock()
		_ = p.closeStale(stale)
		return nil, poolRetry, nil
	}

	ch := make(chan struct{})
	p.inflight[key] = ch
	p.mu.Unlock()
	_ = p.closeStale(stale)
	return ch, poolOpen, nil
}

func (p *Pool[K, S]) release(key K, token chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cur, ok := p.inflight[key]; ok && cur == token {
		delete(p.inflight, key)
		close(token)
	}
}

func (p *Pool[K, S]) evict(key K, ent *poolEntry[S]) {
	p.mu.Lock()
	if cur := p.entries[key]; cur == ent {
		st := p.removeLocked(key, ent)
		p.mu.Unlock()
		_ = p.closeStale([]staleEntry[K, S]{st})
		return
	}
	p.mu.Unlock()
	_ = ent.ses.Close()
}

// removeLocked drops the entry and, when the key has no opener inflight,
// reserves the inflight slot until closeStale releases it after the close.
func (p *Pool[K, S]) removeLocked(key K, ent *poolEntry[S]) staleEntry[K, S] {
	delete(p.entries, key)
	st := staleEntry[K, S]{key: key, ses: ent.ses}
	if _, ok := p.inflight[key]; !ok {
		ch := make(chan struct{})
		p.inflight[key] = ch
		st.token = ch
	}
	return st
}

func (p *Pool[K, S]) purgeLocked() []staleEntry[K, S] {
	now := p.now()
	var stale []staleEntry[K, S]
	for key, ent := range p.entries {
		if now.Sub(ent.used) <= p.ttl && ent.ses.Alive() {
			continue
		}
		stale = append(stale, p.removeLocked(key, ent))
	}
	return stale
}

func (p *Pool[K, S]) closeStale(stale []staleEntry[K, S]) error {
	var errs []error
	for _, st := range stale {
		if err := st.ses.Close(); err != nil {
			errs = append(errs, err)
		}
		if st.token != nil {
			p.release(st.key, st.token)
		}
	}
	return errors.Join(errs...)
}
