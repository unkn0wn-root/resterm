package k8s

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/tunnel"
)

const (
	defaultDialRetryDelay   = 150 * time.Millisecond
	defaultLocalDialTimeout = 10 * time.Second
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

	cache  *sessionCache
	closed bool

	ttl time.Duration
	now func() time.Time

	opt        LoadOptions
	start      startFn
	dial       dialFn
	retryDelay time.Duration
}

func NewManager() *Manager {
	return &Manager{
		cache:      newSessionCache(),
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

	entries, inflight := m.ensureCacheLocked().reset()
	m.mu.Unlock()

	for _, ch := range inflight {
		close(ch)
	}

	var errs []error
	for _, ent := range entries {
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

func (m *Manager) connect(
	ctx context.Context,
	cfg execConfig,
	load loadSettings,
) (*session, error) {
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
	addr, err := ses.localAddress()
	if err != nil {
		return nil, err
	}
	return m.dial(ctx, n, addr)
}

func (m *Manager) loadSettings() (loadSettings, error) {
	m.mu.Lock()
	opt := m.opt
	m.mu.Unlock()

	opt.ExecAllowlist = append([]string(nil), opt.ExecAllowlist...)
	return normalizeLoadOptions(opt)
}
