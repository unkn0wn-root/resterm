package ssh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"
)

const defaultTTL = 10 * time.Minute

type sessionKey struct {
	label       string
	name        string
	host        string
	port        int
	user        string
	keyPath     string
	passHash    string
	keyPassHash string
	knownHosts  string
	strict      bool
	agent       bool
	persist     bool
	timeout     time.Duration
	keepAlive   time.Duration
	retries     int
}

type cacheState uint8

const (
	cacheReturn cacheState = iota
	cacheRetry
	cacheOpen
)

type sessionCache struct {
	mu sync.Mutex

	entries  map[sessionKey]*cacheEntry
	inflight map[sessionKey]chan struct{}
	closed   bool

	ttl time.Duration
	now func() time.Time
}

type cacheEntry struct {
	ses  *session
	used time.Time
}

func newSessionCache(ttl time.Duration, now func() time.Time) *sessionCache {
	if now == nil {
		now = time.Now
	}
	return &sessionCache{
		entries:  make(map[sessionKey]*cacheEntry),
		inflight: make(map[sessionKey]chan struct{}),
		ttl:      ttl,
		now:      now,
	}
}

func newCacheEntry(ses *session, now time.Time) *cacheEntry {
	return &cacheEntry{ses: ses, used: now}
}

func (e *cacheEntry) touch(now time.Time) {
	if e != nil {
		e.used = now
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

func (c *sessionCache) dial(
	ctx context.Context,
	cfg execConfig,
	op sessionOpener,
	network, addr string,
) (net.Conn, error) {
	key := cfg.key

	for {
		conn, state, err := c.try(ctx, key, network, addr)
		switch state {
		case cacheReturn:
			return conn, err
		case cacheRetry:
			continue
		}

		conn, state, err = c.open(ctx, key, cfg, op, network, addr)
		switch state {
		case cacheReturn:
			return conn, err
		case cacheRetry:
			continue
		default:
			return nil, errors.New("ssh: invalid cache dial state")
		}
	}
}

func (c *sessionCache) close() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true

	entries := c.entries
	inflight := c.inflight
	c.entries = make(map[sessionKey]*cacheEntry)
	c.inflight = make(map[sessionKey]chan struct{})
	c.mu.Unlock()

	for _, ch := range inflight {
		close(ch)
	}

	return closeCacheEntries(entries)
}

func (c *sessionCache) isClosed() bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *sessionCache) try(
	ctx context.Context,
	key sessionKey,
	network, addr string,
) (net.Conn, cacheState, error) {
	if c == nil {
		return nil, cacheReturn, errManagerUnavailable
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, cacheReturn, errManagerClosed
	}
	stale := c.purgeLocked()

	ent := c.entries[key]
	if ent != nil {
		ent.touch(c.time())
		ses := ent.ses
		c.mu.Unlock()
		closeStale(stale)

		if ses.alive() {
			conn, err := ses.dial(network, addr)
			if err == nil {
				return conn, cacheReturn, nil
			}
		}

		c.evict(key, ent)
		return nil, cacheRetry, nil
	}

	ch, wait := c.inflight[key]
	c.mu.Unlock()
	closeStale(stale)
	if !wait {
		return nil, cacheOpen, nil
	}

	select {
	case <-ch:
		return nil, cacheRetry, nil
	case <-ctx.Done():
		return nil, cacheReturn, ctx.Err()
	}
}

func (c *sessionCache) open(
	ctx context.Context,
	key sessionKey,
	cfg execConfig,
	op sessionOpener,
	network, addr string,
) (net.Conn, cacheState, error) {
	ch, state, err := c.claim(key)
	if state != cacheOpen {
		return nil, state, err
	}

	ses, err := op.open(ctx, cfg, true)
	if err != nil {
		c.release(key, ch)
		return nil, cacheReturn, err
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		c.release(key, ch)
		return nil, cacheReturn, joinCloseErr(errManagerClosed, ses.close())
	}
	if cur := c.entries[key]; cur != nil && cur.alive() {
		c.mu.Unlock()
		_ = ses.close()
		c.release(key, ch)

		conn, dialErr := cur.ses.dial(network, addr)
		if dialErr == nil {
			return conn, cacheReturn, nil
		}
		c.evict(key, cur)
		return nil, cacheRetry, nil
	}

	ent := newCacheEntry(ses, c.time())
	c.entries[key] = ent
	c.mu.Unlock()
	c.release(key, ch)

	conn, err := ses.dial(network, addr)
	if err == nil {
		return conn, cacheReturn, nil
	}

	c.mu.Lock()
	if cur := c.entries[key]; cur == ent {
		delete(c.entries, key)
	}
	c.mu.Unlock()
	return nil, cacheReturn, joinCloseErr(err, ses.close())
}

func (c *sessionCache) claim(key sessionKey) (chan struct{}, cacheState, error) {
	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()
		return nil, cacheReturn, errManagerClosed
	}
	stale := c.purgeLocked()
	if c.entries[key] != nil || c.inflight[key] != nil {
		c.mu.Unlock()
		closeStale(stale)
		return nil, cacheRetry, nil
	}

	ch := make(chan struct{})
	c.inflight[key] = ch
	c.mu.Unlock()
	closeStale(stale)
	return ch, cacheOpen, nil
}

func (c *sessionCache) release(key sessionKey, ch chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cur, ok := c.inflight[key]; ok && cur == ch {
		delete(c.inflight, key)
		close(ch)
	}
}

func (c *sessionCache) evict(key sessionKey, ent *cacheEntry) {
	c.mu.Lock()
	if cur := c.entries[key]; cur == ent {
		delete(c.entries, key)
	}
	c.mu.Unlock()
	_ = ent.close()
}

func (c *sessionCache) purgeLocked() []*cacheEntry {
	now := c.time()
	var stale []*cacheEntry
	for key, ent := range c.entries {
		if ent != nil && now.Sub(ent.used) <= c.ttl && ent.alive() {
			continue
		}
		delete(c.entries, key)
		stale = append(stale, ent)
	}
	return stale
}

func (c *sessionCache) time() time.Time {
	if c.now == nil {
		return time.Now()
	}
	return c.now()
}

func closeStale(entries []*cacheEntry) {
	for _, ent := range entries {
		_ = ent.close()
	}
}

func closeCacheEntries(entries map[sessionKey]*cacheEntry) error {
	var errs []error
	for _, ent := range entries {
		if err := ent.close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// sessionKeyFor expects cfg to have already passed through Config.normalize.
func sessionKeyFor(cfg Config) sessionKey {
	return sessionKey{
		label:       cfg.Label,
		name:        cfg.Name,
		host:        cfg.Host,
		port:        cfg.Port,
		user:        cfg.User,
		keyPath:     cfg.KeyPath,
		passHash:    hashIfSet(cfg.Pass),
		keyPassHash: hashIfSet(cfg.KeyPass),
		knownHosts:  cfg.KnownHosts,
		strict:      cfg.Strict,
		agent:       cfg.Agent,
		persist:     cfg.Persist,
		timeout:     cfg.Timeout,
		keepAlive:   cfg.KeepAlive,
		retries:     cfg.Retries,
	}
}

func hashIfSet(secret string) string {
	if secret == "" {
		return ""
	}
	return hashSecret(secret)
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
