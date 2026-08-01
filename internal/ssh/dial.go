package ssh

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"

	xssh "golang.org/x/crypto/ssh"
)

func dialSSH(ctx context.Context, execCfg execConfig) (sshClient, error) {
	addr := execCfg.addr()
	base := &net.Dialer{Timeout: execCfg.Timeout}

	netConn, err := base.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	auth, closeAuth, err := execCfg.auth.methods()
	if err != nil {
		return nil, joinCloseErr(err, closeAuthConn(netConn, closeAuth))
	}

	hostKeyCb, err := execCfg.hk.callback()
	if err != nil {
		return nil, joinCloseErr(err, closeAuthConn(netConn, closeAuth))
	}

	sshCfg := &xssh.ClientConfig{
		User:            execCfg.User,
		Auth:            auth,
		HostKeyCallback: hostKeyCb,
		Timeout:         execCfg.Timeout,
	}
	if sshCfg.User == "" {
		sshCfg.User = os.Getenv("USER")
	}

	conn, chans, reqs, err := xssh.NewClientConn(netConn, addr, sshCfg)
	if err != nil {
		return nil, joinCloseErr(err, closeAuthConn(netConn, closeAuth))
	}
	return wrapClient(xssh.NewClient(conn, chans, reqs), closeAuth), nil
}

func (cfg execConfig) addr() string {
	return net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
}

func closeAuthConn(conn net.Conn, closeAuth func() error) error {
	var errs []error
	if conn != nil {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if closeAuth != nil {
		if err := closeAuth(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

type clientWrap struct {
	sshClient
	closeFn func() error
}

func wrapClient(cli sshClient, closeFn func() error) sshClient {
	if closeFn == nil {
		return cli
	}
	return &clientWrap{sshClient: cli, closeFn: closeFn}
}

func (c *clientWrap) Close() error {
	var errs []error
	if c.sshClient != nil {
		if err := c.sshClient.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if c.closeFn != nil {
		if err := c.closeFn(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
