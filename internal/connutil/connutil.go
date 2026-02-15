package connutil

import (
	"context"
	"errors"
	"net"
	"time"
)

func WaitWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

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
