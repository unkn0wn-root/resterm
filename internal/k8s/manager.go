package k8s

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/tunnel"
)

const (
	defaultDialRetryDelay   = 150 * time.Millisecond
	defaultLocalDialTimeout = 10 * time.Second
	closeWaitWindow         = 3 * time.Second
	podPollInterval         = 300 * time.Millisecond
)

var (
	errManagerUnavailable = errors.New("k8s: manager unavailable")
	errManagerClosed      = errors.New("k8s: manager closed")
)

type (
	startFn func(context.Context, execConfig, loadSettings) (*session, error)
	dialFn  func(context.Context, string, string) (net.Conn, error)
)

type Manager struct {
	mu sync.Mutex

	cache    map[sessionKey]*cacheEntry
	inflight map[sessionKey]chan struct{}
	closed   bool

	ttl time.Duration
	now func() time.Time

	opt        LoadOptions
	start      startFn
	dial       dialFn
	retryDelay time.Duration
}

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

type cacheEntry struct {
	ses      *session
	lastUsed time.Time
}

type session struct {
	localAddr string
	stopCh    chan struct{}
	doneCh    chan struct{}

	mu       sync.RWMutex
	err      error
	diag     *diagCollector
	ended    bool
	closed   sync.Once
	finished sync.Once
}

func newCacheEntry(ses *session, now time.Time) *cacheEntry {
	return &cacheEntry{ses: ses, lastUsed: now}
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

func NewManager() *Manager {
	return &Manager{
		cache:      make(map[sessionKey]*cacheEntry),
		inflight:   make(map[sessionKey]chan struct{}),
		ttl:        defaultTTL,
		now:        time.Now,
		start:      startSession,
		dial:       newLocalDialer(),
		retryDelay: defaultDialRetryDelay,
	}
}

func newLocalDialer() dialFn {
	dialer := &net.Dialer{Timeout: defaultLocalDialTimeout}
	return dialer.DialContext
}

func (m *Manager) SetLoadOptions(opt LoadOptions) {
	if m == nil {
		return
	}
	opt.ExecAllowlist = append([]string(nil), opt.ExecAllowlist...)
	m.mu.Lock()
	m.opt = opt
	m.mu.Unlock()
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
	m.cache = make(map[sessionKey]*cacheEntry)
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
	network, _ string,
) (net.Conn, error) {
	if err := m.ready(); err != nil {
		return nil, err
	}

	execCfg, err := prepareExecConfig(cfg)
	if err != nil {
		return nil, err
	}

	load, err := m.loadSettings()
	if err != nil {
		return nil, err
	}

	if !execCfg.Persist {
		return m.dialOnce(ctx, execCfg, load, network)
	}
	return m.dialCached(ctx, execCfg, load, network)
}

func (m *Manager) ready() error {
	if m == nil {
		return errManagerUnavailable
	}
	if m.start == nil || m.dial == nil {
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
	load loadSettings,
	network string,
) (net.Conn, error) {
	ses, err := m.connect(ctx, cfg, load)
	if err != nil {
		return nil, err
	}
	if err = m.ready(); err != nil {
		return nil, joinCleanupErr(err, ses.close())
	}

	ses.bindRequestDiag(ctx)
	conn, err := m.dialSession(ctx, ses, network)
	if err != nil {
		return nil, joinCleanupErr(err, ses.close())
	}
	return tunnel.WrapConn(conn, ses.close), nil
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

	ent := m.cache[key]
	if ent != nil {
		ent.touch(m.now())
		ses := ent.ses
		m.mu.Unlock()
		closeEntries(stale)

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
	if cur := m.cache[key]; cur != nil && cur.alive() {
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

	m.cache[key] = newCacheEntry(ses, m.now())
	m.mu.Unlock()
	m.releaseInflight(key, token)

	ses.bindRequestDiag(ctx)
	conn, err := m.dialSession(ctx, ses, network)
	if err == nil {
		return conn, cachedDialReturn, nil
	}

	m.mu.Lock()
	if cur := m.cache[key]; cur != nil && cur.ses == ses {
		delete(m.cache, key)
	}
	m.mu.Unlock()
	return nil, cachedDialReturn, joinCleanupErr(err, ses.close())
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

func (m *Manager) evictCachedSession(key sessionKey, ent *cacheEntry) {
	m.mu.Lock()
	if cur := m.cache[key]; cur == ent {
		delete(m.cache, key)
	}
	m.mu.Unlock()
	_ = ent.close()
}

func (m *Manager) releaseInflight(key sessionKey, token chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cur, ok := m.inflight[key]; ok && cur == token {
		delete(m.inflight, key)
		close(token)
	}
}

func joinCleanupErr(baseErr error, cleanupErr error) error {
	if cleanupErr == nil {
		return baseErr
	}
	if baseErr == nil {
		return cleanupErr
	}
	return errors.Join(baseErr, cleanupErr)
}

func (m *Manager) connect(ctx context.Context, cfg execConfig, load loadSettings) (*session, error) {
	attempts := max(cfg.Retries+1, 1)

	retryDelay := m.retryDelay
	if retryDelay <= 0 {
		retryDelay = defaultDialRetryDelay
	}

	var lastErr error
	for i := range attempts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		ses, err := m.start(ctx, cfg, load)
		if err == nil {
			return ses, nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if i+1 < attempts {
			if err := tunnel.WaitWithContext(ctx, retryDelay); err != nil {
				return nil, err
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("k8s: port-forward start failed")
	}
	return nil, lastErr
}

func (m *Manager) dialSession(ctx context.Context, ses *session, network string) (net.Conn, error) {
	n, err := normalizeNetwork(network)
	if err != nil {
		return nil, err
	}
	if ses == nil || ses.localAddr == "" {
		return nil, errors.New("k8s: local forward address unavailable")
	}
	return m.dial(ctx, n, ses.localAddr)
}

func (m *Manager) purgeLocked() []*cacheEntry {
	now := m.now()
	var stale []*cacheEntry
	for key, ent := range m.cache {
		if now.Sub(ent.lastUsed) <= m.ttl && ent.alive() {
			continue
		}
		delete(m.cache, key)
		stale = append(stale, ent)
	}
	return stale
}

func closeEntries(entries []*cacheEntry) {
	for _, ent := range entries {
		_ = ent.close()
	}
}

func (m *Manager) loadSettings() (loadSettings, error) {
	m.mu.Lock()
	opt := m.opt
	m.mu.Unlock()

	opt.ExecAllowlist = append([]string(nil), opt.ExecAllowlist...)
	return normalizeLoadOptions(opt)
}

func (s *session) alive() bool {
	if s == nil || s.doneCh == nil {
		return false
	}
	select {
	case <-s.doneCh:
		return false
	default:
		return true
	}
}

func (s *session) finish(err error) {
	if s == nil {
		return
	}

	s.mu.Lock()
	s.err = err
	s.ended = true
	diag := s.diag
	s.mu.Unlock()

	s.finished.Do(func() {
		if s.doneCh != nil {
			close(s.doneCh)
		}
	})
	if diag != nil {
		diag.close()
	}
}

func (s *session) close() error {
	if s == nil {
		return nil
	}

	s.closed.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})

	var errs []error
	if s.doneCh != nil {
		select {
		case <-s.doneCh:
		case <-time.After(closeWaitWindow):
			errs = append(errs, errors.New("k8s: timeout closing port-forward"))
		}
	}
	return errors.Join(errs...)
}

func (s *session) errValue() error {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func (s *session) setDiag(collector *diagCollector) {
	if s == nil || collector == nil {
		return
	}

	closeCollector := false
	s.mu.Lock()
	if s.ended {
		closeCollector = true
	} else {
		s.diag = collector
	}
	s.mu.Unlock()

	if closeCollector {
		collector.close()
	}
}

func (s *session) bindRequestDiag(ctx context.Context) {
	if s == nil {
		return
	}

	s.mu.RLock()
	diag := s.diag
	s.mu.RUnlock()

	bindRequestDiag(ctx, diag)
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

func loadOptionsFromSettings(st loadSettings) LoadOptions {
	return LoadOptions{
		ExecPolicy:             st.policy,
		ExecAllowlist:          append([]string(nil), st.allowlist...),
		StdinUnavailable:       st.stdinUnavail,
		StdinUnavailableSet:    true,
		StdinUnavailableReason: st.stdinMsg,
	}
}
