package k8s

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	"github.com/unkn0wn-root/resterm/internal/tunnel"
)

const (
	defaultDialRetryDelay   = 150 * time.Millisecond
	defaultLocalDialTimeout = 10 * time.Second
	closeWaitWindow         = 3 * time.Second
	podPollInterval         = 300 * time.Millisecond
)

type startFn func(context.Context, Cfg, loadSettings) (*session, error)
type dialFn func(context.Context, string, string) (net.Conn, error)

type Manager struct {
	mu sync.Mutex

	cache    map[string]*cacheEntry
	inflight map[string]chan struct{}

	ttl time.Duration
	now func() time.Time

	opt        LoadOpt
	start      startFn
	dial       dialFn
	retryDelay time.Duration
}

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
	closed   sync.Once
	finished sync.Once
	closeFn  func() error
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
	dialer := &net.Dialer{Timeout: defaultLocalDialTimeout}
	return &Manager{
		cache:      make(map[string]*cacheEntry),
		inflight:   make(map[string]chan struct{}),
		ttl:        defaultTTL,
		now:        time.Now,
		start:      startSession,
		dial:       dialer.DialContext,
		retryDelay: defaultDialRetryDelay,
	}
}

func (m *Manager) SetLoadOptions(opt LoadOpt) {
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
	defer m.mu.Unlock()

	var errs []error
	for key, ent := range m.cache {
		if err := ent.close(); err != nil {
			errs = append(errs, err)
		}
		delete(m.cache, key)
	}
	for key, ch := range m.inflight {
		close(ch)
		delete(m.inflight, key)
	}
	return errors.Join(errs...)
}

func (m *Manager) DialContext(
	ctx context.Context,
	cfg Cfg,
	network, addr string,
) (net.Conn, error) {
	if m == nil {
		return nil, errors.New("k8s: manager unavailable")
	}
	if m.start == nil || m.dial == nil {
		return nil, errors.New("k8s: manager unavailable")
	}

	// The target address argument is intentionally ignored for k8s:
	// traffic always goes through the active port-forward session.
	_ = addr

	cfg = normalizeCfg(cfg)
	if cfg.Namespace == "" {
		cfg.Namespace = defaultNamespace
	}
	if cfg.targetRef() == "" {
		return nil, errors.New("k8s: target required")
	}
	if cfg.Port <= 0 && cfg.portRef() == "" {
		return nil, errors.New("k8s: port required")
	}
	if cfg.Port > 65535 {
		return nil, errors.New("k8s: port out of range")
	}

	load, err := m.loadSettings()
	if err != nil {
		return nil, err
	}

	if !cfg.Persist {
		return m.dialOnce(ctx, cfg, load, network)
	}
	return m.dialCached(ctx, cfg, load, network)
}

func (m *Manager) dialOnce(
	ctx context.Context,
	cfg Cfg,
	load loadSettings,
	network string,
) (net.Conn, error) {
	ses, err := m.connect(ctx, cfg, load)
	if err != nil {
		return nil, err
	}

	conn, err := m.dialSession(ctx, ses, network)
	if err != nil {
		return nil, joinCleanupErr(err, ses.close())
	}
	return tunnel.WrapConn(conn, ses.close), nil
}

func (m *Manager) dialCached(
	ctx context.Context,
	cfg Cfg,
	load loadSettings,
	network string,
) (net.Conn, error) {
	key := cacheKey(cfg, load)

	for {
		m.mu.Lock()
		m.purgeLocked()

		ent := m.cache[key]
		if ent != nil {
			ent.touch(m.now())
			ses := ent.ses
			m.mu.Unlock()

			if ses.alive() {
				conn, err := m.dialSession(ctx, ses, network)
				if err == nil {
					return conn, nil
				}
			}

			m.mu.Lock()
			if cur := m.cache[key]; cur == ent {
				_ = ent.close()
				delete(m.cache, key)
			} else {
				_ = ent.close()
			}
			m.mu.Unlock()
			continue
		}

		waitCh, waiting := m.inflight[key]
		if waiting {
			m.mu.Unlock()
			select {
			case <-waitCh:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		token := make(chan struct{})
		m.inflight[key] = token
		m.mu.Unlock()

		ses, err := m.connect(ctx, cfg, load)
		if err != nil {
			m.releaseInflight(key, token)
			return nil, err
		}

		m.mu.Lock()
		if cur := m.cache[key]; cur != nil && cur.alive() {
			m.mu.Unlock()
			_ = ses.close()
			m.releaseInflight(key, token)

			conn, dialErr := m.dialSession(ctx, cur.ses, network)
			if dialErr == nil {
				return conn, nil
			}

			m.mu.Lock()
			if latest := m.cache[key]; latest == cur {
				_ = cur.close()
				delete(m.cache, key)
			} else {
				_ = cur.close()
			}
			m.mu.Unlock()
			continue
		}

		m.cache[key] = newCacheEntry(ses, m.now())
		m.mu.Unlock()
		m.releaseInflight(key, token)

		conn, err := m.dialSession(ctx, ses, network)
		if err == nil {
			return conn, nil
		}

		m.mu.Lock()
		if cur := m.cache[key]; cur != nil && cur.ses == ses {
			delete(m.cache, key)
		}
		m.mu.Unlock()
		return nil, joinCleanupErr(err, ses.close())
	}
}

func (m *Manager) releaseInflight(key string, token chan struct{}) {
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

func (m *Manager) connect(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
	attempts := max(cfg.Retries+1, 1)

	retryDelay := m.retryDelay
	if retryDelay <= 0 {
		retryDelay = defaultDialRetryDelay
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
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

func (m *Manager) purgeLocked() {
	now := m.now()
	for key, ent := range m.cache {
		if now.Sub(ent.lastUsed) <= m.ttl && ent.alive() {
			continue
		}
		_ = ent.close()
		delete(m.cache, key)
	}
}

func (m *Manager) loadSettings() (loadSettings, error) {
	m.mu.Lock()
	opt := m.opt
	m.mu.Unlock()

	opt.ExecAllowlist = append([]string(nil), opt.ExecAllowlist...)
	return normalizeLoadOpt(opt)
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
	s.mu.Unlock()

	s.finished.Do(func() {
		if s.doneCh != nil {
			close(s.doneCh)
		}
	})
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
	if s.closeFn != nil {
		if err := s.closeFn(); err != nil {
			errs = append(errs, err)
		}
	}

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

func cacheKey(cfg Cfg, load loadSettings) string {
	parts := []string{
		cfg.Label,
		cfg.Name,
		cfg.Namespace,
		cfg.targetRef(),
		cfg.portRef(),
		cfg.Context,
		cfg.Kubeconfig,
		cfg.Container,
		cfg.Address,
		strconv.Itoa(cfg.LocalPort),
		connprofile.BoolKey(cfg.Persist),
		cfg.PodWait.String(),
		strconv.Itoa(cfg.Retries),
		string(load.policy),
		connprofile.BoolKey(load.stdinUnavail),
		load.stdinMsg,
		strings.Join(load.allowlist, ","),
	}
	return strings.Join(parts, "|")
}

func loadOptFromCfg(cfg loadSettings) LoadOpt {
	return LoadOpt{
		ExecPolicy:             cfg.policy,
		ExecAllowlist:          append([]string(nil), cfg.allowlist...),
		StdinUnavailable:       cfg.stdinUnavail,
		StdinUnavailableSet:    true,
		StdinUnavailableReason: cfg.stdinMsg,
	}
}
