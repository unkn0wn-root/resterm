package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	knownhosts "golang.org/x/crypto/ssh/knownhosts"
)

const dialRetryDelay = 150 * time.Millisecond

type Client interface {
	Dial(network, addr string) (net.Conn, error)
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
	Close() error
}

type Manager struct {
	mu    sync.Mutex
	cache map[string]*entry
	ttl   time.Duration
	now   func() time.Time
	dial  func(context.Context, Cfg) (Client, error)
}

type entry struct {
	cfg      Cfg
	cli      Client
	lastUsed time.Time
	stop     chan struct{}
}

func NewManager() *Manager {
	return &Manager{
		cache: make(map[string]*entry),
		ttl:   defaultTTL,
		now:   time.Now,
		dial:  dialSSH,
	}
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var errs []error
	for key, ent := range m.cache {
		if err := closeEntry(ent); err != nil {
			errs = append(errs, err)
		}
		delete(m.cache, key)
	}
	return errors.Join(errs...)
}

func (m *Manager) DialContext(
	ctx context.Context,
	cfg Cfg,
	network, addr string,
) (net.Conn, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("ssh host required")
	}
	if !cfg.Persist {
		return m.dialOnce(ctx, cfg, network, addr)
	}
	return m.dialCached(ctx, cfg, network, addr)
}

func (m *Manager) dialOnce(ctx context.Context, cfg Cfg, network, addr string) (net.Conn, error) {
	cli, err := m.connect(ctx, cfg)
	if err != nil {
		return nil, err
	}

	conn, err := cli.Dial(network, addr)
	if err != nil {
		_ = cli.Close()
		return nil, err
	}

	return wrapConn(conn, cli.Close), nil
}

func (m *Manager) dialCached(ctx context.Context, cfg Cfg, network, addr string) (net.Conn, error) {
	key := cacheKey(cfg)

	m.mu.Lock()
	m.purgeLocked()
	ent := m.cache[key]
	if ent != nil {
		ent.lastUsed = m.now()
		cli := ent.cli
		m.mu.Unlock()
		if conn, err := cli.Dial(network, addr); err == nil {
			return conn, nil
		}

		m.mu.Lock()
		_ = closeEntry(ent)
		delete(m.cache, key)
	}
	m.mu.Unlock()

	cli, err := m.connect(ctx, cfg)
	if err != nil {
		return nil, err
	}

	ent = &entry{cfg: cfg, cli: cli, lastUsed: m.now(), stop: make(chan struct{})}
	if cfg.KeepAlive > 0 {
		go keepAliveLoop(cli, cfg.KeepAlive, ent.stop)
	}

	conn, err := cli.Dial(network, addr)
	if err != nil {
		_ = closeEntry(ent)
		return nil, err
	}

	m.mu.Lock()
	m.cache[key] = ent
	m.mu.Unlock()

	return conn, nil
}

func (m *Manager) connect(ctx context.Context, cfg Cfg) (Client, error) {
	attempts := cfg.Retries + 1
	var lastErr error
	for i := 0; i < attempts; i++ {
		cli, err := m.dial(ctx, cfg)
		if err == nil {
			return cli, nil
		}

		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if i+1 < attempts {
			if err := waitWithContext(ctx, dialRetryDelay); err != nil {
				return nil, err
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("ssh dial failed")
	}
	return nil, lastErr
}

func (m *Manager) purgeLocked() {
	now := m.now()
	for key, ent := range m.cache {
		if now.Sub(ent.lastUsed) > m.ttl {
			_ = closeEntry(ent)
			delete(m.cache, key)
		}
	}
}

func dialSSH(ctx context.Context, cfg Cfg) (Client, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	base := &net.Dialer{Timeout: cfg.Timeout}

	netConn, err := base.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	auth, err := authMethods(cfg)
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}

	hostKeyCb, err := hostKeyCallback(cfg)
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}

	sshCfg := &xssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCb,
		Timeout:         cfg.Timeout,
	}
	if sshCfg.User == "" {
		sshCfg.User = os.Getenv("USER")
	}

	conn, chans, reqs, err := xssh.NewClientConn(netConn, addr, sshCfg)
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}
	return xssh.NewClient(conn, chans, reqs), nil
}

func authMethods(cfg Cfg) ([]xssh.AuthMethod, error) {
	var methods []xssh.AuthMethod

	if cfg.KeyPath != "" {
		keyData, err := os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh key: %w", err)
		}

		signer, err := parseKey(keyData, cfg.KeyPass)
		if err != nil {
			return nil, err
		}

		methods = append(methods, xssh.PublicKeys(signer))
	}

	if cfg.KeyPath == "" && cfg.Pass == "" {
		if signer := loadDefaultKey(cfg.KeyPass); signer != nil {
			methods = append(methods, xssh.PublicKeys(signer))
		}
	}

	if cfg.Agent {
		if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
			conn, err := net.Dial("unix", sock)
			if err == nil {
				methods = append(methods, xssh.PublicKeysCallback(agent.NewClient(conn).Signers))
			}
		}
	}

	if cfg.Pass != "" {
		methods = append(methods, xssh.Password(cfg.Pass))
	}

	if len(methods) == 0 {
		return nil, errors.New("no ssh auth methods")
	}
	return methods, nil
}

func parseKey(data []byte, pass string) (xssh.Signer, error) {
	if pass == "" {
		return xssh.ParsePrivateKey(data)
	}
	return xssh.ParsePrivateKeyWithPassphrase(data, []byte(pass))
}

func loadDefaultKey(pass string) xssh.Signer {
	paths := []string{
		filepath.Join(userHomeDir(), ".ssh", "id_ed25519"),
		filepath.Join(userHomeDir(), ".ssh", "id_rsa"),
		filepath.Join(userHomeDir(), ".ssh", "id_ecdsa"),
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		signer, err := parseKey(data, pass)
		if err != nil {
			continue
		}
		return signer
	}
	return nil
}

func hostKeyCallback(cfg Cfg) (xssh.HostKeyCallback, error) {
	if !cfg.Strict {
		return xssh.InsecureIgnoreHostKey(), nil
	}
	if cfg.KnownHosts == "" {
		return nil, errors.New("strict host key but no known_hosts")
	}
	return knownhosts.New(cfg.KnownHosts)
}

func keepAliveLoop(cli Client, interval time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			_, _, err := cli.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				return
			}
		}
	}
}

func closeEntry(ent *entry) error {
	if ent == nil {
		return nil
	}
	select {
	case <-ent.stop:
	default:
		close(ent.stop)
	}
	if ent.cli != nil {
		return ent.cli.Close()
	}
	return nil
}

func waitWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type wrappedConn struct {
	net.Conn
	closeFn func() error
}

func wrapConn(c net.Conn, closer func() error) net.Conn {
	return &wrappedConn{Conn: c, closeFn: closer}
}

func (c *wrappedConn) Close() error {
	var errs []error
	if c.Conn != nil {
		if err := c.Conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if c.closeFn != nil {
		if err := c.closeFn(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
}
