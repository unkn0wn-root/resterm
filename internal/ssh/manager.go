package ssh

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/unkn0wn-root/resterm/internal/tunnel"
)

const defaultTTL = 10 * time.Minute

var (
	errManagerUnavailable = errors.New("ssh: manager unavailable")
	errManagerClosed      = errors.New("ssh: manager closed")
)

type Manager struct {
	pool   *tunnel.Pool[sessionKey, *session]
	opener sessionOpener
}

func NewManager() *Manager {
	return &Manager{
		pool:   newSessionPool(defaultTTL, nil),
		opener: newSessionOpener(dialSSH, dialRetryDelay),
	}
}

func newSessionPool(ttl time.Duration, now func() time.Time) *tunnel.Pool[sessionKey, *session] {
	return tunnel.NewPool[sessionKey, *session](ttl, now, errManagerClosed)
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

	return m.pool.Dial(ctx, execCfg.key,
		func(ctx context.Context) (*session, error) {
			return m.opener.open(ctx, execCfg, true)
		},
		func(_ context.Context, ses *session) (net.Conn, error) {
			return ses.dial(network, addr)
		},
	)
}

func (m *Manager) ready() error {
	if m == nil || m.pool == nil || !m.opener.ready() {
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
	network, addr string,
) (net.Conn, error) {
	ses, err := m.opener.open(ctx, cfg, false)
	if err != nil {
		return nil, err
	}

	if m.pool.Closed() {
		return nil, joinCloseErr(errManagerClosed, ses.Close())
	}

	return ses.dialOnce(network, addr)
}
