package tunnel

import (
	"errors"
	"net"
)

type wrappedConn struct {
	net.Conn
	closeFn func() error
}

func WrapConn(conn net.Conn, closeFn func() error) net.Conn {
	return &wrappedConn{Conn: conn, closeFn: closeFn}
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
	return errors.Join(errs...)
}
