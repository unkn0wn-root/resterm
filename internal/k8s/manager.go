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
	mu  sync.Mutex
	opt LoadOptions

	pool *tunnel.Pool[sessionKey, *session]

	start      startFn
	dial       dialFn
	retryDelay time.Duration
}

func NewManager() *Manager {
	return &Manager{
		pool:       newSessionPool(defaultTTL, nil),
		start:      startSession,
		dial:       newLocalDialer(),
		retryDelay: defaultDialRetryDelay,
	}
}

func newSessionPool(ttl time.Duration, now func() time.Time) *tunnel.Pool[sessionKey, *session] {
	return tunnel.NewPool[sessionKey, *session](ttl, now, errManagerClosed)
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
	if m == nil || m.pool == nil {
		return nil
	}
	return m.pool.Close()
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

	return m.pool.Dial(ctx, sessionKeyFor(execCfg, load),
		func(ctx context.Context) (*session, error) {
			return m.connect(ctx, execCfg, load)
		},
		func(ctx context.Context, ses *session) (net.Conn, error) {
			ses.bindRequestDiag(ctx)
			return m.dialSession(ctx, ses, network)
		},
	)
}

func (m *Manager) ready() error {
	if m == nil || m.pool == nil || m.start == nil || m.dial == nil {
		return errManagerUnavailable
	}
	if m.pool.Closed() {
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
	if m.pool.Closed() {
		return nil, joinCleanupErr(errManagerClosed, ses.Close())
	}

	ses.bindRequestDiag(ctx)
	conn, err := m.dialSession(ctx, ses, network)
	if err != nil {
		return nil, joinCleanupErr(err, ses.Close())
	}
	return tunnel.WrapConn(conn, ses.Close), nil
}

func (m *Manager) connect(
	ctx context.Context,
	cfg execConfig,
	load loadSettings,
) (*session, error) {
	delay := m.retryDelay
	if delay <= 0 {
		delay = defaultDialRetryDelay
	}

	return tunnel.OpenRetry(ctx, cfg.Retries+1, delay,
		func(ctx context.Context) (*session, error) {
			return m.start(ctx, cfg, load)
		})
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
	return normalizeLoadOptions(opt)
}
