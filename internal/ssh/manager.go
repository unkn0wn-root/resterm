package ssh

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/tunnel"
)

const dialRetryDelay = 150 * time.Millisecond

var (
	errManagerUnavailable = errors.New("ssh: manager unavailable")
	errManagerClosed      = errors.New("ssh: manager closed")
)

type client interface {
	Dial(network, addr string) (net.Conn, error)
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
	Close() error
}

type Manager struct {
	mu sync.Mutex

	cache    map[sessionKey]*entry
	inflight map[sessionKey]chan struct{}
	closed   bool

	ttl        time.Duration
	now        func() time.Time
	dial       func(context.Context, execConfig) (client, error)
	retryDelay time.Duration
}

type cachedDialStep uint8

const (
	cachedDialReturn cachedDialStep = iota
	cachedDialRetry
	cachedDialAcquire
)

type entry struct {
	ses      *session
	lastUsed time.Time
}

func newEntry(ses *session, now time.Time) *entry {
	return &entry{ses: ses, lastUsed: now}
}

func (e *entry) touch(now time.Time) {
	if e != nil {
		e.lastUsed = now
	}
}

func (e *entry) alive() bool {
	return e != nil && e.ses.alive()
}

func (e *entry) close() error {
	if e == nil {
		return nil
	}
	return e.ses.close()
}

func NewManager() *Manager {
	return &Manager{
		cache:      make(map[sessionKey]*entry),
		inflight:   make(map[sessionKey]chan struct{}),
		ttl:        defaultTTL,
		now:        time.Now,
		dial:       dialSSH,
		retryDelay: dialRetryDelay,
	}
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true

	cache := m.cache
	inflight := m.inflight
	m.cache = make(map[sessionKey]*entry)
	m.inflight = make(map[sessionKey]chan struct{})
	m.mu.Unlock()

	for _, ch := range inflight {
		close(ch)
	}

	var errs []error
	for _, ent := range cache {
		if err := ent.close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) DialContext(
	ctx context.Context,
	cfg Config,
	network, addr string,
) (net.Conn, error) {
	if err := m.ready(); err != nil {
		return nil, err
	}

	execCfg, err := prepareExecConfig(cfg)
	if err != nil {
		return nil, err
	}

	if !execCfg.Persist {
		return m.dialOnce(ctx, execCfg, network, addr)
	}
	return m.dialCached(ctx, execCfg, network, addr)
}

func (m *Manager) ready() error {
	if m == nil {
		return errManagerUnavailable
	}
	if m.dial == nil {
		return errManagerUnavailable
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errManagerClosed
	}
	return nil
}

func (m *Manager) dialOnce(
	ctx context.Context,
	cfg execConfig,
	network, addr string,
) (net.Conn, error) {
	ses, err := m.connect(ctx, cfg, false)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, joinCloseErr(errManagerClosed, ses.close())
	}

	conn, err := ses.dial(network, addr)
	if err != nil {
		return nil, joinCloseErr(err, ses.close())
	}
	return tunnel.WrapConn(conn, ses.close), nil
}

func (m *Manager) dialCached(
	ctx context.Context,
	cfg execConfig,
	network, addr string,
) (net.Conn, error) {
	key := cfg.key

	for {
		conn, step, err := m.tryCachedSession(ctx, key, network, addr)
		switch step {
		case cachedDialReturn:
			return conn, err
		case cachedDialRetry:
			continue
		}

		conn, step, err = m.acquireNewSession(ctx, key, cfg, network, addr)
		switch step {
		case cachedDialReturn:
			return conn, err
		case cachedDialRetry:
			continue
		default:
			return nil, errors.New("ssh: invalid cached dial state")
		}
	}
}

func (m *Manager) tryCachedSession(
	ctx context.Context,
	key sessionKey,
	network, addr string,
) (net.Conn, cachedDialStep, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, cachedDialReturn, errManagerClosed
	}
	stale := m.purgeLocked()

	ent := m.cache[key]
	if ent != nil {
		ent.touch(m.now())
		ses := ent.ses
		m.mu.Unlock()
		closeEntries(stale)

		if ses.alive() {
			conn, err := ses.dial(network, addr)
			if err == nil {
				return conn, cachedDialReturn, nil
			}
		}

		m.evictCachedSession(key, ent)
		return nil, cachedDialRetry, nil
	}

	waitCh, waiting := m.inflight[key]
	m.mu.Unlock()
	closeEntries(stale)
	if !waiting {
		return nil, cachedDialAcquire, nil
	}

	select {
	case <-waitCh:
		return nil, cachedDialRetry, nil
	case <-ctx.Done():
		return nil, cachedDialReturn, ctx.Err()
	}
}

func (m *Manager) acquireNewSession(
	ctx context.Context,
	key sessionKey,
	cfg execConfig,
	network, addr string,
) (net.Conn, cachedDialStep, error) {
	token, step, err := m.claimInflight(key)
	if step != cachedDialAcquire {
		return nil, step, err
	}

	ses, err := m.connect(ctx, cfg, true)
	if err != nil {
		m.releaseInflight(key, token)
		return nil, cachedDialReturn, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		m.releaseInflight(key, token)
		return nil, cachedDialReturn, joinCloseErr(errManagerClosed, ses.close())
	}
	if cur := m.cache[key]; cur != nil && cur.alive() {
		m.mu.Unlock()
		_ = ses.close()
		m.releaseInflight(key, token)

		conn, dialErr := cur.ses.dial(network, addr)
		if dialErr == nil {
			return conn, cachedDialReturn, nil
		}
		m.evictCachedSession(key, cur)
		return nil, cachedDialRetry, nil
	}

	ent := newEntry(ses, m.now())
	m.cache[key] = ent
	m.mu.Unlock()
	m.releaseInflight(key, token)

	conn, err := ses.dial(network, addr)
	if err == nil {
		return conn, cachedDialReturn, nil
	}

	m.mu.Lock()
	if cur := m.cache[key]; cur == ent {
		delete(m.cache, key)
	}
	m.mu.Unlock()
	return nil, cachedDialReturn, joinCloseErr(err, ses.close())
}

func (m *Manager) claimInflight(key sessionKey) (chan struct{}, cachedDialStep, error) {
	m.mu.Lock()

	if m.closed {
		m.mu.Unlock()
		return nil, cachedDialReturn, errManagerClosed
	}
	stale := m.purgeLocked()
	if m.cache[key] != nil || m.inflight[key] != nil {
		m.mu.Unlock()
		closeEntries(stale)
		return nil, cachedDialRetry, nil
	}

	token := make(chan struct{})
	m.inflight[key] = token
	m.mu.Unlock()
	closeEntries(stale)
	return token, cachedDialAcquire, nil
}

func (m *Manager) releaseInflight(key sessionKey, token chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cur, ok := m.inflight[key]; ok && cur == token {
		delete(m.inflight, key)
		close(token)
	}
}

func (m *Manager) evictCachedSession(key sessionKey, ent *entry) {
	m.mu.Lock()
	if cur := m.cache[key]; cur == ent {
		delete(m.cache, key)
	}
	m.mu.Unlock()
	_ = ent.close()
}

func (m *Manager) connect(ctx context.Context, cfg execConfig, cached bool) (*session, error) {
	attempts := cfg.retry + 1
	if attempts < 1 {
		attempts = 1
	}

	delay := m.retryDelay
	if delay <= 0 {
		delay = dialRetryDelay
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		cli, err := m.dial(ctx, cfg)
		if err == nil {
			keepAlive := time.Duration(0)
			if cached {
				keepAlive = cfg.KeepAlive
			}
			return newSession(cli, keepAlive), nil
		}

		lastErr = err
		if i+1 < attempts {
			if err := tunnel.WaitWithContext(ctx, delay); err != nil {
				return nil, err
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("ssh dial failed")
	}
	return nil, lastErr
}

func (m *Manager) purgeLocked() []*entry {
	now := m.now()
	var stale []*entry
	for key, ent := range m.cache {
		if now.Sub(ent.lastUsed) <= m.ttl && ent.alive() {
			continue
		}
		delete(m.cache, key)
		stale = append(stale, ent)
	}
	return stale
}

func closeEntries(entries []*entry) {
	for _, ent := range entries {
		_ = ent.close()
	}
}
