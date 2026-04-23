package ssh

import (
	"context"
	"errors"
	"net"
	"os"
	"strconv"

	xssh "golang.org/x/crypto/ssh"
)

func dialSSH(ctx context.Context, execCfg execConfig) (client, error) {
	addr := execCfg.ep.addr()
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

func (ep endpoint) addr() string {
	return net.JoinHostPort(ep.host, strconv.Itoa(ep.port))
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
	client
	closeFn func() error
}

func wrapClient(cli client, closeFn func() error) client {
	if closeFn == nil {
		return cli
	}
	return &clientWrap{client: cli, closeFn: closeFn}
}

func (c *clientWrap) Close() error {
	var errs []error
	if c.client != nil {
		if err := c.client.Close(); err != nil {
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
