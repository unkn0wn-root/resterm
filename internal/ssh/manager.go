package ssh

import (
	"context"
	"errors"
	"net"
)

var (
	errManagerUnavailable = errors.New("ssh: manager unavailable")
	errManagerClosed      = errors.New("ssh: manager closed")
)

type Manager struct {
	cache  *sessionCache
	opener sessionOpener
}

func NewManager() *Manager {
	return &Manager{
		cache:  newSessionCache(defaultTTL, nil),
		opener: newSessionOpener(dialSSH, dialRetryDelay),
	}
}

func (m *Manager) Close() error {
	if m == nil || m.cache == nil {
		return nil
	}
	return m.cache.close()
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
	if m == nil || m.cache == nil || !m.opener.ready() {
		return errManagerUnavailable
	}
	if m.cache.isClosed() {
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

	if m.cache.isClosed() {
		return nil, joinCloseErr(errManagerClosed, ses.close())
	}

	return ses.dialOnce(network, addr)
}

func (m *Manager) dialCached(
	ctx context.Context,
	cfg execConfig,
	network, addr string,
) (net.Conn, error) {
	return m.cache.dial(ctx, cfg, m.opener, network, addr)
}
