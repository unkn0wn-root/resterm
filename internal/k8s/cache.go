package k8s

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"
)

type sessionKey struct {
	label        string
	name         string
	namespace    string
	target       string
	port         string
	context      string
	kubeconfig   string
	container    string
	address      string
	localPort    int
	persist      bool
	podWait      time.Duration
	retries      int
	policy       ExecPolicy
	stdinUnavail bool
	stdinMsg     string
	allowlistKey string
}

type cachedDialStep uint8

const (
	cachedDialReturn cachedDialStep = iota
	cachedDialRetry
	cachedDialAcquire
)

type sessionCache struct {
	entries  map[sessionKey]*cacheEntry
	inflight map[sessionKey]chan struct{}
}

type cacheEntry struct {
	ses      *session
	lastUsed time.Time
}

type pendingClose struct {
	key   sessionKey
	ent   *cacheEntry
	token chan struct{}
}

func newSessionCache() *sessionCache {
	return &sessionCache{
		entries:  make(map[sessionKey]*cacheEntry),
		inflight: make(map[sessionKey]chan struct{}),
	}
}

func (m *Manager) ensureCacheLocked() *sessionCache {
	if m.cache == nil {
		m.cache = newSessionCache()
	}
	return m.cache
}

func newCacheEntry(ses *session, now time.Time) *cacheEntry {
	return &cacheEntry{ses: ses, lastUsed: now}
}

func (c *sessionCache) reset() (map[sessionKey]*cacheEntry, map[sessionKey]chan struct{}) {
	if c == nil {
		return nil, nil
	}

	entries := c.entries
	inflight := c.inflight
	c.entries = make(map[sessionKey]*cacheEntry)
	c.inflight = make(map[sessionKey]chan struct{})
	return entries, inflight
}

func (c *sessionCache) entry(key sessionKey) *cacheEntry {
	if c == nil {
		return nil
	}
	return c.entries[key]
}

func (c *sessionCache) put(key sessionKey, ses *session, now time.Time) *cacheEntry {
	if c == nil {
		return nil
	}
	ent := newCacheEntry(ses, now)
	c.entries[key] = ent
	return ent
}

func (c *sessionCache) deleteSession(key sessionKey, ses *session) {
	if c == nil {
		return
	}
	if cur := c.entries[key]; cur != nil && cur.ses == ses {
		delete(c.entries, key)
	}
}

func (c *sessionCache) wait(key sessionKey) (chan struct{}, bool) {
	if c == nil {
		return nil, false
	}
	ch, ok := c.inflight[key]
	return ch, ok
}

func (c *sessionCache) claim(key sessionKey) chan struct{} {
	if c == nil {
		return nil
	}
	ch := make(chan struct{})
	c.inflight[key] = ch
	return ch
}

func (c *sessionCache) release(key sessionKey, token chan struct{}) bool {
	if c == nil {
		return false
	}
	if cur, ok := c.inflight[key]; ok && cur == token {
		delete(c.inflight, key)
		close(token)
		return true
	}
	return false
}

func (c *sessionCache) purge(now time.Time, ttl time.Duration) []*pendingClose {
	if c == nil {
		return nil
	}

	var stale []*pendingClose
	for key, ent := range c.entries {
		if now.Sub(ent.lastUsed) <= ttl && ent.alive() {
			continue
		}
		stale = append(stale, c.reserveClose(key, ent))
	}
	return stale
}

func (c *sessionCache) reserveClose(key sessionKey, ent *cacheEntry) *pendingClose {
	if c == nil {
		return nil
	}
	delete(c.entries, key)
	if ent == nil {
		return nil
	}
	if _, ok := c.inflight[key]; ok {
		return &pendingClose{key: key, ent: ent}
	}
	token := c.claim(key)
	return &pendingClose{key: key, ent: ent, token: token}
}

func (e *cacheEntry) touch(now time.Time) {
	if e != nil {
		e.lastUsed = now
	}
}

func (e *cacheEntry) alive() bool {
	return e != nil && e.ses.alive()
}

func (e *cacheEntry) close() error {
	if e == nil {
		return nil
	}
	return e.ses.close()
}

func (m *Manager) dialCached(
	ctx context.Context,
	cfg execConfig,
	load loadSettings,
	network string,
) (net.Conn, error) {
	key := sessionKeyFor(cfg, load)

	for {
		conn, step, err := m.tryCachedSession(ctx, key, network)
		switch step {
		case cachedDialReturn:
			return conn, err
		case cachedDialRetry:
			continue
		}

		conn, step, err = m.acquireNewSession(ctx, key, cfg, load, network)
		switch step {
		case cachedDialReturn:
			return conn, err
		case cachedDialRetry:
			continue
		default:
			return nil, errors.New("k8s: invalid cached dial state")
		}
	}
}

func (m *Manager) tryCachedSession(
	ctx context.Context,
	key sessionKey,
	network string,
) (net.Conn, cachedDialStep, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, cachedDialReturn, errManagerClosed
	}
	stale := m.purgeLocked()

	ent := m.cache.entry(key)
	if ent != nil {
		ent.touch(m.now())
		ses := ent.ses
		m.mu.Unlock()
		m.closeEntries(stale)

		if ses.alive() {
			ses.bindRequestDiag(ctx)
			conn, err := m.dialSession(ctx, ses, network)
			if err == nil {
				return conn, cachedDialReturn, nil
			}
		}

		m.evictCachedSession(key, ent)
		return nil, cachedDialRetry, nil
	}

	waitCh, waiting := m.cache.wait(key)
	m.mu.Unlock()
	m.closeEntries(stale)
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
	load loadSettings,
	network string,
) (net.Conn, cachedDialStep, error) {
	token, step, err := m.claimInflight(key)
	if step != cachedDialAcquire {
		return nil, step, err
	}

	ses, err := m.connect(ctx, cfg, load)
	if err != nil {
		m.releaseInflight(key, token)
		return nil, cachedDialReturn, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		m.releaseInflight(key, token)
		return nil, cachedDialReturn, joinCleanupErr(errManagerClosed, ses.close())
	}
	if cur := m.cache.entry(key); cur != nil && cur.alive() {
		m.mu.Unlock()
		_ = ses.close()
		m.releaseInflight(key, token)

		cur.ses.bindRequestDiag(ctx)
		conn, dialErr := m.dialSession(ctx, cur.ses, network)
		if dialErr == nil {
			return conn, cachedDialReturn, nil
		}

		m.evictCachedSession(key, cur)
		return nil, cachedDialRetry, nil
	}

	m.cache.put(key, ses, m.now())
	m.mu.Unlock()
	m.releaseInflight(key, token)

	ses.bindRequestDiag(ctx)
	conn, err := m.dialSession(ctx, ses, network)
	if err == nil {
		return conn, cachedDialReturn, nil
	}

	m.mu.Lock()
	m.cache.deleteSession(key, ses)
	m.mu.Unlock()
	return nil, cachedDialReturn, joinCleanupErr(err, ses.close())
}

func (m *Manager) claimInflight(key sessionKey) (chan struct{}, cachedDialStep, error) {
	m.mu.Lock()

	if m.closed {
		m.mu.Unlock()
		return nil, cachedDialReturn, errManagerClosed
	}
	cache := m.ensureCacheLocked()
	stale := m.purgeLocked()
	if cache.entry(key) != nil {
		m.mu.Unlock()
		m.closeEntries(stale)
		return nil, cachedDialRetry, nil
	}
	if _, ok := cache.wait(key); ok {
		m.mu.Unlock()
		m.closeEntries(stale)
		return nil, cachedDialRetry, nil
	}

	token := cache.claim(key)
	m.mu.Unlock()
	m.closeEntries(stale)
	return token, cachedDialAcquire, nil
}

func (m *Manager) evictCachedSession(key sessionKey, ent *cacheEntry) {
	m.mu.Lock()
	var pending *pendingClose
	if cur := m.cache.entry(key); cur == ent {
		pending = m.cache.reserveClose(key, ent)
	}
	m.mu.Unlock()

	if pending != nil {
		m.closeEntries([]*pendingClose{pending})
		return
	}
	_ = ent.close()
}

func (m *Manager) releaseInflight(key sessionKey, token chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ensureCacheLocked().release(key, token)
}

func (m *Manager) purgeLocked() []*pendingClose {
	return m.ensureCacheLocked().purge(m.now(), m.ttl)
}

func (m *Manager) closeEntries(entries []*pendingClose) {
	for _, pending := range entries {
		if pending == nil {
			continue
		}
		_ = pending.ent.close()
		if pending.token != nil {
			m.releaseInflight(pending.key, pending.token)
		}
	}
}

func sessionKeyFor(cfg execConfig, load loadSettings) sessionKey {
	return sessionKey{
		label:        cfg.Label,
		name:         cfg.Name,
		namespace:    cfg.Namespace,
		target:       cfg.targetRef(),
		port:         cfg.portRef(),
		context:      cfg.Context,
		kubeconfig:   cfg.Kubeconfig,
		container:    cfg.Container,
		address:      cfg.Address,
		localPort:    cfg.LocalPort,
		persist:      cfg.Persist,
		podWait:      cfg.PodWait,
		retries:      cfg.Retries,
		policy:       load.policy,
		stdinUnavail: load.stdinUnavail,
		stdinMsg:     load.stdinMsg,
		allowlistKey: allowlistCacheKey(load.allowlist),
	}
}

func allowlistCacheKey(allowlist []string) string {
	return strings.Join(allowlist, "\x00")
}
